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

package domainmgr

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

// only safe to use this sorter when you know all receipts have a non-nil on-chain
type txCompletionsOrdered []*components.TxCompletion

func (r txCompletionsOrdered) Len() int      { return len(r) }
func (r txCompletionsOrdered) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r txCompletionsOrdered) Less(i, j int) bool {
	return r[i].OnChain.Compare(&r[j].OnChain) < 0
}

type pscEventBatch struct {
	prototk.HandleEventBatchRequest
	psc *domainContract
}

func (dm *domainManager) registrationIndexer(ctx context.Context, dbTX persistence.DBTX, batch *blockindexer.EventDeliveryBatch) ([]*pldapi.EventWithData, txCompletionsOrdered, error) {

	var contracts []*PrivateSmartContract
	var txCompletions txCompletionsOrdered
	nonRegisterEvents := make([]*pldapi.EventWithData, 0, len(batch.Events))

	for _, ev := range batch.Events {
		processedEvent := false
		if ev.SoliditySignature == eventSolSig_PaladinRegisterSmartContract_V0 {

			// We only register against registries that we have indexed, so we should be able to find the domain for this address
			d, err := dm.getDomainByAddress(ctx, &ev.Address)
			if err != nil {
				log.L(ctx).Errorf("Registration event for unknown domain event: %s", pldtypes.JSONString(ev))
				continue
			}

			var parsedEvent event_PaladinRegisterSmartContract_V0
			parseErr := json.Unmarshal(ev.Data, &parsedEvent)
			if parseErr != nil {
				log.L(ctx).Errorf("Failed to parse domain event (%s): %s", parseErr, pldtypes.JSONString(ev))
			} else {
				processedEvent = true
				txID := parsedEvent.TXId.UUIDFirst16()
				contracts = append(contracts, &PrivateSmartContract{
					DeployTX:        txID,
					RegistryAddress: ev.Address,
					Address:         parsedEvent.Instance,
					ConfigBytes:     parsedEvent.Config,
				})
				// We don't know if the private transaction will match, but we need to pass it over
				// to the private TX manager within our DB transaction to allow it to check
				txCompletions = append(txCompletions, &components.TxCompletion{
					ReceiptInput: components.ReceiptInput{
						ReceiptType:   components.RT_Success,
						TransactionID: txID,
						Domain:        d.name,
						OnChain: pldtypes.OnChainLocation{
							Type:             pldtypes.OnChainEvent,
							TransactionHash:  ev.TransactionHash,
							BlockNumber:      ev.BlockNumber,
							TransactionIndex: ev.TransactionIndex,
							LogIndex:         ev.LogIndex,
							Source:           &ev.Address,
						},
						ContractAddress: &parsedEvent.Instance,
					},
					PSC: nil, // currently unset for deployments (rather than fluffing up the domainContract at this point)
				})
			}
		}
		if !processedEvent {
			nonRegisterEvents = append(nonRegisterEvents, ev)
		}
	}

	// Insert the batch of new contracts in this DB transaction (we do this before we call the domain to process the events)
	if len(contracts) > 0 {
		err := dbTX.DB().
			Table("private_smart_contracts").
			WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "address"}},
				DoNothing: true, // immutable
			}).
			Create(contracts).
			Error
		if err != nil {
			return nil, nil, err
		}
	}

	return nonRegisterEvents, txCompletions, nil
}

func (dm *domainManager) notifyTransactions(txCompletions txCompletionsOrdered) {
	for _, completion := range txCompletions {
		// Private transaction manager needs to know about these to update its in-memory state
		dm.privateTxManager.PrivateTransactionConfirmed(dm.bgCtx, completion)

		// We also provide a direct waiter that's used by the testbed
		inflight := dm.privateTxWaiter.GetInflight(completion.TransactionID)
		log.L(dm.bgCtx).Infof("Notifying for private deployment TransactionID %s (waiter=%t)", completion.TransactionID, inflight != nil)
		if inflight != nil {
			inflight.Complete(&completion.ReceiptInput)
		}
	}

}

