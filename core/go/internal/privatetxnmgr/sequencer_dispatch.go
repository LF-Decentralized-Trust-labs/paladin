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
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/preparedtxdistribution"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/syncpoints"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// synchronously prepare and dispatch all given transactions to their associated signing address / or deliver prepared transaction to their custodian
func (s *Sequencer) DispatchTransactions(ctx context.Context, dispatchableTransactions ptmgrtypes.DispatchableTransactions) error {
	log.L(ctx).Debug("DispatchTransactions")
	//prepare all transactions then dispatch them

	// array of sequences with space for one per signing address
	// dispatchableTransactions is a map of signing address to transaction IDs so we can group by signing address
	dispatchBatch := &syncpoints.DispatchBatch{
		PublicDispatches: make([]*syncpoints.PublicDispatch, 0, len(dispatchableTransactions)),
	}

	stateDistributions := make([]*components.StateDistribution, 0)
	localStateDistributions := make([]*components.StateDistribution, 0)
	preparedTxnDistributions := make([]*preparedtxdistribution.PreparedTxnDistribution, 0)

	completed := false // and include whether we committed the DB transaction or not
	for signingAddress, transactionIDs := range dispatchableTransactions {
		log.L(ctx).Debugf("DispatchTransactions: %d transactions for signingAddress %s", len(transactionIDs), signingAddress)

		publicTransactionsToSend := make([]*components.PrivateTransaction, 0, len(transactionIDs))

		sequence := &syncpoints.PublicDispatch{}

		for _, transactionID := range transactionIDs {
			// prepare all transactions for the given transaction IDs

			txProcessor := s.getTransactionProcessor(transactionID)
			if txProcessor == nil {
				//TODO currently assume that all the transactions are in flight and in memory
				// need to reload from database if not in memory
				panic("Transaction not found")
			}

			// If we don't have a signing key for the TX at this point, we use our randomly assigned one
			// TODO: Rotation
			preparedTransaction, err := txProcessor.PrepareTransaction(ctx, s.defaultSigner)
			if err != nil {
				log.L(ctx).Errorf("Error preparing transaction: %s", err)
				//TODO this is a really bad time to be getting an error.  need to think carefully about how to handle this
				return err
			}
			hasPublicTransaction := preparedTransaction.PreparedPublicTransaction != nil
			hasPrivateTransaction := preparedTransaction.PreparedPrivateTransaction != nil
			switch {
			case preparedTransaction.Inputs.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPublicTransaction && !hasPrivateTransaction:
				log.L(ctx).Infof("Result of transaction %s is a prepared public transaction", preparedTransaction.ID)
				publicTransactionsToSend = append(publicTransactionsToSend, preparedTransaction)
				sequence.PrivateTransactionDispatches = append(sequence.PrivateTransactionDispatches, &syncpoints.DispatchPersisted{
					PrivateTransactionID: transactionID,
				})
			case preparedTransaction.Inputs.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPrivateTransaction && !hasPublicTransaction:
				log.L(ctx).Infof("Result of transaction %s is a chained private transaction", preparedTransaction.ID)
				validatedPrivateTx, err := s.components.TxManager().PrepareInternalPrivateTransaction(ctx, s.components.Persistence().DB(), preparedTransaction.PreparedPrivateTransaction, pldapi.SubmitModeAuto)
				if err != nil {
					log.L(ctx).Errorf("Error preparing transaction %s: %s", preparedTransaction.ID, err)
					// TODO: this is just an error situation for one transaction - this function is a batch function
					return err
				}
				dispatchBatch.PrivateDispatches = append(dispatchBatch.PrivateDispatches, validatedPrivateTx)
			case preparedTransaction.Inputs.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION && (hasPublicTransaction || hasPrivateTransaction):
				log.L(ctx).Infof("Result of transaction %s is a prepared transaction public=%t private=%t", preparedTransaction.ID, hasPublicTransaction, hasPrivateTransaction)
				preparedTransactionWithRefs := mapPreparedTransaction(preparedTransaction)
				dispatchBatch.PreparedTransactions = append(dispatchBatch.PreparedTransactions, preparedTransactionWithRefs)
				preparedTransactionJSON, err := json.Marshal(preparedTransactionWithRefs)
				if err != nil {
					log.L(ctx).Errorf("Error marshalling prepared transaction: %s", err)
					// TODO: this is just an error situation for one transaction - this function is a batch function
					return err
				}
				preparedTxnDistributions = append(preparedTxnDistributions, &preparedtxdistribution.PreparedTxnDistribution{
					ID:                      uuid.New().String(),
					PreparedTxnID:           preparedTransactionWithRefs.ID.String(),
					IdentityLocator:         preparedTransactionWithRefs.Sender,
					Domain:                  preparedTransactionWithRefs.Domain,
					ContractAddress:         preparedTransactionWithRefs.To.String(),
					PreparedTransactionJSON: preparedTransactionJSON,
				})

			default:
				err = i18n.NewError(ctx, msgs.MsgPrivateTxMgrInvalidPrepareOutcome, preparedTransaction.ID, preparedTransaction.Inputs.Intent, hasPublicTransaction, hasPrivateTransaction)
				log.L(ctx).Errorf("Error preparing transaction %s: %s", preparedTransaction.ID, err)
				// TODO: this is just an error situation for one transaction - this function is a batch function
				return err
			}

			sds, err := txProcessor.GetStateDistributions(ctx)
			if err != nil {
				return err
			}
			stateDistributions = append(stateDistributions, sds.Remote...)
			localStateDistributions = append(localStateDistributions, sds.Local...)
		}

		preparedTransactionPayloads := make([]*pldapi.TransactionInput, len(publicTransactionsToSend))

		for j, preparedTransaction := range publicTransactionsToSend {
			preparedTransactionPayloads[j] = preparedTransaction.PreparedPublicTransaction
		}

		//Now we have the payloads, we can prepare the submission
		publicTransactionEngine := s.components.PublicTxManager()

		signers := make([]string, len(publicTransactionsToSend))
		for i, pt := range publicTransactionsToSend {
			unqualifiedSigner, err := tktypes.PrivateIdentityLocator(pt.Signer).Identity(ctx)
			if err != nil {
				errorMessage := fmt.Sprintf("failed to parse lookup key for signer %s : %s", pt.Signer, err)
				log.L(ctx).Error(errorMessage)
				return i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, errorMessage)
			}

			signers[i] = unqualifiedSigner
		}
		keyMgr := s.components.KeyManager()
		resolvedAddrs, err := keyMgr.ResolveEthAddressBatchNewDatabaseTX(ctx, signers)
		if err != nil {
			return err
		}

		publicTXs := make([]*components.PublicTxSubmission, len(publicTransactionsToSend))
		for i, pt := range publicTransactionsToSend {
			log.L(ctx).Debugf("DispatchTransactions: creating PublicTxSubmission from %s", pt.Signer)
			publicTXs[i] = &components.PublicTxSubmission{
				Bindings: []*components.PaladinTXReference{{TransactionID: pt.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
				PublicTxInput: pldapi.PublicTxInput{
					From:            resolvedAddrs[i],
					To:              &s.contractAddress,
					PublicTxOptions: pldapi.PublicTxOptions{}, // TODO: Consider propagation from paladin transaction input
				},
			}

			// TODO: This aligning with submission in public Tx manage
			data, err := pt.PreparedPublicTransaction.ABI[0].EncodeCallDataJSONCtx(ctx, pt.PreparedPublicTransaction.Data)
			if err != nil {
				return err
			}
			publicTXs[i].Data = tktypes.HexBytes(data)
		}
		pubBatch, err := publicTransactionEngine.PrepareSubmissionBatch(ctx, publicTXs)
		if err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgPrivTxMgrPublicTxFail)
		}
		// Must make sure from this point we return the nonces
		sequence.PublicTxBatch = pubBatch
		defer func() {
			pubBatch.Completed(ctx, completed)
		}()
		if len(pubBatch.Rejected()) > 0 {
			// We do not handle partial success - roll everything back
			return i18n.WrapError(ctx, pubBatch.Rejected()[0].RejectedError(), msgs.MsgPrivTxMgrPublicTxFail)
		}

		dispatchBatch.PublicDispatches = append(dispatchBatch.PublicDispatches, sequence)
	}

	dCtx := s.coordinatorDomainContext

	// Determine if there are any local nullifiers that need to be built and put into the domain context
	// before we persist the dispatch batch
	localNullifiers, err := s.stateDistributer.BuildNullifiers(ctx, localStateDistributions)
	if err == nil && len(localNullifiers) > 0 {
		err = dCtx.UpsertNullifiers(localNullifiers...)
	}
	if err != nil {
		return err
	}

	err = s.syncPoints.PersistDispatchBatch(dCtx, s.contractAddress, dispatchBatch, stateDistributions, preparedTxnDistributions)
	if err != nil {
		log.L(ctx).Errorf("Error persisting batch: %s", err)
		return err
	}
	completed = true
	for signingAddress, sequence := range dispatchableTransactions {
		for _, privateTransactionID := range sequence {
			s.publisher.PublishTransactionDispatchedEvent(ctx, privateTransactionID, uint64(0) /*TODO*/, signingAddress)
		}
	}
	//now that the DB write has been persisted, we can trigger the in-memory distribution of the prepared transactions and states
	s.stateDistributer.DistributeStates(ctx, stateDistributions)

	s.preparedTransactionDistributer.DistributePreparedTransactions(ctx, preparedTxnDistributions)

	// We also need to trigger ourselves for any private TX we chained
	for _, tx := range dispatchBatch.PrivateDispatches {
		if err := s.privateTxManager.HandleNewTx(ctx, tx); err != nil {
			log.L(ctx).Errorf("Sequencer failed to notify private TX manager for chained transaction")
		}
	}

	return nil

}

func mapPreparedTransaction(tx *components.PrivateTransaction) *components.PrepareTransactionWithRefs {
	pt := &components.PrepareTransactionWithRefs{
		ID:       tx.ID,
		Domain:   tx.Inputs.Domain,
		To:       &tx.Inputs.To,
		Metadata: tx.PreparedMetadata,
		Sender:   tx.Inputs.From,
	}
	for _, s := range tx.PostAssembly.InputStates {
		pt.States.Spent = append(pt.States.Spent, s.ID)
	}
	for _, s := range tx.PostAssembly.ReadStates {
		pt.States.Read = append(pt.States.Read, s.ID)
	}
	for _, s := range tx.PostAssembly.OutputStates {
		pt.States.Confirmed = append(pt.States.Confirmed, s.ID)
	}
	for _, s := range tx.PostAssembly.InfoStates {
		pt.States.Info = append(pt.States.Info, s.ID)
	}
	if tx.PreparedPublicTransaction != nil {
		pt.Transaction = tx.PreparedPublicTransaction
	} else {
		pt.Transaction = tx.PreparedPrivateTransaction
	}
	return pt

}
