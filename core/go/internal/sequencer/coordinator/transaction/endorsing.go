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
	"fmt"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type endorsementRequirement struct {
	attRequest *prototk.AttestationRequest
	party      string
}

// applyEndorsement collects an endorsement and clears the request it answers, so the party is not
// nudged again. It is only reached for an endorsement that validator_MatchesPendingEndorsementRequest
// has matched to the request still outstanding for that attestation requirement and party, which is
// also what makes the verifier safe to dereference here.
func (t *coordinatorTransaction) applyEndorsement(ctx context.Context, endorsement *prototk.AttestationResult) error {
	log.L(ctx).Debugf("endorsement '%s' received for transaction %s from %s", endorsement.Name, t.pt.ID, endorsement.Verifier.Lookup)
	delete(t.pendingEndorsementRequests[endorsement.Name], endorsement.Verifier.Lookup)
	t.pt.PostAssembly.CollectedEndorsements = append(t.pt.PostAssembly.CollectedEndorsements, endorsement)

	// Log complete list of current endorsements
	for _, endorsement := range t.pt.PostAssembly.CollectedEndorsements {
		log.L(ctx).Debugf("completed endorsement: %+v", endorsement)
	}
	return nil
}

func (t *coordinatorTransaction) hasUnfulfilledEndorsementRequirements(ctx context.Context) bool {
	return len(t.unfulfilledEndorsementRequirements(ctx)) > 0
}