func (d *domain) batchEventsByAddress(ctx context.Context, dbTX persistence.DBTX, batchID string, events []*pldapi.EventWithData) (map[pldtypes.EthAddress]*pscEventBatch, error) {

	batches := make(map[pldtypes.EthAddress]*pscEventBatch)

	for _, ev := range events {
		batch := batches[ev.Address]
		if batch == nil {
			// Note: hits will be cached, but events from unrecognized contracts will always
			// result in a cache miss and a database lookup
			// TODO: revisit if we should optimize this
			_, psc, err := d.dm.getSmartContractCached(ctx, dbTX, ev.Address)
			if err != nil {
				return nil, err
			}
			if psc == nil {
				log.L(ctx).Debugf("Discarding %s event for unregistered address %s", ev.SoliditySignature, ev.Address)
				continue
			}
			batch = &pscEventBatch{
				psc: psc,
				HandleEventBatchRequest: prototk.HandleEventBatchRequest{
					BatchId: batchID,
					ContractInfo: &prototk.ContractInfo{
						ContractAddress:    psc.Address().String(),
						ContractConfigJson: psc.config.ContractConfigJson,
					},
				},
			}
			batches[ev.Address] = batch
		}
		batch.Events = append(batch.Events, &prototk.OnChainEvent{
			Location: &prototk.OnChainEventLocation{
				TransactionHash:  ev.TransactionHash.String(),
				BlockNumber:      ev.BlockNumber,
				TransactionIndex: ev.TransactionIndex,
				LogIndex:         ev.LogIndex,
			},
			Signature:         ev.Signature.String(),
			SoliditySignature: ev.SoliditySignature,
			DataJson:          ev.Data.String(),
		})
	}

	return batches, nil
}

func (d *domain) handleEventBatch(ctx context.Context, dbTX persistence.DBTX, batch *blockindexer.EventDeliveryBatch) error {

	// First index any domain contract deployments
	nonDeployEvents, txCompletions, err := d.dm.registrationIndexer(ctx, dbTX, batch)
	if err != nil {
		return err
	}

	// Then divide remaining events by contract address and dispatch to the appropriate domain context
	batchesByAddress, err := d.batchEventsByAddress(ctx, dbTX, batch.BatchID.String(), nonDeployEvents)
	if err != nil {
		return err
	}
	for addr, batch := range batchesByAddress {
		res, err := d.handleEventBatchForContract(ctx, dbTX, addr, batch)
		if err != nil {
			return err
		}
		for _, txCompletionEvent := range res.TransactionsComplete {
			var txHash pldtypes.Bytes32
			txID, err := d.recoverTransactionID(ctx, txCompletionEvent.TransactionId)
			if err == nil {
				txHash, err = pldtypes.ParseBytes32(txCompletionEvent.Location.TransactionHash)
			}
			if err != nil {
				return err
			}
			log.L(ctx).Infof("Domain transaction completion: %s", txID)
			completion := &components.TxCompletion{
				PSC: batch.psc,
				ReceiptInput: components.ReceiptInput{
					TransactionID: *txID,
					Domain:        d.name,
					ReceiptType:   components.RT_Success,
					OnChain: pldtypes.OnChainLocation{
						Type:             pldtypes.OnChainEvent, // the on-chain confirmation is an event (even though it's a private transaction we're confirming)
						TransactionHash:  txHash,
						BlockNumber:      txCompletionEvent.Location.BlockNumber,
						TransactionIndex: txCompletionEvent.Location.TransactionIndex,
						LogIndex:         txCompletionEvent.Location.LogIndex,
						Source:           &addr,
					},
				},
			}
			txCompletions = append(txCompletions, completion)
		}
	}

	if len(txCompletions) > 0 {
		// Ensure we are sorted in block order, as the above processing extracted the array in two
		// phases (contract deployments, then transactions) so the list will be out of order.
		sort.Sort(txCompletions)

		receipts := make([]*components.ReceiptInput, len(txCompletions))
		for i, txc := range txCompletions {
			receipts[i] = &txc.ReceiptInput
		}

		// We have completions to hand to the TxManager to write as completions
		// Note we go directly to the TxManager (bypassing the private TX manager) during the database
		// transaction to write these receipts. We only write receipts for transactions where
		// we are the sender.
		//
		// Note separately below there is a notification to the private TX manager (after DB commit)
		// for ALL private transactions (not just those where we're the sender) as there
		// might be in-memory coordination activities that need to re-process now these
		// transactions have been finalized.
		err = d.dm.txManager.FinalizeTransactions(ctx, dbTX, receipts)
		if err != nil {
			return err
		}
	}

	dbTX.AddPostCommit(func(txCtx context.Context) {
		d.dm.notifyTransactions(txCompletions)
	})
	return nil
}

func (d *domain) recoverTransactionID(ctx context.Context, txIDString string) (*uuid.UUID, error) {
	txIDBytes, err := pldtypes.ParseBytes32Ctx(ctx, txIDString)
	if err != nil {
		return nil, err
	}
	txUUID := txIDBytes.UUIDFirst16()
	return &txUUID, nil
}

