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

package common

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
	mock "github.com/stretchr/testify/mock"
)

// This is the subset of the StateDistributer interface from "github.com/LF-Decentralized-Trust-labs/paladin/core/internal/statedistribution"
// Here we define the subset that we rely on in this package

type Hooks interface {
	GetBlockHeight() int64
	GetNodeName() string
}

type EngineIntegration interface {
	// WriteLockStatesForTransaction is a method that writes a lock to the states for a transaction
	WriteLockStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error

	ResetTransactions(ctx context.Context, transactionID uuid.UUID)

	GetStateLocks(ctx context.Context) ([]byte, error)

	GetBlockHeight(ctx context.Context) (int64, error)

	//Assemble and sign is a single, synchronous operation that assembles a transaction using the domain smart contract
	// and then fulfills any signature requests in the attestation plan
	// there would be a benefit in separating this out to `assemble` and `sign` steps and to make then asynchronous
	// In particular, signing could involved collecting multiple signatures and the signing module may be remote
	// and unknown latency could incur back pressure to the state machines input channel
	//However, to fully reap the benefits of tolerating latency in this phase, we would need to revisit the algorithm that currently
	// assumes that the coordinator will not assemble any transactions while it is waiting for a signed post assembly for one transaction
	// . e.g. it might make sense to split out the assembling and gatheringSignatures into separate states on the coordinator side so that it can
	// single thread assembly and still tolerate latency in the signing phase.
	AssembleAndSign(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) (*components.TransactionPostAssembly, error)
}

func NewEngineIntegration(ctx context.Context, allComponents components.AllComponents, nodeName string, domainSmartContract components.DomainSmartContract, domainContext components.DomainContext, delegateDomainContext components.DomainContext, hooks Hooks) EngineIntegration {
	return &engineIntegration{
		environment:           hooks,
		components:            allComponents,
		domainSmartContract:   domainSmartContract,
		domainContext:         domainContext,
		delegateDomainContext: delegateDomainContext,
		nodeName:              nodeName,
	}

}

// mockery doesn't really work well when used in code outside of _test.go files and we need these test utils to be usable by other packages so can't put them into _test.go files
// so we have to define the mock manually
type FakeEngineIntegrationForTesting struct {
	mock.Mock
}

func (f *FakeEngineIntegrationForTesting) WriteLockStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error {
	return nil
}

func (f *FakeEngineIntegrationForTesting) GetStateLocks(ctx context.Context) ([]byte, error) {
	return nil, nil
}

func (f *FakeEngineIntegrationForTesting) GetBlockHeight(ctx context.Context) (int64, error) {
	return 0, nil
}

func (f *FakeEngineIntegrationForTesting) ResetTransactions(ctx context.Context, transactionID uuid.UUID) {
}

func (f *FakeEngineIntegrationForTesting) AssembleAndSign(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) (*components.TransactionPostAssembly, error) {
	return f.Called(ctx, transactionID, preAssembly, stateLocksJSON, blockHeight).Get(0).(*components.TransactionPostAssembly), nil
}

type engineIntegration struct {
	components            components.AllComponents
	domainSmartContract   components.DomainSmartContract
	domainContext         components.DomainContext
	delegateDomainContext components.DomainContext
	nodeName              string
	environment           Hooks
}

