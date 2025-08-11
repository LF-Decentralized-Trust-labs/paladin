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

package privatetxnmgr

import (
	"context"
	"fmt"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

func NewEndorsementGatherer(p persistence.Persistence, psc components.DomainSmartContract, dCtx components.DomainContext, keyMgr components.KeyManager) ptmgrtypes.EndorsementGatherer {
	return &endorsementGatherer{
		p:      p,
		psc:    psc,
		dCtx:   dCtx,
		keyMgr: keyMgr,
	}
}

type endorsementGatherer struct {
	p      persistence.Persistence
	psc    components.DomainSmartContract
	dCtx   components.DomainContext
	keyMgr components.KeyManager
}

func (e *endorsementGatherer) DomainContext() components.DomainContext {
	return e.dCtx
}

func (e *endorsementGatherer) GatherEndorsement(ctx context.Context, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*prototk.EndorsableState, readStates []*prototk.EndorsableState, outputStates []*prototk.EndorsableState, infoStates []*prototk.EndorsableState, partyName string, endorsementRequest *prototk.AttestationRequest) (*prototk.AttestationResult, *string, error) {

	unqualifiedLookup, err := pldtypes.PrivateIdentityLocator(partyName).Identity(ctx)
	if err != nil {
		errorMessage := fmt.Sprintf("failed to parse lookup key for party %s : %s", partyName, err)
		log.L(ctx).Error(errorMessage)
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, errorMessage)
	}

	resolvedSigner, err := e.keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, endorsementRequest.Algorithm, endorsementRequest.VerifierType)
	if err != nil {
		errorMessage := fmt.Sprintf("failed to resolve key for party %s (algorithm=%s,verifierType=%s): %s", partyName, endorsementRequest.Algorithm, endorsementRequest.VerifierType, err)
		log.L(ctx).Error(errorMessage)
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, errorMessage)
	}
	// Invoke the domain
	endorseRes, err := e.psc.EndorseTransaction(e.dCtx, e.p.NOTX(), &components.PrivateTransactionEndorseRequest{
		TransactionSpecification: transactionSpecification,
		Verifiers:                verifiers,
		Signatures:               signatures,
		InputStates:              inputStates,
		ReadStates:               readStates,
		OutputStates:             outputStates,
		InfoStates:               infoStates,
		Endorsement:              endorsementRequest,
		Endorser: &prototk.ResolvedVerifier{
			Lookup:       partyName,
			Algorithm:    endorsementRequest.Algorithm,
			Verifier:     resolvedSigner.Verifier.Verifier,
			VerifierType: endorsementRequest.VerifierType,
		},
	})
	if err != nil {
		errorMessage := fmt.Sprintf("failed to endorse for party %s (verifier=%s,algorithm=%s): %s", partyName, resolvedSigner.Verifier.Verifier, endorsementRequest.Algorithm, err)
		log.L(ctx).Error(errorMessage)
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, errorMessage)
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
		signaturePayload, err := e.keyMgr.Sign(ctx, resolvedSigner, endorsementRequest.PayloadType, endorseRes.Payload)
		if err != nil {
			errorMessage := fmt.Sprintf("failed to endorse for party %s (verifier=%s,algorithm=%s): %s", partyName, resolvedSigner.Verifier.Verifier, endorsementRequest.Algorithm, err)
			log.L(ctx).Error(errorMessage)
			return nil, nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, errorMessage)
		}
		result.Payload = signaturePayload
	case prototk.EndorseTransactionResponse_ENDORSER_SUBMIT:
		result.Constraints = append(result.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
	}

	return result, nil, nil
}
