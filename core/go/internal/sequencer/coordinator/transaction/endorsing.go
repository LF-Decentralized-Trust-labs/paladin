/*
 * Copyright © 2025 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
package transaction

import (
	"context"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type endorsementRequirement struct {
	attRequest *prototk.AttestationRequest
	party      string
}

func (t *Transaction) applyEndorsement(ctx context.Context, endorsement *prototk.AttestationResult, requestID uuid.UUID) error {
	pendingRequestsForAttRequest, ok := t.pendingEndorsementRequests[endorsement.Name]
	if !ok {
		log.L(ctx).Infof("Ignoring endorsement response for transaction %s from %s because no pending request found for attestation request name %s", t.ID, endorsement.Verifier.Lookup, endorsement.Name)
		return nil
	}
	if pendingRequest, ok := pendingRequestsForAttRequest[endorsement.Verifier.Lookup]; ok {
		if pendingRequest.IdempotencyKey() == requestID {
			log.L(ctx).Infof("Endorsement received for transaction %s from %s", t.ID, endorsement.Verifier.Lookup)
			delete(t.pendingEndorsementRequests[endorsement.Name], endorsement.Verifier.Lookup)
			t.PostAssembly.Endorsements = append(t.PostAssembly.Endorsements, endorsement)
		} else {
			log.L(ctx).Infof("Ignoring endorsement response for transaction %s from %s because idempotency key %s does not match expected %s ", t.ID, endorsement.Verifier.Lookup, requestID.String(), pendingRequest.IdempotencyKey().String())
		}
	} else {
		log.L(ctx).Infof("Ignoring endorsement response for transaction %s from %s because no pending request found", t.ID, endorsement.Verifier.Lookup)
	}

	return nil
}

func (t *Transaction) applyEndorsementRejection(ctx context.Context, revertReason string, party string, attestationRequestName string) error {
	//The endorsement rejection is not currently stored in the PrivateTransaction struct.
	//  Only thing that the state machine currently cares about is the error count (which may be used as part of the logic to select transactions from the pool for assembly) and that is incremented in the transition functions
	return nil
}

func (d *Transaction) IsEndorsed(ctx context.Context) bool {
	return !d.hasUnfulfilledEndorsementRequirements(ctx)
}

func (d *Transaction) hasUnfulfilledEndorsementRequirements(ctx context.Context) bool {
	return len(d.unfulfilledEndorsementRequirements(ctx)) > 0
}

func (d *Transaction) unfulfilledEndorsementRequirements(ctx context.Context) []*endorsementRequirement {
	unfulfilledEndorsementRequirements := make([]*endorsementRequirement, 0)
	if d.PostAssembly == nil {
		log.L(ctx).Debug("PostAssembly is nil so there are no outstanding endorsement requirements")
		return unfulfilledEndorsementRequirements
	}
	for _, attRequest := range d.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				found := false
				for _, endorsement := range d.PostAssembly.Endorsements {
					found = endorsement.Name == attRequest.Name &&
						party == endorsement.Verifier.Lookup &&
						attRequest.VerifierType == endorsement.Verifier.VerifierType

					log.L(ctx).Infof("endorsement matched=%t: request[name=%s,party=%s,verifierType=%s] endorsement[name=%s,party=%s,verifierType=%s] verifier=%s",
						found,
						attRequest.Name, party, attRequest.VerifierType,
						endorsement.Name, endorsement.Verifier.Lookup, endorsement.Verifier.VerifierType,
						endorsement.Verifier.Verifier,
					)
					if found {
						break
					}
				}
				if !found {
					log.L(ctx).Debugf("endorsement requirement for %s unfulfilled for transaction %s", party, d.ID)
					unfulfilledEndorsementRequirements = append(unfulfilledEndorsementRequirements, &endorsementRequirement{party: party, attRequest: attRequest})
				}
			}
		}
	}
	return unfulfilledEndorsementRequirements
}

// Function sendEndorsementRequests iterates through the attestation plan and for each endorsement request that has not been fulfilled
// sends an endorsement request to the appropriate party unless there was a recent request (i.e. within the retry threshold)
// it is safe to call this function multiple times and on a frequent basis (e.g. every heartbeat interval while in the endorsement gathering state) as it will not send duplicate requests unless they have timedout
func (t *Transaction) sendEndorsementRequests(ctx context.Context) error {

	if t.pendingEndorsementRequests == nil {
		//we are starting a new round of endorsement requests so set an interval to remind us to resend any requests that have not been fulfilled on a periodic basis
		//this is done by emitting events rather so that this behavior is obvious from the state machine definition
		t.cancelEndorsementRequestTimeoutSchedule = t.clock.ScheduleInterval(ctx, t.requestTimeout, func() {
			t.emit(&RequestTimeoutIntervalEvent{
				BaseEvent: BaseEvent{
					TransactionID: t.ID,
				},
			})
		})
		t.pendingEndorsementRequests = make(map[string]map[string]*common.IdempotentRequest)
	}

	for _, endorsementRequirement := range t.unfulfilledEndorsementRequirements(ctx) {

		pendingRequestsForAttRequest, ok := t.pendingEndorsementRequests[endorsementRequirement.attRequest.Name]
		if !ok {
			pendingRequestsForAttRequest = make(map[string]*common.IdempotentRequest)
			t.pendingEndorsementRequests[endorsementRequirement.attRequest.Name] = pendingRequestsForAttRequest
		}
		pendingRequest, ok := pendingRequestsForAttRequest[endorsementRequirement.party]
		if !ok {
			pendingRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
				return t.requestEndorsement(ctx, idempotencyKey, endorsementRequirement.party, endorsementRequirement.attRequest)
			})
			pendingRequestsForAttRequest[endorsementRequirement.party] = pendingRequest
		}
		err := pendingRequest.Nudge(ctx)
		if err != nil {
			log.L(ctx).Errorf("Failed to nudge endorsement request for party %s: %s", endorsementRequirement.party, err)
			t.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerEndorsementRequestError), endorsementRequirement.party, err.Error())
		}

	}

	return nil
}

func (t *Transaction) requestEndorsement(ctx context.Context, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest) error {

	err := t.messageSender.SendEndorsementRequest(
		ctx,
		idempotencyKey,
		party,
		attRequest,
		t.PreAssembly.TransactionSpecification,
		t.PreAssembly.Verifiers,
		t.PostAssembly.Signatures,
		toEndorsableList(t.PostAssembly.InputStates),
		toEndorsableList(t.PostAssembly.OutputStates),
		toEndorsableList(t.PostAssembly.InfoStates),
	)
	if err != nil {
		log.L(ctx).Errorf("Failed to send endorsement request to party %s: %s", party, err)
		t.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerEndorsementRequestError), party, err.Error())
	}
	return err
}

func toEndorsableList(states []*components.FullState) []*prototk.EndorsableState {
	endorsableList := make([]*prototk.EndorsableState, len(states))
	for i, input := range states {
		endorsableList[i] = &prototk.EndorsableState{
			Id:            input.ID.String(),
			SchemaId:      input.Schema.String(),
			StateDataJson: string(input.Data),
		}
	}
	return endorsableList
}

func action_SendEndorsementRequests(ctx context.Context, txn *Transaction) error {
	return txn.sendEndorsementRequests(ctx)
}

func action_NudgeEndorsementRequests(ctx context.Context, txn *Transaction) error {
	return txn.sendEndorsementRequests(ctx)
}

// endorsed by all required endorsers
func guard_AttestationPlanFulfilled(ctx context.Context, txn *Transaction) bool {
	return !txn.hasUnfulfilledEndorsementRequirements(ctx)
}
