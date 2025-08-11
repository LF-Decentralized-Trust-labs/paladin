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
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// assemble a transaction that we are not coordinating, using the provided state locks
// all errors are assumed to be transient and the request should be retried
// if the domain as deemed the request as invalid then it will communicate the `revert` directive via the AssembleTransactionResponse_REVERT result without any error
func (s *Sequencer) assembleForRemoteCoordinator(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) (*components.TransactionPostAssembly, error) {

	log.L(ctx).Debugf("assembleForRemoteCoordinator: Assembling transaction %s ", transactionID)

	log.L(ctx).Debugf("assembleForRemoteCoordinator: resetting domain context with state locks from the coordinator which assumes a block height of %d compared with local blockHeight of %d", blockHeight, s.environment.GetBlockHeight())
	//If our block height is behind the coordinator, there are some states that would otherwise be available to us but we wont see
	// if our block height is ahead of the coordinator, there is a small chance that we we assemble a transaction that the coordinator will not be able to
	// endorse yet but it is better to wait around on the endorsement flow than to wait around on the assemble flow which is single threaded per domain

	err := s.delegateDomainContext.ImportSnapshot(stateLocksJSON)
	if err != nil {
		log.L(ctx).Errorf("assembleForRemoteCoordinator: Error importing state locks: %s", err)
		return nil, err
	}

	postAssembly, err := s.assembleAndSign(ctx, transactionID, preAssembly, s.delegateDomainContext)

	if err != nil {
		log.L(ctx).Errorf("assembleForRemoteCoordinator: Error assembling and signing transaction: %s", err)
		return nil, err
	}

	return postAssembly, nil
}

func (s *Sequencer) AssembleLocal(ctx context.Context, requestID string, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly) {

	log.L(ctx).Debugf("assembleForLocalCoordinator: Assembling transaction %s ", transactionID)

	postAssembly, err := s.assembleAndSign(ctx, transactionID, preAssembly, s.coordinatorDomainContext)

	if err != nil {
		log.L(ctx).Errorf("assembleForLocalCoordinator: Error assembling and signing transaction: %s", err)
		s.publisher.PublishTransactionAssembleFailedEvent(ctx,
			transactionID.String(),
			i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerAssembleError), err.Error()),
			requestID,
		)
		return
	}

	s.publisher.PublishTransactionAssembledEvent(ctx,
		transactionID.String(),
		postAssembly,
		requestID,
	)
}

func (s *Sequencer) resolveLocalTransaction(ctx context.Context, transactionID uuid.UUID) (*components.ResolvedTransaction, error) {
	locallyResolvedTx, err := s.components.TxManager().GetResolvedTransactionByID(ctx, transactionID)
	if err == nil && locallyResolvedTx == nil {
		err = i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrAssembleTxnNotFound, transactionID)
	}
	return locallyResolvedTx, err
}

