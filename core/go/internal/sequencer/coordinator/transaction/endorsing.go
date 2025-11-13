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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type endorsementRequirement struct {
	attRequest *prototk.AttestationRequest
	party      string
}

func (t *Transaction) applyEndorsement(ctx context.Context, endorsement *prototk.AttestationResult, requestID uuid.UUID) error {
	t.pendingEndorsementsMutex.Lock()
	defer t.pendingEndorsementsMutex.Unlock()
	log.L(ctx).Debugf("apply endorsement - received endorsement name '%s'", endorsement.Name)
	pendingRequestsForAttRequest, ok := t.pendingEndorsementRequests[endorsement.Name]
	if !ok {
		log.L(ctx).Debugf("ignoring endorsement response for transaction %s from %s because no pending request found for attestation request name %s", t.ID, endorsement.Verifier.Lookup, endorsement.Name)
		return nil
	}
	if pendingRequest, ok := pendingRequestsForAttRequest[endorsement.Verifier.Lookup]; ok {
		if pendingRequest.IdempotencyKey() == requestID {
			log.L(ctx).Debugf("endorsement '%s' received for transaction %s from %s", endorsement.Name, t.ID, endorsement.Verifier.Lookup)
			delete(t.pendingEndorsementRequests[endorsement.Name], endorsement.Verifier.Lookup)
			t.PostAssembly.Endorsements = append(t.PostAssembly.Endorsements, endorsement)

			// MRW TODO - Hashing the TX for dispatch confirmation requires that there is > 0 signatures. Need to follow up where an endorsed TX populates the signatures. Temporarily put this workaround in.
			// log.L(ctx).Infof("Applying endorsement. Appending %+v to list of endorsements received for transaction %s from %s", endorsement, t.ID, endorsement.Verifier.Lookup)
			// t.PostAssembly.Signatures = append(t.PostAssembly.Signatures, endorsement)
		} else {
			log.L(ctx).Debugf("ignoring endorsement response for transaction %s from %s because idempotency key %s does not match expected %s ", t.ID, endorsement.Verifier.Lookup, requestID.String(), pendingRequest.IdempotencyKey().String())
		}
	} else {
		log.L(ctx).Debugf("ignoring endorsement response for transaction %s from %s because no pending request found", t.ID, endorsement.Verifier.Lookup)
	}

	// Log complete list of current endorsements
	for _, endorsement := range t.PostAssembly.Endorsements {
		log.L(ctx).Debugf("completed endorsement: %+v", endorsement)
	}
	return nil
}

func (t *Transaction) applyEndorsementRejection(ctx context.Context, revertReason string, party string, attestationRequestName string) error {
	//The endorsement rejection is not currently stored in the PrivateTransaction struct.
	//  Only thing that the state machine currently cares about is the error count (which may be used as part of the logic to select transactions from the pool for assembly) and that is incremented in the transition functions
	return nil
}

func (t *Transaction) IsEndorsed(ctx context.Context) bool {
	return !t.hasUnfulfilledEndorsementRequirements(ctx)
}

func (t *Transaction) hasUnfulfilledEndorsementRequirements(ctx context.Context) bool {
	return len(t.unfulfilledEndorsementRequirements(ctx)) > 0
}

func (t *Transaction) unfulfilledEndorsementRequirements(ctx context.Context) []*endorsementRequirement {
	unfulfilledEndorsementRequirements := make([]*endorsementRequirement, 0)
	if t.PostAssembly == nil {
		log.L(ctx).Debug("PostAssembly is nil so there are no outstanding endorsement requirements")
		return unfulfilledEndorsementRequirements
	}
	for _, attRequest := range t.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				log.L(ctx).Debugf("party %s must endorse this request. Checking for endorsement", party)
				found := false
				for _, endorsement := range t.PostAssembly.Endorsements {
					log.L(ctx).Debugf("existing endorsement from party %s", endorsement.Verifier.Lookup)
					found = endorsement.Name == attRequest.Name &&
						party == endorsement.Verifier.Lookup &&
						attRequest.VerifierType == endorsement.Verifier.VerifierType

					if found {
						log.L(ctx).Debugf("endorsement found: request[name=%s,party=%s,verifierType=%s] endorsement[name=%s,party=%s,verifierType=%s] verifier=%s",
							attRequest.Name, party, attRequest.VerifierType,
							endorsement.Name, endorsement.Verifier.Lookup, endorsement.Verifier.VerifierType,
							endorsement.Verifier.Verifier,
						)
						break
					}
				}
				if !found {
					log.L(ctx).Debugf("no endorsement exists from party %s for transaction %s", party, t.ID)
					unfulfilledEndorsementRequirements = append(unfulfilledEndorsementRequirements, &endorsementRequirement{party: party, attRequest: attRequest})
				}
			}
		}
	}

	for _, req := range unfulfilledEndorsementRequirements {
		log.L(ctx).Debugf("unfulfilled endorsement requirement: %+v", req)
	}
	return unfulfilledEndorsementRequirements
}

// Function sendEndorsementRequests iterates through the attestation plan and for each endorsement request that has not been fulfilled
// sends an endorsement request to the appropriate party unless there was a recent request (i.e. within the retry threshold)
// it is safe to call this function multiple times and on a frequent basis (e.g. every heartbeat interval while in the endorsement gathering state) as it will not send duplicate requests unless they have timedout
func (t *Transaction) sendEndorsementRequests(ctx context.Context) error {

	log.L(ctx).Debugf("sendEndorsementRequests: number of verifiers %d", len(t.PreAssembly.Verifiers))

	if t.pendingEndorsementRequests == nil {
		//we are starting a new round of endorsement requests so set an interval to remind us to resend any requests that have not been fulfilled on a periodic basis
		//this is done by emitting events rather so that this behavior is obvious from the state machine definition
		t.cancelEndorsementRequestTimeoutSchedule = t.clock.ScheduleInterval(ctx, t.requestTimeout, func() {
			err := t.eventHandler(ctx, &RequestTimeoutIntervalEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: t.ID,
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error handling RequestTimeoutIntervalEvent: %s", err)
				return
			}
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
			log.L(ctx).Errorf("failed to nudge endorsement request for party %s: %s", endorsementRequirement.party, err)
			t.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgSequencerEndorsementRequestError), endorsementRequirement.party, err.Error())
		}

	}

	return nil
}

func (t *Transaction) resetEndorsementRequests(ctx context.Context) {
	if t.pendingEndorsementRequests == nil {
		return
	}

	log.L(ctx).Trace("resetting endorsement requests")
	t.cancelEndorsementRequestTimeoutSchedule()
	t.pendingEndorsementRequests = make(map[string]map[string]*common.IdempotentRequest)
}

func (t *Transaction) requestEndorsement(ctx context.Context, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest) error {
	err := t.transportWriter.SendEndorsementRequest(
		ctx,
		t.ID,
		idempotencyKey,
		party,
		attRequest,
		t.PreAssembly.TransactionSpecification,
		t.PreAssembly.Verifiers,
		t.PostAssembly.Signatures,
		toEndorsableList(t.PostAssembly.InputStates),
		toEndorsableList(t.PostAssembly.ReadStates),
		toEndorsableList(t.PostAssembly.OutputStates),
		toEndorsableList(t.PostAssembly.InfoStates),
	)
	if err != nil {
		log.L(ctx).Errorf("failed to send endorsement request to party %s: %s", party, err)
		t.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgSequencerEndorsementRequestError), party, err.Error())
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