func (t *coordinatorTransaction) unfulfilledEndorsementRequirements(ctx context.Context) []*endorsementRequirement {
	unfulfilledEndorsementRequirements := make([]*endorsementRequirement, 0)
	if t.pt.PostAssembly == nil {
		log.L(ctx).Debug("PostAssembly is nil so there are no outstanding endorsement requirements")
		return unfulfilledEndorsementRequirements
	}
	for _, attRequest := range t.pt.PostAssembly.AssembleResponse.GetAttestationPlan() {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			// When threshold is unset (0) every party must endorse, which is equivalent to a
			// threshold equal to the total party count.
			effectiveThreshold := int(attRequest.GetThreshold())
			if effectiveThreshold == 0 {
				effectiveThreshold = len(attRequest.Parties)
			}

			receivedCount := 0
			for _, endorsement := range t.pt.PostAssembly.CollectedEndorsements {
				if endorsement.Name == attRequest.Name &&
					attRequest.VerifierType == endorsement.Verifier.VerifierType {
					receivedCount++
				}
			}
			shortfall := effectiveThreshold - receivedCount
			if shortfall <= 0 {
				log.L(ctx).Debugf("endorsement request %s: threshold %d met (received %d)", attRequest.Name, effectiveThreshold, receivedCount)
				continue
			}

			for _, party := range attRequest.Parties {
				log.L(ctx).Debugf("party %s may endorse this request. Checking for endorsement", party)
				found := false
				for _, endorsement := range t.pt.PostAssembly.CollectedEndorsements {
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
					log.L(ctx).Debugf("no endorsement exists from party %s for transaction %s (need %d of %d)",
						party, t.pt.ID, effectiveThreshold, len(attRequest.Parties))
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
func (t *coordinatorTransaction) sendEndorsementRequests(ctx context.Context) error {

	log.L(ctx).Debugf("sendEndorsementRequests: number of verifiers %d", len(t.pt.PostAssembly.AssembleResponse.GetResolvedVerifiers()))

	if t.pendingEndorsementRequests == nil {
		//we are starting a new round of endorsement requests so set an interval to remind us to resend any requests that have not been fulfilled on a periodic basis
		//this is done by emitting events rather so that this behavior is obvious from the state machine definition
		t.scheduleRequestTimeout(ctx)
		t.pendingEndorsementRequests = make(map[string]map[string]*common.IdempotentRequest)
		// Notify the coordinator about endorser nodes discovered from the attestation plan so it can
		// grow the endorser candidate pool even when no candidates were pre-configured.
		t.notifyEndorserCandidates(ctx, t.extractEndorserNodes(ctx)...)
	}

	for _, endorsementRequirement := range t.unfulfilledEndorsementRequirements(ctx) {
		pendingRequestsForAttRequest, ok := t.pendingEndorsementRequests[endorsementRequirement.attRequest.Name]
		if !ok {
			pendingRequestsForAttRequest = make(map[string]*common.IdempotentRequest)
			t.pendingEndorsementRequests[endorsementRequirement.attRequest.Name] = pendingRequestsForAttRequest
		}
		pendingRequest, ok := pendingRequestsForAttRequest[endorsementRequirement.party]
		if ok && pendingRequest == nil {
			// Party was marked as permanently failed this round — do not re-send.
			continue
		}
		if !ok {
			pendingRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
				return t.requestEndorsement(ctx, idempotencyKey, endorsementRequirement.party, endorsementRequirement.attRequest)
			})
			pendingRequestsForAttRequest[endorsementRequirement.party] = pendingRequest
		}

		err := pendingRequest.Nudge(ctx)
		if err != nil {
			log.L(ctx).Errorf("failed to nudge endorsement request for party %s: %s", endorsementRequirement.party, err)
		}
	}

	return nil
}

// extractEndorserNodes returns deduplicated node names from all ENDORSE-type attestation parties.
func (t *coordinatorTransaction) extractEndorserNodes(ctx context.Context) []string {
	seen := make(map[string]struct{})
	nodes := make([]string, 0)
	if t.pt.PostAssembly == nil {
		return nodes
	}
	for _, attRequest := range t.pt.PostAssembly.AssembleResponse.GetAttestationPlan() {
		if attRequest.AttestationType != prototk.AttestationType_ENDORSE {
			continue
		}
		for _, party := range attRequest.Parties {
			node, err := pldtypes.PrivateIdentityLocator(party).Node(ctx, false)
			if err != nil {
				log.L(ctx).Warnf("could not extract node from endorser party %q: %v", party, err)
				continue
			}
			if _, exists := seen[node]; !exists {
				seen[node] = struct{}{}
				nodes = append(nodes, node)
			}
		}
	}
	return nodes
}

func (t *coordinatorTransaction) resetEndorsementRequests(ctx context.Context) {
	log.L(ctx).Trace("resetting endorsement requests")
	t.clearTimeoutSchedules()
	t.pendingEndorsementRequests = nil
	t.endorseFailureCountByRequirement = nil
	t.endorseRevertCountByRequirement = nil
	t.endorseRevertReasons = nil
	t.endorseToleranceByRequirement = nil
}

func (t *coordinatorTransaction) requestEndorsement(ctx context.Context, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest) error {
	partyNode, err := pldtypes.PrivateIdentityLocator(party).Node(ctx, false)
	if err != nil {
		return err
	}
	err = t.transportWriter.SendEndorsementRequest(ctx, partyNode, &engineProto.EndorsementRequest{
		IdempotencyKey:           idempotencyKey.String(),
		ContractAddress:          t.pt.Address.HexString(),
		AttestationRequest:       attRequest,
		Party:                    party,
		TransactionSpecification: t.pt.PreAssembly.TransactionSpecification,
		BlockContext:             t.pt.PostAssembly.AssembleResponse.GetBlockContext(),
		Verifiers:                t.pt.PostAssembly.AssembleResponse.GetResolvedVerifiers(),
		Signatures:               t.pt.PostAssembly.AssembleResponse.GetSignatures(),
		InputStates:              t.pt.PostAssembly.AssembleResponse.GetInputStates(),
		ReadStates:               t.pt.PostAssembly.AssembleResponse.GetReadStates(),
		OutputStates:             t.pt.PostAssembly.OutputStates,
		InfoStates:               t.pt.PostAssembly.InfoStates,
		ExpiryTimeUnixMs:         t.clock.Now().Add(t.stateTimeout).UnixMilli(),
		CoordinatorBlockHeight:   t.getBlockHeight(),
		BlockHeightTolerance:     int64(t.blockHeightTolerance),
	})
	if err != nil {
		log.L(ctx).Errorf("failed to send endorsement request to party %s: %s", party, err)
	}
	return err
}

func action_Endorsed(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*EndorsedEvent)
	return t.applyEndorsement(ctx, e.Endorsement)
}

func action_RefreshBlockHeight(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.refreshBlockHeight(ctx)
	return nil
}

func action_SendEndorsementRequests(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.sendEndorsementRequests(ctx)
}

func action_NudgeEndorsementRequests(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	return txn.sendEndorsementRequests(ctx)
}

func action_ResetEndorsementRequests(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.resetEndorsementRequests(ctx)
	return nil
}

// endorsed by all required endorsers
func guard_AttestationPlanFulfilled(ctx context.Context, txn *coordinatorTransaction) bool {
	return !txn.hasUnfulfilledEndorsementRequirements(ctx)
}

// guard_EndorseFailureExceedsTolerance returns true if the failure count for any single
// attestation requirement now exceeds its pre-computed tolerance, meaning the plan can no
// longer be fulfilled. action_RecordEndorseFailure increments the count before this guard runs.
func guard_EndorseFailureExceedsTolerance(_ context.Context, txn *coordinatorTransaction) bool {
	for reqName, tolerance := range txn.endorseToleranceByRequirement {
		if txn.endorseFailureCountByRequirement[reqName] > tolerance {
			return true
		}
	}
	return false
}

// guard_EndorseRevertExceedsTolerance returns true if the number of endorsement reverts for any
// single attestation requirement now exceeds its tolerance, meaning the threshold can no longer be
// reached. A correctly implemented domain does not assemble a transaction that its own endorsers
// would revert, so the transaction is finalized as reverted rather than repooled.
// action_RecordEndorseFailure increments the count before this guard runs.
func guard_EndorseRevertExceedsTolerance(_ context.Context, txn *coordinatorTransaction) bool {
	for reqName, tolerance := range txn.endorseToleranceByRequirement {
		if txn.endorseRevertCountByRequirement[reqName] > tolerance {
			return true
		}
	}
	return false
}

// endorseRevertFailureMessage renders the accumulated revert reasons into the message recorded on
// the transaction receipt.
func (t *coordinatorTransaction) endorseRevertFailureMessage(ctx context.Context) string {
	return i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgSequencerEndorseRevert), strings.Join(t.endorseRevertReasons, "; "))
}