func (s *Sequencer) assembleAndSign(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, domainContext components.DomainContext) (*components.TransactionPostAssembly, error) {
	//Assembles the transaction and synchronously fulfills any local signature attestation requests
	// Given that the coordinator is single threading calls to assemble, there may be benefits to performance if we were to fulfill the signature request async
	// but that would introduce levels of complexity that may not be justified so this is open as a potential for future optimization where we would need to think about
	// whether a lost/late signature would trigger a re-assembly of the transaction ( and any transaction that come after it in the sequencer) or whether we could safely ask the assembly
	// to post hoc sign an assembly

	// The transaction input data that is the senders intent to perform the transaction for this ID,
	// MUST be retrieved from the local database. We cannot process it from the data that is received
	// over the wire from another node (otherwise that node could "tell us" to do something that no
	// application locally instructed us to do).
	localTx, err := s.resolveLocalTransaction(ctx, transactionID)
	if err != nil || localTx.Transaction.Domain != s.domainAPI.Domain().Name() || localTx.Transaction.To == nil || *localTx.Transaction.To != s.domainAPI.Address() {
		if err == nil {
			log.L(ctx).Errorf("assembleAndSign: transaction %s for invalid domain/address domain=%s (expected=%s) to=%s (expected=%s)",
				transactionID, localTx.Transaction.Domain, s.domainAPI.Domain().Name(), localTx.Transaction.To, s.domainAPI.Address())
		}
		err := i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrAssembleRequestInvalid, transactionID)
		return nil, err
	}
	transaction := &components.PrivateTransaction{
		ID:          transactionID,
		Domain:      localTx.Transaction.Domain,
		Address:     *localTx.Transaction.To,
		PreAssembly: preAssembly,
	}

	/*
	 * Assemble
	 */
	err = s.domainAPI.AssembleTransaction(domainContext, s.components.Persistence().NOTX(), transaction, localTx)
	if err != nil {
		log.L(ctx).Errorf("assembleAndSign: Error assembling transaction: %s", err)
		return nil, err
	}
	if transaction.PostAssembly == nil {
		log.L(ctx).Errorf("assembleForCoordinator: AssembleTransaction returned nil PostAssembly")
		// This is most likely a programming error in the domain
		err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "AssembleTransaction returned nil PostAssembly")
		log.L(ctx).Error(err)
		return nil, err
	}

	// Some validation that we are confident we can execute the given attestation plan
	for _, attRequest := range transaction.PostAssembly.AttestationPlan {
		switch attRequest.AttestationType {
		case prototk.AttestationType_ENDORSE:
		case prototk.AttestationType_SIGN:
		case prototk.AttestationType_GENERATE_PROOF:
			errorMessage := "AttestationType_GENERATE_PROOF is not implemented yet"
			log.L(ctx).Error(errorMessage)
			return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)

		default:
			errorMessage := fmt.Sprintf("Unsupported attestation type: %s", attRequest.AttestationType)
			log.L(ctx).Error(errorMessage)
			return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
		}
	}

	/*
	 * Sign
	 */
	for _, attRequest := range transaction.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_SIGN {
			for _, partyName := range attRequest.Parties {
				unqualifiedLookup, signerNode, err := pldtypes.PrivateIdentityLocator(partyName).Validate(ctx, s.nodeName, true)
				if err != nil {
					log.L(ctx).Errorf("Failed to validate identity locator for signing party %s: %s", partyName, err)
					return nil, err
				}
				if signerNode == s.nodeName {

					keyMgr := s.components.KeyManager()
					resolvedKey, err := keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, attRequest.Algorithm, attRequest.VerifierType)
					if err != nil {
						log.L(ctx).Errorf("Failed to resolve local signer for %s (algorithm=%s): %s", unqualifiedLookup, attRequest.Algorithm, err)
						return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerResolveError, unqualifiedLookup, attRequest.Algorithm)
					}

					signaturePayload, err := keyMgr.Sign(ctx, resolvedKey, attRequest.PayloadType, attRequest.Payload)
					if err != nil {
						log.L(ctx).Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", unqualifiedLookup, resolvedKey.Verifier.Verifier, attRequest.Algorithm, err)
						return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerSignError, unqualifiedLookup, resolvedKey.Verifier.Verifier, attRequest.Algorithm)
					}
					log.L(ctx).Debugf("payload: %x signed %x by %s (%s)", attRequest.Payload, signaturePayload, unqualifiedLookup, resolvedKey.Verifier.Verifier)

					transaction.PostAssembly.Signatures = append(transaction.PostAssembly.Signatures, &prototk.AttestationResult{
						Name:            attRequest.Name,
						AttestationType: attRequest.AttestationType,
						Verifier: &prototk.ResolvedVerifier{
							Lookup:       partyName,
							Algorithm:    attRequest.Algorithm,
							Verifier:     resolvedKey.Verifier.Verifier,
							VerifierType: attRequest.VerifierType,
						},
						Payload:     signaturePayload,
						PayloadType: &attRequest.PayloadType,
					})
				} else {
					log.L(ctx).Warnf("assembleAndSign: ignoring signature request of transaction %s for remote party %s ", transactionID, partyName)

				}
			}
		} else {
			log.L(ctx).Debugf("assembleAndSign: ignoring attestationType %s for fulfillment later", attRequest.AttestationType)
		}
	}

	if log.IsDebugEnabled() {
		stateIDs := ""
		for _, state := range transaction.PostAssembly.OutputStates {
			stateIDs += "," + state.ID.String()
		}
		log.L(ctx).Debugf("assembleAndSign: Assembled transaction %s : %s", transactionID, stateIDs)
	}
	return transaction.PostAssembly, nil
}
