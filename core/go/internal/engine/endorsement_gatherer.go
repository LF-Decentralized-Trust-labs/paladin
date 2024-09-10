/*
 * Copyright © 2024 Kaleido, Inc.
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

package engine

import (
	"context"
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

func NewEndorsementGatherer(psc components.DomainSmartContract, keyMgr ethclient.KeyManager) enginespi.EndorsementGatherer {
	return &endorsementGatherer{
		psc:    psc,
		keyMgr: keyMgr,
	}
}

type endorsementGatherer struct {
	psc    components.DomainSmartContract
	keyMgr ethclient.KeyManager
}

func (e *endorsementGatherer) GatherEndorsement(ctx context.Context, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*prototk.EndorsableState, outputStates []*prototk.EndorsableState, partyName string, endorsementRequest *prototk.AttestationRequest) (*prototk.AttestationResult, *string, error) {
	keyHandle, verifier, err := e.keyMgr.ResolveKey(ctx, partyName, endorsementRequest.Algorithm)
	if err != nil {
		errorMessage := fmt.Sprintf("failed to resolve key for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, endorsementRequest.Algorithm, err)
		log.L(ctx).Error(errorMessage)
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError, errorMessage)
	}
	// Invoke the domain
	endorseRes, err := e.psc.EndorseTransaction(ctx,
		transactionSpecification,
		verifiers,
		signatures,
		inputStates,
		outputStates,
		endorsementRequest,
		&prototk.ResolvedVerifier{
			Lookup:    partyName,
			Algorithm: endorsementRequest.Algorithm,
			Verifier:  verifier,
		})
	if err != nil {
		errorMessage := fmt.Sprintf("failed to endorse for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, endorsementRequest.Algorithm, err)
		log.L(ctx).Error(errorMessage)
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError, errorMessage)
	}

	result := &prototk.AttestationResult{
		Name:            endorsementRequest.Name,
		AttestationType: endorsementRequest.AttestationType,
		Verifier:        endorseRes.Endorser,
	}
	switch endorseRes.Result {
	case prototk.EndorseTransactionResponse_REVERT:
		revertReason := "(no revert reason)"
		if endorseRes.RevertReason != nil {
			revertReason = *endorseRes.RevertReason
		}
		return nil, confutil.P(revertReason), nil
	case prototk.EndorseTransactionResponse_SIGN:
		// Build the signature
		signaturePayload, err := e.keyMgr.Sign(ctx, &proto.SignRequest{
			KeyHandle: keyHandle,
			Algorithm: endorsementRequest.Algorithm,
			Payload:   endorseRes.Payload,
		})
		if err != nil {
			errorMessage := fmt.Sprintf("failed to endorse for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, endorsementRequest.Algorithm, err)
			log.L(ctx).Error(errorMessage)
			return nil, nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError, errorMessage)
		}
		result.Payload = signaturePayload.Payload
	case prototk.EndorseTransactionResponse_ENDORSER_SUBMIT:
		result.Constraints = append(result.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
	}

	return result, nil, nil
}