func (e *engineIntegration) WriteLockStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error {

	// Write output states
	if txn.PostAssembly.OutputStatesPotential != nil && txn.PostAssembly.OutputStates == nil {
		readTX := e.components.Persistence().NOTX() // no DB transaction required here for the reads from the DB (writes happen on syncpoint flusher)
		err := e.domainSmartContract.WritePotentialStates(e.domainContext, readTX, txn)
		if err != nil {
			//Any error from WritePotentialStates is likely to be caused by an invalid init or assemble of the transaction
			// which is most likely a programming error in the domain or the domain manager or the sequencer
			errorMessage := fmt.Sprintf("Failed to write potential states: %s", err)
			log.L(ctx).Error(errorMessage)
			return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
		} else {
			log.L(ctx).Debugf("Potential states written %s", e.domainContext.Info().ID)
		}
	}

	// Lock input states
	if len(txn.PostAssembly.InputStates) > 0 {
		readTX := e.components.Persistence().NOTX() // no DB transaction required here for the reads from the DB (writes happen on syncpoint flusher)
		err := e.domainSmartContract.LockStates(e.domainContext, readTX, txn)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to lock states: %s", err)
			log.L(ctx).Error(errorMessage)
			return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)

		} else {
			log.L(ctx).Tracef("Input states locked %s: %s", e.domainContext.Info().ID, txn.PostAssembly.InputStates[0].ID)
		}
	}

	return nil

}

func (e *engineIntegration) GetStateLocks(ctx context.Context) ([]byte, error) {
	log.L(ctx).Debugf("GetStateLocks: Exporting snapshot for domain context %s", e.domainContext.Info().ID)
	return e.domainContext.ExportSnapshot()
}

func (e *engineIntegration) GetBlockHeight(ctx context.Context) (int64, error) {
	return e.environment.GetBlockHeight(), nil
}

func (e *engineIntegration) ResetTransactions(ctx context.Context, transactionID uuid.UUID) {
	log.L(ctx).Debugf("ResetTransactions: resetting domain context for transaction %s", transactionID.String())
	e.domainContext.ResetTransactions(transactionID)
}

// assemble a transaction that we are not coordinating, using the provided state locks
// all errors are assumed to be transient and the request should be retried
// if the domain as deemed the request as invalid then it will communicate the `revert` directive via the AssembleTransactionResponse_REVERT result without any error
func (e *engineIntegration) AssembleAndSign(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) (*components.TransactionPostAssembly, error) {

	log.L(ctx).Debugf("Assembling transaction %s. Resetting domain context with state locks from the coordinator which assumes a block height of %d compared with local blockHeight of %d", transactionID, blockHeight, e.environment.GetBlockHeight())
	//If our block height is behind the coordinator, there are some states that would otherwise be available to us but we wont see
	// if our block height is ahead of the coordinator, there is a small chance that we we assemble a transaction that the coordinator will not be able to
	// endorse yet but it is better to wait around on the endorsement flow than to wait around on the assemble flow which is single threaded per domain

	err := e.delegateDomainContext.ImportSnapshot(stateLocksJSON)
	if err != nil {
		log.L(ctx).Errorf("error importing state locks: %s", err)
		return nil, err
	}

	// Reset to an empty list in case we've already assembled before
	preAssembly.Verifiers = make([]*prototk.ResolvedVerifier, 0)

	for _, v := range preAssembly.RequiredVerifiers {
		log.L(ctx).Debugf("resolving required verifier %s", v.Lookup)
		verifier, err := e.components.IdentityResolver().ResolveVerifier(
			ctx,
			v.Lookup,
			v.Algorithm,
			v.VerifierType,
		)
		if err != nil {
			log.L(ctx).Errorf("error resolving verifier %s: %s", v.Lookup, err)
			return nil, err
		}
		preAssembly.Verifiers = append(preAssembly.Verifiers, &prototk.ResolvedVerifier{
			Lookup:       v.Lookup,
			Algorithm:    v.Algorithm,
			VerifierType: v.VerifierType,
			Verifier:     verifier,
		})
	}

	postAssembly, err := e.assembleAndSign(ctx, transactionID, preAssembly, e.delegateDomainContext)

	if err != nil {
		log.L(ctx).Errorf("error assembling and signing transaction: %s", err)
		return nil, err
	}

	return postAssembly, nil
}