// action_NotifyOriginatorOfEndorseRevert tells the originator the transaction is reverted so its own
// state machine terminates rather than waiting for a dispatch that will never come.
func action_NotifyOriginatorOfEndorseRevert(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	return t.transportWriter.SendTransactionConfirmed(ctx, t.originatorNode, &engineProto.TransactionConfirmed{
		Id:              uuid.New().String(),
		TransactionId:   t.pt.ID.String(),
		ContractAddress: t.pt.Address.HexString(),
		Outcome:         engineProto.TransactionConfirmed_OUTCOME_REVERTED,
		FailureMessage:  t.endorseRevertFailureMessage(ctx),
	})
}

// action_FinalizeEndorseRevert writes the failure receipt. Retries indefinitely on error, as the
// other finalization paths do — the transaction is terminal either way and the receipt must land.
func action_FinalizeEndorseRevert(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	failureMessage := t.endorseRevertFailureMessage(ctx)
	log.L(ctx).Infof("finalizing transaction %s as reverted on endorsement: %s", t.pt.ID, failureMessage)
	var tryFinalize func()
	tryFinalize = func() {
		t.syncPoints.QueueTransactionFinalize(ctx,
			&syncpoints.TransactionFinalizeRequest{
				Domain:          t.pt.Domain,
				ContractAddress: t.pt.Address,
				Originator:      t.originator,
				TransactionID:   t.pt.ID,
				FailureMessage:  failureMessage,
			},
			func(ctx context.Context) {
				log.L(ctx).Debugf("finalized endorsement revert for transaction %s", t.pt.ID)
			},
			func(ctx context.Context, err error) {
				log.L(ctx).Errorf("error finalizing endorsement revert for transaction %s: %s", t.pt.ID, err)
				tryFinalize()
			},
		)
	}
	tryFinalize()
	return nil
}

// validator_MatchesPendingEndorsementRequest gates every endorsement outcome - endorsed, reverted,
// errored and rejected - so that each outstanding request is answered at most once. The pending
// requests map is the record of what is still outstanding this round, so a response is only acted on
// when it belongs to a request still waiting for an answer: a missing attestation requirement or
// party means the round has already been reset or the party has already answered, a nil entry means
// this party has already been recorded as failed, and a request ID that does not match the
// outstanding request means the response belongs to an earlier round.
//
// Without this, a duplicate or late failure response would increment the failure counters a second
// time for a party that has only failed once, and could push a requirement past a tolerance whose
// parties had not actually been exhausted.
func validator_MatchesPendingEndorsementRequest(ctx context.Context, txn *coordinatorTransaction, event common.Event) (bool, error) {
	var reqName, party string
	var requestID uuid.UUID
	switch e := event.(type) {
	case *EndorsedEvent:
		if e.Endorsement == nil || e.Endorsement.Verifier == nil {
			log.L(ctx).Warnf("ignoring endorsement response for transaction %s because it carries no verifier", txn.pt.ID)
			return false, nil
		}
		reqName, party, requestID = e.Endorsement.Name, e.Endorsement.Verifier.Lookup, e.RequestID
	case *EndorseErrorEvent:
		reqName, party, requestID = e.AttestationRequestName, e.Party, e.RequestID
	case *EndorseRequestRejectedEvent:
		reqName, party, requestID = e.AttestationRequestName, e.Party, e.RequestID
	case *EndorseRevertEvent:
		reqName, party, requestID = e.AttestationRequestName, e.Party, e.RequestID
	default:
		return false, nil
	}
	pendingRequest, ok := txn.pendingEndorsementRequests[reqName][party]
	if !ok || pendingRequest == nil {
		log.L(ctx).Warnf("ignoring %s for transaction %s from %s because no request for '%s' is outstanding for that party", event.TypeString(), txn.pt.ID, party, reqName)
		return false, nil
	}
	if pendingRequest.IdempotencyKey() != requestID {
		log.L(ctx).Warnf("ignoring %s for transaction %s from %s because request ID %s is not the outstanding request %s for '%s'", event.TypeString(), txn.pt.ID, party, requestID, pendingRequest.IdempotencyKey(), reqName)
		return false, nil
	}
	return true, nil
}

