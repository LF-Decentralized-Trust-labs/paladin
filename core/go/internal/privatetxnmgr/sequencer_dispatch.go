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
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/syncpoints"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
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
	localStateDistributions := make([]*components.StateDistributionWithData, 0)
	preparedTxnDistributions := make([]*components.PreparedTransactionWithRefs, 0)

	for signingAddress, transactionFlows := range dispatchableTransactions {
		log.L(ctx).Debugf("DispatchTransactions: %d transactions for signingAddress %s", len(transactionFlows), signingAddress)

		publicTransactionsToSend := make([]*components.PrivateTransaction, 0, len(transactionFlows))

		sequence := &syncpoints.PublicDispatch{}

		for _, transactionFlow := range transactionFlows {
			// prepare all transactions

			// If we don't have a signing key for the TX at this point, we use our randomly assigned one
			// TODO: Rotation
			preparedTransaction, err := transactionFlow.PrepareTransaction(ctx, s.defaultSigner)
			if err != nil {
				log.L(ctx).Errorf("Error preparing transaction: %s", err)
				//TODO this is a really bad time to be getting an error.  need to think carefully about how to handle this
				return err
			}
			hasPublicTransaction := preparedTransaction.PreparedPublicTransaction != nil
			hasPrivateTransaction := preparedTransaction.PreparedPrivateTransaction != nil
			switch {
			case preparedTransaction.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPublicTransaction && !hasPrivateTransaction:
				log.L(ctx).Infof("Result of transaction %s is a public transaction (gas=%d)", preparedTransaction.ID, *preparedTransaction.PreparedPublicTransaction.Gas)
				publicTransactionsToSend = append(publicTransactionsToSend, preparedTransaction)
				sequence.PrivateTransactionDispatches = append(sequence.PrivateTransactionDispatches, &syncpoints.DispatchPersisted{
					PrivateTransactionID: transactionFlow.ID(ctx).String(),
				})
			case preparedTransaction.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPrivateTransaction && !hasPublicTransaction:
				log.L(ctx).Infof("Result of transaction %s is a chained private transaction", preparedTransaction.ID)
				tx := transactionFlow.PrivateTransaction()
				senderLocator := tx.PreAssembly.TransactionSpecification.From
				validatedPrivateTx, err := s.components.TxManager().
					PrepareChainedPrivateTransaction(ctx, s.components.Persistence().NOTX(),
						senderLocator, tx.ID, tx.Domain, &s.contractAddress,
						preparedTransaction.PreparedPrivateTransaction, pldapi.SubmitModeAuto)
				if err != nil {
					log.L(ctx).Errorf("Error preparing transaction %s: %s", preparedTransaction.ID, err)
					// TODO: this is just an error situation for one transaction - this function is a batch function
					return err
				}
				dispatchBatch.PrivateDispatches = append(dispatchBatch.PrivateDispatches, validatedPrivateTx)
			case preparedTransaction.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION && (hasPublicTransaction || hasPrivateTransaction):
				log.L(ctx).Infof("Result of transaction %s is a prepared transaction public=%t private=%t", preparedTransaction.ID, hasPublicTransaction, hasPrivateTransaction)
				preparedTransactionWithRefs := mapPreparedTransaction(preparedTransaction)
				dispatchBatch.PreparedTransactions = append(dispatchBatch.PreparedTransactions, preparedTransactionWithRefs)

				// The prepared transaction needs to end up on the node that is able to submit it.
				preparedTxnDistributions = append(preparedTxnDistributions, preparedTransactionWithRefs)

			default:
				err = i18n.NewError(ctx, msgs.MsgPrivateTxMgrInvalidPrepareOutcome, preparedTransaction.ID, preparedTransaction.Intent, hasPublicTransaction, hasPrivateTransaction)
				log.L(ctx).Errorf("Error preparing transaction %s: %s", preparedTransaction.ID, err)
				// TODO: this is just an error situation for one transaction - this function is a batch function
				return err
			}

			sds, err := transactionFlow.GetStateDistributions(ctx)
			if err != nil {
				return err
			}
			for _, sd := range sds.Remote {
				stateDistributions = append(stateDistributions, &sd.StateDistribution)
			}
			localStateDistributions = append(localStateDistributions, sds.Local...)
		}

		//Now we have the payloads, we can prepare the submission
		publicTransactionEngine := s.components.PublicTxManager()

		// we may or may not have any transactions to send depending on the submit mode
		if len(publicTransactionsToSend) == 0 {
			log.L(ctx).Debugf("No public transactions to send for signing address %s", signingAddress)
		} else {

			signers := make([]string, len(publicTransactionsToSend))
			for i, pt := range publicTransactionsToSend {
				unqualifiedSigner, err := pldtypes.PrivateIdentityLocator(pt.Signer).Identity(ctx)
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
						PublicTxOptions: pt.PreparedPublicTransaction.PublicTxOptions,
					},
				}

				// TODO: This aligning with submission in public Tx manage
				data, err := pt.PreparedPublicTransaction.ABI[0].EncodeCallDataJSONCtx(ctx, pt.PreparedPublicTransaction.Data)
				if err != nil {
					return err
				}
				publicTXs[i].Data = pldtypes.HexBytes(data)

				err = publicTransactionEngine.ValidateTransaction(ctx, s.components.Persistence().NOTX(), publicTXs[i])
				if err != nil {
					return i18n.WrapError(ctx, err, msgs.MsgPrivTxMgrPublicTxFail)
				}
			}
			sequence.PublicTxs = publicTXs
			dispatchBatch.PublicDispatches = append(dispatchBatch.PublicDispatches, sequence)

		}
	}

	dCtx := s.coordinatorDomainContext

	// Determine if there are any local nullifiers that need to be built and put into the domain context
	// before we persist the dispatch batch
	localNullifiers, err := s.privateTxManager.BuildNullifiers(ctx, localStateDistributions)
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
	for signingAddress, sequence := range dispatchableTransactions {
		for _, transactionFlow := range sequence {
			s.publisher.PublishTransactionDispatchedEvent(ctx, transactionFlow.ID(ctx).String(), uint64(0) /*TODO*/, signingAddress)
		}
	}
	for _, preparedTransaction := range dispatchBatch.PreparedTransactions {
		s.publisher.PublishTransactionPreparedEvent(ctx, preparedTransaction.ID.String())
	}

	// We also need to trigger ourselves for any private TX we chained
	for _, dispatch := range dispatchBatch.PrivateDispatches {
		if err := s.privateTxManager.HandleNewTx(ctx, s.components.Persistence().NOTX(), dispatch.NewTransaction); err != nil {
			log.L(ctx).Errorf("Sequencer failed to notify private TX manager for chained transaction")
		}
	}

	return nil

}

func mapPreparedTransaction(tx *components.PrivateTransaction) *components.PreparedTransactionWithRefs {
	pt := &components.PreparedTransactionWithRefs{
		PreparedTransactionBase: &pldapi.PreparedTransactionBase{
			ID:       tx.ID,
			Domain:   tx.Domain,
			To:       &tx.Address,
			Metadata: tx.PreparedMetadata,
		},
	}
	for _, s := range tx.PostAssembly.InputStates {
		pt.StateRefs.Spent = append(pt.StateRefs.Spent, s.ID)
	}
	for _, s := range tx.PostAssembly.ReadStates {
		pt.StateRefs.Read = append(pt.StateRefs.Read, s.ID)
	}
	for _, s := range tx.PostAssembly.OutputStates {
		pt.StateRefs.Confirmed = append(pt.StateRefs.Confirmed, s.ID)
	}
	for _, s := range tx.PostAssembly.InfoStates {
		pt.StateRefs.Info = append(pt.StateRefs.Info, s.ID)
	}
	if tx.PreparedPublicTransaction != nil {
		pt.Transaction = *tx.PreparedPublicTransaction
	} else {
		pt.Transaction = *tx.PreparedPrivateTransaction
	}
	return pt

}