func (d *domain) handleEventBatchForContract(ctx context.Context, dbTX persistence.DBTX, addr pldtypes.EthAddress, batch *pscEventBatch) (*prototk.HandleEventBatchResponse, error) {

	// We have a domain context for queries, but we never flush it to DB - as the only updates
	// we allow in this function are those performed within our dbTX.
	c := d.newInFlightDomainRequest(dbTX, d.dm.stateStore.NewDomainContext(ctx, d, addr), false /* write enabled */)
	defer c.close()

	batch.StateQueryContext = c.id

	var res *prototk.HandleEventBatchResponse
	res, err := d.api.HandleEventBatch(ctx, &batch.HandleEventBatchRequest)
	if err != nil {
		return nil, err
	}

	stateSpends := make([]*pldapi.StateSpendRecord, len(res.SpentStates))
	for i, state := range res.SpentStates {
		txUUID, stateID, err := d.prepareIndexRecord(ctx, state.TransactionId, state.Id)
		if err != nil {
			return nil, err
		}
		stateSpends[i] = &pldapi.StateSpendRecord{DomainName: d.name, State: stateID, Transaction: txUUID}
	}

	stateReads := make([]*pldapi.StateReadRecord, len(res.ReadStates))
	for i, state := range res.ReadStates {
		txUUID, stateID, err := d.prepareIndexRecord(ctx, state.TransactionId, state.Id)
		if err != nil {
			return nil, err
		}
		stateReads[i] = &pldapi.StateReadRecord{DomainName: d.name, State: stateID, Transaction: txUUID}
	}

	stateConfirms := make([]*pldapi.StateConfirmRecord, len(res.ConfirmedStates))
	for i, state := range res.ConfirmedStates {
		txUUID, stateID, err := d.prepareIndexRecord(ctx, state.TransactionId, state.Id)
		if err != nil {
			return nil, err
		}
		stateConfirms[i] = &pldapi.StateConfirmRecord{DomainName: d.name, State: stateID, Transaction: txUUID}
	}

	stateInfoRecords := make([]*pldapi.StateInfoRecord, len(res.InfoStates))
	for i, state := range res.InfoStates {
		txUUID, stateID, err := d.prepareIndexRecord(ctx, state.TransactionId, state.Id)
		if err != nil {
			return nil, err
		}
		stateInfoRecords[i] = &pldapi.StateInfoRecord{DomainName: d.name, State: stateID, Transaction: txUUID}
	}

	newStates := make([]*components.StateUpsertOutsideContext, 0)
	for _, state := range res.NewStates {
		var id pldtypes.HexBytes
		if state.Id != nil {
			id, err = pldtypes.ParseHexBytes(ctx, *state.Id)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidStateID, *state.Id)
			}
		}
		txUUID, err := d.recoverTransactionID(ctx, state.TransactionId)
		if err != nil {
			return nil, err
		}
		schemaID, err := pldtypes.ParseBytes32(state.SchemaId)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidSchemaID, state.SchemaId)
		}
		newStates = append(newStates, &components.StateUpsertOutsideContext{
			ID:              id,
			SchemaID:        schemaID,
			ContractAddress: &addr,
			Data:            pldtypes.RawJSON(state.StateDataJson),
		})

		// These have implicit confirmations
		stateConfirms = append(stateConfirms, &pldapi.StateConfirmRecord{DomainName: d.name, State: id, Transaction: *txUUID})
	}

	// Write any new states first
	if len(newStates) > 0 {
		// These states are trusted as they come from the domain on our local node (no need to go back round VerifyStateHashes for customer hash functions)
		_, err = d.dm.stateStore.WritePreVerifiedStates(ctx, dbTX, d.name, newStates)
		if err != nil {
			return nil, err
		}
	}

	// Then any finalizations of those states
	if len(stateSpends) > 0 || len(stateReads) > 0 || len(stateConfirms) > 0 || len(stateInfoRecords) > 0 {
		if err := d.dm.stateStore.WriteStateFinalizations(ctx, dbTX, stateSpends, stateReads, stateConfirms, stateInfoRecords); err != nil {
			return nil, err
		}
	}
	return res, err
}

func (d *domain) prepareIndexRecord(ctx context.Context, txIDStr, stateIDStr string) (uuid.UUID, pldtypes.HexBytes, error) {
	txUUID, err := d.recoverTransactionID(ctx, txIDStr)
	if err != nil {
		return uuid.UUID{}, nil, err
	}
	stateID, err := pldtypes.ParseHexBytes(ctx, stateIDStr)
	if err != nil {
		return uuid.UUID{}, nil, i18n.NewError(ctx, msgs.MsgDomainInvalidStateID, stateIDStr)
	}
	return *txUUID, stateID, nil
}