// action_RecordEndorseFailure records the failing party for the given attestation requirement,
// removing them from the pending requests map so they are not nudged again this round.
//
// Two counters are kept per requirement, because they answer different questions:
//
//   - endorseFailureCountByRequirement counts every failure whatever the cause, and answers
//     "can this attestation plan still be fulfilled this round".
//   - endorseRevertCountByRequirement counts only endorsement reverts, and answers "did an
//     endorser judge this transaction invalid". A revert means the endorser evaluated the
//     transaction and refused it; a correctly implemented domain should never assemble a
//     transaction that reverts at endorsement.
//
// A revert increments both: it is a failure like any other for the threshold arithmetic, and
// additionally a rejection of the transaction itself. Unexpected errors and rejections increment
// only the first, since either may be transient and succeed on a retry.
func action_RecordEndorseFailure(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	var reqName, party, revertReason string
	isRevert := false
	switch e := event.(type) {
	case *EndorseErrorEvent:
		reqName = e.AttestationRequestName
		party = e.Party
		log.L(ctx).Warnf("endorsement error by %s (%s)", party, reqName)
	case *EndorseRequestRejectedEvent:
		reqName = e.AttestationRequestName
		party = e.Party
		switch e.RejectionReason {
		case engineProto.RejectionReason_ENDORSER_IS_ACTIVE_COORDINATOR:
			log.L(ctx).Warnf("endorsement request rejected by %s (%s): endorser is the active coordinator", party, reqName)
		case engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE:
			log.L(ctx).Warnf("endorsement request rejected by %s (%s) due to block height tolerance: coordinator block height=%d, endorser block height=%d, endorser tolerance=%d",
				party, reqName, e.CoordinatorBlockHeight, e.EndorserBlockHeight, e.BlockHeightTolerance)
		}
	case *EndorseRevertEvent:
		reqName = e.AttestationRequestName
		party = e.Party
		isRevert = true
		revertReason = e.RevertReason
		log.L(ctx).Warnf("endorsement reverted by %s (%s): %s", party, reqName, revertReason)
	}
	if party == "" {
		log.L(ctx).Warnf("action_RecordEndorseFailure: missing party on event %T", event)
		return nil
	}
	// Mark the party as permanently failed this round using a nil sentinel in the pending map,
	// so sendEndorsementRequests will not re-send to them.
	t.pendingEndorsementRequests[reqName][party] = nil
	// Increment the per-requirement failure count. Handler actions run before transition guards,
	// so guard_EndorseFailureExceedsTolerance sees the post-increment value.
	if t.endorseFailureCountByRequirement == nil {
		t.endorseFailureCountByRequirement = make(map[string]int)
	}
	t.endorseFailureCountByRequirement[reqName]++
	if isRevert {
		if t.endorseRevertCountByRequirement == nil {
			t.endorseRevertCountByRequirement = make(map[string]int)
		}
		t.endorseRevertCountByRequirement[reqName]++
		// Accumulated for the finalization message, so the originator's receipt names every
		// endorser that refused and why, not just the one that tipped it over the tolerance.
		t.endorseRevertReasons = append(t.endorseRevertReasons, fmt.Sprintf("[%s] %s", party, revertReason))
	}
	return nil
}

// action_ComputeEndorseTolerances pre-computes the per-requirement failure tolerance from the
// current attestation plan: how many parties can fail without making a requirement impossible to
// fulfill (tolerance = len(parties) - effectiveThreshold). Must run before sendEndorsementRequests.
func action_ComputeEndorseTolerances(_ context.Context, t *coordinatorTransaction, _ common.Event) error {
	tolerances := make(map[string]int)
	if t.pt.PostAssembly != nil {
		for _, attRequest := range t.pt.PostAssembly.AssembleResponse.GetAttestationPlan() {
			if attRequest.AttestationType != prototk.AttestationType_ENDORSE {
				continue
			}
			threshold := int(attRequest.GetThreshold())
			if threshold == 0 {
				threshold = len(attRequest.Parties)
			}
			tolerances[attRequest.Name] = len(attRequest.Parties) - threshold
		}
	}
	t.endorseToleranceByRequirement = tolerances
	return nil
}