func (e *engineIntegration) resolveLocalTransaction(ctx context.Context, transactionID uuid.UUID) (*components.ResolvedTransaction, error) {
	locallyResolvedTx, err := e.components.TxManager().GetResolvedTransactionByID(ctx, transactionID)
	if err == nil && locallyResolvedTx == nil {
		err = i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrAssembleTxnNotFound, transactionID)
	}
	return locallyResolvedTx, err
}

func (e *engineIntegration) assembleAndSign(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, domainContext components.DomainContext) (*components.TransactionPostAssembly, error) {
	//Assembles the transaction and synchronously fulfills any local signature attestation requests
	// Given that the coordinator is single threading calls to assemble, there may be benefits to performance if we were to fulfill the signature request async
	// but that would introduce levels of complexity that may not be justified so this is open as a potential for future optimization where we would need to think about
	// whether a lost/late signature would trigger a re-assembly of the transaction ( and any transaction that come after it in the sequencer) or whether we could safely ask the assembly
	// to post hoc sign an assembly

	// The transaction input data that is the sender's intent to perform the transaction for this ID,
	// MUST be retrieved from the local database. We cannot process it from the data that is received
	// over the wire from another node (otherwise that node could "tell us" to do something that no
	// application locally instructed us to do).
	// TODO is this still necessary? We are not receiving the PreAssembly from the coordinator. We only get it from the originator's state machine which was initialized from reading the DB
	// there may be some weird cases where we get a assemble request and we have somehow swapped out the memory record of the preassembly since delegating but that is an edge case and not what we should optimize for

	localTx, err := e.resolveLocalTransaction(ctx, transactionID)
	if err != nil || localTx.Transaction.Domain != e.domainSmartContract.Domain().Name() || localTx.Transaction.To == nil || *localTx.Transaction.To != e.domainSmartContract.Address() {
		if err == nil {
			log.L(ctx).Errorf("transaction %s for invalid domain/address domain=%s (expected=%s) to=%s (expected=%s)",
				transactionID, localTx.Transaction.Domain, e.domainSmartContract.Domain().Name(), localTx.Transaction.To, e.domainSmartContract.Address())
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
	log.L(ctx).Debugf("Assembling transaction: %+v", transaction)
	err = e.domainSmartContract.AssembleTransaction(domainContext, e.components.Persistence().NOTX(), transaction, localTx)
	if err != nil {
		log.L(ctx).Errorf("error assembling transaction: %s", err)
		return nil, err
	}
	if transaction.PostAssembly == nil {
		// This is most likely a programming error in the domain
		err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "AssembleTransaction returned nil PostAssembly")
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
				log.L(ctx).Debugf("validating identity locator for signing party %s", partyName)
				unqualifiedLookup, signerNode, err := pldtypes.PrivateIdentityLocator(partyName).Validate(ctx, e.nodeName, true)
				if err != nil {
					log.L(ctx).Errorf("failed to validate identity locator for signing party %s: %s", partyName, err)
					return nil, err
				}
				if signerNode == e.nodeName {
					log.L(ctx).Debugf("we are in the signing parties list - signing")

					keyMgr := e.components.KeyManager()
					resolvedKey, err := keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, attRequest.Algorithm, attRequest.VerifierType)
					if err != nil {
						log.L(ctx).Errorf("failed to resolve local signer for %s (algorithm=%s): %s", unqualifiedLookup, attRequest.Algorithm, err)
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
					log.L(ctx).Warnf("ignoring sign request of transaction %s for remote party %s ", transactionID, partyName)
				}
			}
		} else {
			log.L(ctx).Debugf("ignoring attestationType %s for fulfillment later", attRequest.AttestationType)
		}
	}

	if log.IsDebugEnabled() {
		stateIDs := ""
		for _, state := range transaction.PostAssembly.OutputStates {
			stateIDs += "," + state.ID.String()
		}
		log.L(ctx).Debugf("Assembled transaction %s, state IDs: %s", transactionID, stateIDs)
	}
	return transaction.PostAssembly, nil
}
