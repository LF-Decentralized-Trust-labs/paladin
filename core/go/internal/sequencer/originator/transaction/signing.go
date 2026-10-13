/*
 * Copyright © 2026 Kaleido, Inc.
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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// guard_HasLocalSignRequirement returns true when the assembled plan contains a SIGN attestation with a
// party local to this node. Under the push model only the originating node produces signatures, so a
// SIGN party on any other node is skipped here (it is fulfilled by nobody and is fundamentally
// unsupported). SIGN-free and all-remote plans skip straight to endorsement.
func guard_HasLocalSignRequirement(ctx context.Context, txn *originatorTransaction) bool {
	if txn.pt.PostAssembly == nil {
		return false
	}
	localNode := txn.nodeName
	for _, attRequest := range txn.pt.PostAssembly.AssembleResponse.GetAttestationPlan() {
		if attRequest.AttestationType != prototk.AttestationType_SIGN {
			continue
		}
		for _, party := range attRequest.Parties {
			_, signerNode, err := pldtypes.PrivateIdentityLocator(party).Validate(ctx, localNode, true)
			if err != nil {
				log.L(ctx).Warnf("could not validate identity locator for signing party %q: %v", party, err)
				continue
			}
			if signerNode == localNode {
				return true
			}
		}
	}
	return false
}

// action_FulfilSignAttestations spawns a background goroutine that signs every local SIGN attestation of
// the assembled plan and queues a SignSuccessEvent (or SignErrorEvent on failure). It captures the values
// the goroutine needs so that the goroutine reads nothing from txn.pt.
func action_FulfilSignAttestations(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	// Cancel any previously in-flight sign goroutine for a superseded request.
	if txn.cancelCurrentSign != nil {
		txn.cancelCurrentSign()
	}

	txID := txn.pt.ID
	requestID := txn.latestFulfilledAssembleRequestID
	attestationPlan := txn.pt.PostAssembly.AssembleResponse.GetAttestationPlan()

	signCtx, cancel := context.WithCancel(ctx)
	txn.cancelCurrentSign = cancel

	go func() {
		defer cancel()
		txn.handleSign(signCtx, txID, requestID, attestationPlan)
	}()
	return nil
}

// handleSign runs off the state-machine goroutine. It must not mutate txn.pt; it only reads its captured
// arguments, calls the key manager via engineIntegration, and queues an event.
func (txn *originatorTransaction) handleSign(ctx context.Context, txID uuid.UUID, requestID uuid.UUID, attestationPlan []*prototk.AttestationRequest) {
	results := make([]*prototk.AttestationResult, 0)
	for _, attRequest := range attestationPlan {
		if attRequest.AttestationType != prototk.AttestationType_SIGN {
			continue
		}
		for _, party := range attRequest.Parties {
			result, err := txn.engineIntegration.SignAttestation(ctx, txID, attRequest, party)
			if err != nil {
				if ctx.Err() != nil {
					log.L(ctx).Debugf("abandoning signing for transaction %s: superseded or cancelled", txID)
					return
				}
				log.L(ctx).Errorf("failed to sign attestation %s for party %s: %s", attRequest.Name, party, err)
				txn.queueEventForOriginator(ctx, &SignErrorEvent{
					BaseEvent: BaseEvent{TransactionID: txID},
					RequestID: requestID,
				})
				return
			}
			if result != nil {
				results = append(results, result)
			}
		}
	}
	log.L(ctx).Debugf("emitting SignSuccessEvent for transaction %s with %d signatures", txID, len(results))
	txn.queueEventForOriginator(ctx, &SignSuccessEvent{
		BaseEvent: BaseEvent{TransactionID: txID},
		RequestID: requestID,
		Results:   results,
	})
}

// action_SignSuccess records the collected signatures on the local PostAssembly so they are available to
// action_SendSignResponse, mirroring how action_AssembleSuccess records the PostAssembly before
// action_SendAssembleSuccessResponse reads it.
func action_SignSuccess(_ context.Context, t *originatorTransaction, event common.Event) error {
	e := event.(*SignSuccessEvent)
	t.pt.PostAssembly.AssembleResponse.Signatures = append(t.pt.PostAssembly.AssembleResponse.Signatures, e.Results...)
	return nil
}

// action_SendSignResponse pushes one SignResponse per collected signature to the current delegate, tagged
// with the assemble-request-id so the coordinator can correlate it against its pending assemble request.
func action_SendSignResponse(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	for _, signature := range txn.pt.PostAssembly.AssembleResponse.GetSignatures() {
		if err := txn.transportWriter.SendSignResponse(ctx, txn.currentDelegate, &engineProto.SignResponse{
			TransactionId:     txn.pt.ID.String(),
			AssembleRequestId: txn.latestFulfilledAssembleRequestID.String(),
			ContractAddress:   txn.pt.Address.HexString(),
			AttestationResult: signature,
			PostAssembly:      txn.pt.PostAssembly.AssembleResponse,
		}); err != nil {
			return err
		}
	}
	return nil
}

// action_SendSignError pushes a SignError to the current delegate so the coordinator can repool or evict
// the transaction against the shared assemble retry budget.
func action_SendSignError(ctx context.Context, txn *originatorTransaction, _ common.Event) error {
	return txn.transportWriter.SendSignError(ctx, txn.currentDelegate, &engineProto.SignError{
		TransactionId:     txn.pt.ID.String(),
		AssembleRequestId: txn.latestFulfilledAssembleRequestID.String(),
		ContractAddress:   txn.pt.Address.HexString(),
	})
}

// action_CancelCurrentSign cancels an in-flight sign goroutine when leaving State_Signing. It is a no-op
// when no goroutine is running (identical shape to action_CancelCurrentAssembly).
func action_CancelCurrentSign(_ context.Context, txn *originatorTransaction, _ common.Event) error {
	if txn.cancelCurrentSign != nil {
		txn.cancelCurrentSign()
		txn.cancelCurrentSign = nil
	}
	return nil
}

// validator_SignSuccessMatchesCurrentRequest gates a SignSuccessEvent so that a signature produced for a
// superseded assemble request is ignored, mirroring validator_AssembleSuccessMatchesCurrentRequest.
func validator_SignSuccessMatchesCurrentRequest(_ context.Context, t *originatorTransaction, event common.Event) (bool, error) {
	e := event.(*SignSuccessEvent)
	return t.latestFulfilledAssembleRequestID == e.RequestID, nil
}
