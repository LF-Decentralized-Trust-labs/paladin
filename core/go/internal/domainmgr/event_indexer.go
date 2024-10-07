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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// only safe to use this sorter when you know all receipts have a non-nil on-chain
type receiptsByOnChainOrder []*components.ReceiptInput

func (r receiptsByOnChainOrder) Len() int           { return len(r) }
func (r receiptsByOnChainOrder) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r receiptsByOnChainOrder) Less(i, j int) bool { return r[i].OnChain.Compare(&r[j].OnChain) < 0 }

func (dm *domainManager) registrationIndexer(ctx context.Context, dbTX *gorm.DB, batch *blockindexer.EventDeliveryBatch) ([]*blockindexer.EventWithData, receiptsByOnChainOrder, error) {

	var contracts []*PrivateSmartContract
	var txCompletions receiptsByOnChainOrder
	unprocessedEvents := make([]*blockindexer.EventWithData, 0, len(batch.Events))

	for _, ev := range batch.Events {
		processedEvent := false
		if ev.SoliditySignature == eventSolSig_PaladinRegisterSmartContract_V0 {
			var parsedEvent event_PaladinRegisterSmartContract_V0
			parseErr := json.Unmarshal(ev.Data, &parsedEvent)
			if parseErr != nil {
				log.L(ctx).Errorf("Failed to parse domain event (%s): %s", parseErr, tktypes.JSONString(ev))
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
				txCompletions = append(txCompletions, &components.ReceiptInput{
					ReceiptType:   components.RT_Success,
					TransactionID: txID,
					OnChain: tktypes.OnChainLocation{
						Type:             tktypes.OnChainEvent,
						TransactionHash:  ev.TransactionHash,
						BlockNumber:      ev.BlockNumber,
						TransactionIndex: ev.TransactionIndex,
						LogIndex:         ev.LogIndex,
						Source:           &ev.Address,
					},
					ContractAddress: &parsedEvent.Instance,
				})
			}
		}
		if !processedEvent {
			unprocessedEvents = append(unprocessedEvents, ev)
		}
	}

	// Insert the batch of new contracts in this DB transaction (we do this before we call the domain to process the events)
	if len(contracts) > 0 {
		err := dbTX.
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

	return unprocessedEvents, txCompletions, nil
}

func (dm *domainManager) notifyTransactions(txCompletions receiptsByOnChainOrder) {
	for _, receipt := range txCompletions {
		inflight := dm.privateTxWaiter.GetInflight(receipt.TransactionID)
		if inflight != nil {
			inflight.Complete(receipt)
		}
	}
}

func (d *domain) batchEventsByAddress(ctx context.Context, tx *gorm.DB, batchID string, events []*blockindexer.EventWithData) (map[tktypes.EthAddress]*prototk.HandleEventBatchRequest, error) {

	batches := make(map[tktypes.EthAddress]*prototk.HandleEventBatchRequest)

	for _, ev := range events {
		batch := batches[ev.Address]
		if batch == nil {
			// Note: hits will be cached, but events from unrecognized contracts will always
			// result in a cache miss and a database lookup
			// TODO: revisit if we should optimize this
			psc, err := d.dm.getSmartContractCached(ctx, tx, ev.Address)
			if err != nil {
				return nil, err
			}
			if psc == nil {
				log.L(ctx).Debugf("Discarding %s event for unregistered address %s", ev.SoliditySignature, ev.Address)
				continue
			}
			batch = &prototk.HandleEventBatchRequest{
				BatchId: batchID,
				ContractInfo: &prototk.ContractInfo{
					ContractAddress: psc.Address().String(),
					ContractConfig:  psc.ConfigBytes(),
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

func (d *domain) handleEventBatch(ctx context.Context, dbTX *gorm.DB, batch *blockindexer.EventDeliveryBatch) (blockindexer.PostCommit, error) {
	// First index any domain contract deployments
	nonDeployEvents, txCompletions, err := d.dm.registrationIndexer(ctx, dbTX, batch)
	if err != nil {
		return nil, err
	}

	// Then divide remaining events by contract address and dispatch to the appropriate domain context
	batchesByAddress, err := d.batchEventsByAddress(ctx, dbTX, batch.BatchID.String(), nonDeployEvents)
	if err != nil {
		return nil, err
	}
	for addr, batch := range batchesByAddress {
		res, err := d.handleEventBatchForContract(ctx, addr, batch)
		if err != nil {
			return nil, err
		}
		for _, txCompletionEvent := range res.TransactionsComplete {
			var txHash tktypes.Bytes32
			txID, err := d.recoverTransactionID(ctx, txCompletionEvent.TransactionId)
			if err == nil {
				txHash, err = tktypes.ParseBytes32(txCompletionEvent.Location.TransactionHash)
			}
			if err != nil {
				return nil, err
			}
			txCompletions = append(txCompletions, &components.ReceiptInput{
				TransactionID: *txID,
				ReceiptType:   components.RT_Success,
				OnChain: tktypes.OnChainLocation{
					Type:             tktypes.OnChainEvent, // the on-chain confirmation is an event (even though it's a private transaction we're confirming)
					TransactionHash:  txHash,
					BlockNumber:      txCompletionEvent.Location.BlockNumber,
					TransactionIndex: txCompletionEvent.Location.TransactionIndex,
					LogIndex:         txCompletionEvent.Location.LogIndex,
					Source:           &addr,
				},
			})
		}
	}

	if len(txCompletions) > 0 {
		// Ensure we are sorted in block order, as the above processing extracted the array in two
		// phases (contract deployments, then transactions) so the list will be out of order.
		sort.Sort(txCompletions)

		// We have completions to hand to the TxManager to write as completions
		// Note we go directly to the TxManager (bypassing the private TX manager) during the database
		// transaction to write these receipts. We only write receipts for transactions where
		// we are the sender.
		//
		// Note separately below there is a notification to the private TX manager (after DB commit)
		// for ALL private transactions (not just those where we're the sender) as there
		// might be in-memory coordination activities that need to re-process now these
		// transactions have been finalized.
		if _, err := d.dm.txManager.MatchAndFinalizeTransactions(ctx, dbTX, txCompletions); err != nil {
			return nil, err
		}
	}

	return func() {
		d.dm.notifyTransactions(txCompletions)
	}, nil
}

func (d *domain) recoverTransactionID(ctx context.Context, txIDString string) (*uuid.UUID, error) {
	txIDBytes, err := tktypes.ParseBytes32Ctx(ctx, txIDString)
	if err != nil {
		return nil, err
	}
	txUUID := txIDBytes.UUIDFirst16()
	return &txUUID, nil
}

func (d *domain) handleEventBatchForContract(ctx context.Context, addr tktypes.EthAddress, batch *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {

	var res *prototk.HandleEventBatchResponse
	res, err := d.api.HandleEventBatch(ctx, batch)
	if err != nil {
		return nil, err
	}

	spentStates := make(map[uuid.UUID][]string, len(res.SpentStates))
	for _, state := range res.SpentStates {
		txUUID, err := d.recoverTransactionID(ctx, state.TransactionId)
		if err != nil {
			return nil, err
		}
		spentStates[*txUUID] = append(spentStates[*txUUID], state.Id)
	}

	confirmedStates := make(map[uuid.UUID][]string, len(res.ConfirmedStates))
	for _, state := range res.ConfirmedStates {
		txUUID, err := d.recoverTransactionID(ctx, state.TransactionId)
		if err != nil {
			return nil, err
		}
		confirmedStates[*txUUID] = append(confirmedStates[*txUUID], state.Id)
	}

	newStates := make(map[uuid.UUID][]*components.StateUpsert, len(res.NewStates))
	for _, state := range res.NewStates {
		txUUID, err := d.recoverTransactionID(ctx, state.TransactionId)
		if err != nil {
			return nil, err
		}
		var id tktypes.HexBytes
		if state.Id != nil {
			id, err = tktypes.ParseHexBytes(ctx, *state.Id)
			if err != nil {
				return nil, err
			}
		}
		newStates[*txUUID] = append(newStates[*txUUID], &components.StateUpsert{
			ID:       id,
			SchemaID: state.SchemaId,
			Data:     tktypes.RawJSON(state.StateDataJson),
			Creating: true,
		})
	}

	err = d.dm.stateStore.RunInDomainContext(d.name, addr, func(ctx context.Context, dsi components.DomainStateInterface) error {
		for txID, states := range newStates {
			if _, err = dsi.UpsertStates(&txID, states); err != nil {
				return err
			}
		}
		for txID, states := range spentStates {
			if err = dsi.MarkStatesSpent(txID, states); err != nil {
				return err
			}
		}
		for txID, states := range confirmedStates {
			if err = dsi.MarkStatesConfirmed(txID, states); err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}
