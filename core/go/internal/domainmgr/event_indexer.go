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
	"fmt"
	"sort"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
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

func (dm *domainManager) registrationIndexer(ctx context.Context, dbTX persistence.DBTX, batch *blockindexer.EventDeliveryBatch) ([]*pldapi.EventWithData, []pldtypes.EthAddress, txCompletionsOrdered, error) {

	var contracts []*PrivateSmartContract
	var txCompletions txCompletionsOrdered
	nonRegisterEvents := make([]*pldapi.EventWithData, 0, len(batch.Events))

	for _, ev := range batch.Events {
		if ev.SoliditySignature == eventSolSig_PaladinRegisterSmartContract_V0 {

			// Our event stream configuration includes the addresses of the configured domain registries, so all registration events
			// we receive should be for a domain that is configured on this node.
			d, err := dm.getDomainByAddress(ctx, &ev.Address)
			if err != nil {
				log.L(ctx).Errorf("Registration event for unknown domain event: %s", pldtypes.JSONString(ev))
				continue
			}

			var parsedEvent event_PaladinRegisterSmartContract_V0
			if err := json.Unmarshal(ev.Data, &parsedEvent); err != nil {
				return nil, nil, nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidRegistryEventData, ev.SoliditySignature)
			}
			txID := parsedEvent.TXId.UUIDFirst16()
			contracts = append(contracts, &PrivateSmartContract{
				DeployTX:           txID,
				RegistryAddress:    ev.Address,
				Address:            parsedEvent.Instance,
				ConfigBytes:        parsedEvent.Config,
				ConfigLocation:     configLocation(ev),
				IndexedConfigBytes: parsedEvent.Config,
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
		} else {
			nonRegisterEvents = append(nonRegisterEvents, ev)
		}
	}

	// Insert the batch of new contracts in this DB transaction (we do this before we call the domain to process the events).
	// A replayed registration moves the indexed config back to that at registration, so the events that follow it are
	// dispatched with the config in force when they were emitted.
	var registered []pldtypes.EthAddress
	if len(contracts) > 0 {
		err := dbTX.DB(ctx).
			Table("private_smart_contracts").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "address"}},
				DoUpdates: clause.AssignmentColumns([]string{"indexed_config_bytes"}),
				Where: clause.Where{Exprs: []clause.Expression{clause.Expr{
					SQL: "private_smart_contracts.domain_address = excluded.domain_address AND private_smart_contracts.deploy_tx = excluded.deploy_tx",
				}}},
			}).
			Create(contracts).
			Error
		if err != nil {
			return nil, nil, nil, err
		}
		for _, c := range contracts {
			registered = append(registered, c.Address)
		}
	}

	return nonRegisterEvents, registered, txCompletions, nil
}

// Fixed width, so that locations compare in chain order as strings
func configLocation(ev *pldapi.EventWithData) *string {
	location := fmt.Sprintf("%016x%08x%08x", ev.BlockNumber, ev.TransactionIndex, ev.LogIndex)
	return &location
}

// applyUpgrade sets the indexed config of a contract registered under this domain's registry to that announced by the
// contract, and its live config too unless a later event wrote it.
func (d *domain) applyUpgrade(ctx context.Context, dbTX persistence.DBTX, ev *pldapi.EventWithData, upgrade *event_PaladinUpgradeSmartContractConfig_V0) (registered bool, liveConfigChanged bool, err error) {
	// indexed_config_bytes stores whatever config the event stream has indexed up to. It may go backwards in the
	// case of event replay so is always written when a config upgrade event is received.It ensures that all
	// domain events are handled with the config that was in use when they were emitted.
	res := dbTX.DB(ctx).
		Table("private_smart_contracts").
		Where("address = ? AND domain_address = ?", ev.Address, d.registryAddress).
		Update("indexed_config_bytes", upgrade.Config)
	if res.Error != nil {
		return false, false, res.Error
	}
	if res.RowsAffected == 0 {
		log.L(ctx).Debugf("Discarding upgrade of %s that is not registered under domain %s", ev.Address, d.name)
		return false, false, nil
	}
	// config_bytes stores the latest config for any component that needs the "live" config. The column
	// is only updated if this event is newer than the event which wrote the last config. Location will only
	// be null if the row was written before config upgrade was supported, meaning a null location must be for
	// a registration event and is always older than an upgrade.
	location := configLocation(ev)
	res = dbTX.DB(ctx).
		Table("private_smart_contracts").
		Where("address = ? AND (config_location IS NULL OR config_location < ?)", ev.Address, *location).
		Updates(map[string]any{"config_bytes": upgrade.Config, "config_location": location})
	if res.Error != nil {
		return false, false, res.Error
	}
	if res.RowsAffected == 0 {
		log.L(ctx).Infof("Replayed upgrade of smart contract %s applied to the indexed configuration only", ev.Address)
		return true, false, nil
	}
	log.L(ctx).Infof("Configuration of smart contract %s upgraded", ev.Address)
	return true, true, nil
}

// Direct waiters are only used by the testbed
func (dm *domainManager) notifyWaiters(txCompletions txCompletionsOrdered) {
	for _, completion := range txCompletions {
		inflight := dm.privateTxWaiter.GetInflight(completion.TransactionID)
		log.L(dm.bgCtx).Debugf("Notifying of completion for private deployment TransactionID %s (waiter=%t)", completion.TransactionID, inflight != nil)
		if inflight != nil {
			inflight.Complete(&completion.ReceiptInput)
		}
	}
}

// Splits each contract's events into batches at its upgrades, keeping event order. Upgraded indexed config is
// persisted here in between batches, so that the next batch of events can be dispatched with the contract config
// that was in place at the time the events were emitted.
func (d *domain) batchEventsByContractConfig(ctx context.Context, dbTX persistence.DBTX, batchID string, registered []pldtypes.EthAddress, events []*pldapi.EventWithData) ([]*pscEventBatch, error) {

	var batches []*pscEventBatch
	indexedConfigWritten := make(map[pldtypes.EthAddress]bool)
	for _, addr := range registered {
		indexedConfigWritten[addr] = true
	}
	liveConfigChanged := make(map[pldtypes.EthAddress]bool)
	current := make(map[pldtypes.EthAddress]*pscEventBatch)

	for _, ev := range events {
		if ev.SoliditySignature == eventSolSig_PaladinUpgradeSmartContractConfig_V0 {
			var upgrade event_PaladinUpgradeSmartContractConfig_V0
			if err := json.Unmarshal(ev.Data, &upgrade); err != nil {
				return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidRegistryEventData, ev.SoliditySignature)
			}
			upgraded, liveChanged, err := d.applyUpgrade(ctx, dbTX, ev, &upgrade)
			if err != nil {
				return nil, err
			}
			if upgraded {
				// any batches created before the upgrade still exist in the batches slice but deleting
				// the reference to it from current will force the next event for this contract to
				// start a new batch which will be initialised with the new confix
				delete(current, ev.Address)
				indexedConfigWritten[ev.Address] = true
				if liveChanged {
					liveConfigChanged[ev.Address] = true
				}
			}
			continue
		}
		batch := current[ev.Address]
		if batch == nil {
			// Note: hits will be cached, but events from unrecognized contracts will always
			// result in a cache miss and a database lookup
			// TODO: revisit if we should optimize this
			psc, err := d.indexedSmartContract(ctx, dbTX, ev.Address, indexedConfigWritten[ev.Address])
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
			batches = append(batches, batch)
			current[ev.Address] = batch
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

	dbTX.AddPostCommit(func(txCtx context.Context) {
		// Once the config changes have been written, the cached domain used for handling events
		// must be invalidated, so the next batch of events has a domain loaded with the newly committed
		// config
		for addr := range indexedConfigWritten {
			d.indexedContracts.Delete(addr)
		}
		// Consumers holding a live configuration this batch replaced are refreshed here, before the indexer moves on,
		// so no work started after this batch is indexed can run against it.
		for addr := range liveConfigChanged {
			d.dm.contractConfigChanged(txCtx, addr)
		}
	})

	return batches, nil
}

// Returns the contract initialized with its indexed config, or nil when it is not a valid contract of a domain configured
// on this node. skipCache must be set for a contract whose row this DB TX wrote, as the write is not yet committed.
func (d *domain) indexedSmartContract(ctx context.Context, dbTX persistence.DBTX, addr pldtypes.EthAddress, skipCache bool) (*domainContract, error) {
	if !skipCache {
		if dc, isCached := d.indexedContracts.Get(addr); isCached {
			return dc, nil
		}
	}
	var contracts []*PrivateSmartContract
	err := dbTX.DB(ctx).
		Table("private_smart_contracts").
		Where("address = ?", addr).
		Limit(1).
		Find(&contracts).
		Error
	if err != nil || len(contracts) == 0 {
		return nil, err
	}
	def := contracts[0]
	contractDomain := d.dm.getDomainByAddressOrNil(&def.RegistryAddress)
	if contractDomain == nil {
		return nil, nil
	}
	_, dc, err := contractDomain.newDomainContract(ctx, dbTX, def, def.IndexedConfigBytes)
	if dc != nil && !skipCache {
		d.indexedContracts.Set(addr, dc)
	}
	return dc, err
}

func (d *domain) handleEventBatch(ctx context.Context, dbTX persistence.DBTX, batch *blockindexer.EventDeliveryBatch) error {

	// First index any domain contract deployments
	nonDeployEvents, registered, txCompletions, err := d.dm.registrationIndexer(ctx, dbTX, batch)
	if err != nil {
		return err
	}

	// Then divide remaining events by contract and configuration, and dispatch to the appropriate domain context
	batches, err := d.batchEventsByContractConfig(ctx, dbTX, batch.BatchID.String(), registered, nonDeployEvents)
	if err != nil {
		return err
	}
	for _, batch := range batches {
		addr := batch.psc.Address()
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
		// Enqueue the full sorted batch to the sequencer for ordered background processing.
		// Handling of the completions on the queue must happen outside of this goroutine, which is critical path for the
		// event indexer of the Paladin node.
		// So only if the channel to the sequencer ends up with back pressure will any slow-down happen to this routine
		d.enqueueCompletions(txCompletions)
		// We also provide a direct waiter that's used by the testbed
		d.dm.notifyWaiters(txCompletions)
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
	dqc := d.dm.stateStore.NewDomainQueryContext(ctx, d, addr)
	c := d.newInFlightDomainRequest(dbTX, dqc, false /* write enabled */)
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

	// Only confirmed states are tracked for the completion index
	confirmedStateIDsByTX := make(map[uuid.UUID]map[string]pldtypes.HexBytes)
	stateConfirms := make([]*pldapi.StateConfirmRecord, len(res.ConfirmedStates))
	for i, state := range res.ConfirmedStates {
		txUUID, stateID, err := d.prepareIndexRecord(ctx, state.TransactionId, state.Id)
		if err != nil {
			return nil, err
		}
		stateConfirms[i] = &pldapi.StateConfirmRecord{DomainName: d.name, State: stateID, Transaction: txUUID}
		if d.FullStateAvailablityRequired() {
			if len(stateID) > 0 {
				if confirmedStateIDsByTX[txUUID] == nil {
					confirmedStateIDsByTX[txUUID] = make(map[string]pldtypes.HexBytes)
				}
				confirmedStateIDsByTX[txUUID][stateID.String()] = stateID
			}
		}
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
		log.L(ctx).Infof("Writing state finalizations for %d spends, %d reads, %d confirms, %d info records", len(stateSpends), len(stateReads), len(stateConfirms), len(stateInfoRecords))
		if err := d.dm.stateStore.WriteStateFinalizations(ctx, dbTX, stateSpends, stateReads, stateConfirms, stateInfoRecords); err != nil {
			return nil, err
		}
	}

	// Update the global pending private state data index for domains that opt in.
	if len(confirmedStateIDsByTX) > 0 {
		var batchEntries []components.PendingPrivateStateDataEntry
		for _, txCompletionEvent := range res.TransactionsComplete {
			txUUID, err := d.recoverTransactionID(ctx, txCompletionEvent.TransactionId)
			if err != nil {
				return nil, err
			}
			for _, id := range confirmedStateIDsByTX[*txUUID] {
				batchEntries = append(batchEntries, components.PendingPrivateStateDataEntry{
					StateID:     id,
					Contract:    addr,
					BlockNumber: txCompletionEvent.Location.BlockNumber,
				})
			}
		}
		if err := d.dm.stateStore.WritePendingPrivateStateDataBatch(ctx, dbTX, d.name, batchEntries); err != nil {
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
