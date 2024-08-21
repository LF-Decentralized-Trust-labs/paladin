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

package main

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type PrivateSmartContract struct {
	DeployTX      uuid.UUID        `json:"deployTransaction"   gorm:"column:deploy_tx"`
	DomainAddress types.EthAddress `json:"domainAddress"       gorm:"column:domain_address"`
	Address       types.EthAddress `json:"address"             gorm:"column:address"`
	ConfigBytes   types.HexBytes   `json:"configBytes"         gorm:"column:config_bytes"`
}

type domainContract struct {
	dm   *domainManager
	d    *domain
	api  plugins.DomainManagerToDomain
	info *PrivateSmartContract
}

func (d *domain) GetSmartContractByAddress(ctx context.Context, addr types.EthAddress) (components.DomainSmartContract, error) {
	dc, isCached := d.contractCache.Get(addr)
	if isCached {
		return dc, nil
	}

	var contracts []*PrivateSmartContract
	err := d.dm.persistence.DB().
		Table("private_smart_contracts").
		Where("domain_address = ?", d.factoryContractAddress).
		Where("address = ?", addr).
		WithContext(ctx).
		Limit(1).
		Find(&contracts).
		Error
	if err != nil {
		return nil, err
	}
	if len(contracts) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgDomainContractNotFoundByAddr, addr)
	}

	dc = &domainContract{
		dm:   d.dm,
		d:    d,
		api:  d.api,
		info: contracts[0],
	}
	d.contractCache.Set(addr, dc)
	return dc, nil
}

func (dc *domainContract) InitTransaction(ctx context.Context, tx *components.PrivateTransaction) error {

	// We are responsible for building the PreAssembly
	preAssembly := &components.TransactionPreAssembly{}
	tx.PreAssembly = preAssembly

	// Query the base block height to inform the assembly step that comes later
	confirmedBlockHeight, err := dc.dm.blockIndexer.GetConfirmedBlockHeight(ctx)
	if err != nil {
		return err
	}

	// Build the init request
	txi := tx.Inputs
	var abiJSON []byte
	var paramsJSON []byte
	inputValues, err := txi.Function.Inputs.ParseJSONCtx(ctx, txi.Inputs)
	if err == nil {
		abiJSON, err = json.Marshal(txi.Function)
	}
	if err == nil {
		// Serialize to standardized JSON before passing to domain
		paramsJSON, err = types.StandardABISerializer().SerializeJSONCtx(ctx, inputValues)
	}
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainInvalidFunctionParams, txi.Function.SolString())
	}

	txSpec := &prototk.TransactionSpecification{}
	preAssembly.TransactionSpecification = txSpec
	txSpec.TransactionId = types.Bytes32UUIDLower16(tx.ID).String()
	txSpec.ContractAddress = dc.info.Address.String()
	txSpec.ContractConfig = dc.info.ConfigBytes
	txSpec.FunctionAbiJson = string(abiJSON)
	txSpec.FunctionParamsJson = string(paramsJSON)
	txSpec.FunctionSignature = txi.Function.SolString() // we use the proprietary "Solidity inspired" form that is very specific, including param names and nested struct defs
	txSpec.BaseBlock = int64(confirmedBlockHeight)

	// Do the request with the domain
	res, err := dc.api.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: txSpec,
	})
	if err != nil {
		return err
	}

	// Store the response back on the TX
	preAssembly.RequiredVerifiers = res.RequiredVerifiers
	return nil
}

func (dc *domainContract) AssembleTransaction(ctx context.Context, tx *components.PrivateTransaction) error {

	// Clear any previous assembly state out, as it's considered completely invalid
	// at this point if we're re-assembling.
	preAssembly := tx.PreAssembly
	postAssembly := &components.TransactionPostAssembly{}
	tx.PostAssembly = postAssembly

	// Now we have the required verifiers, we can ask the domain to do the heavy lifting
	// and assemble the transaction (using the state store interface we provide)
	res, err := dc.api.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction:       preAssembly.TransactionSpecification,
		ResolvedVerifiers: preAssembly.Verifiers,
	})
	if err != nil {
		return err
	}

	// We hydrate the states on our side of the Manager<->Plugin divide at this point,
	// which provides back to the engine the full sequence locking information of the
	// states (inputs, and read)
	postAssembly.InputStates, err = dc.loadStates(ctx, res.AssembledTransaction.SpentStates)
	if err != nil {
		return err
	}
	postAssembly.ReadStates, err = dc.loadStates(ctx, res.AssembledTransaction.ReadStates)
	if err != nil {
		return err
	}

	// We need to pass the assembly result back - it needs to be assigned to a sequence
	// before anything interesting can happen with the result here
	postAssembly.AssemblyResult = res.AssemblyResult
	// Note the states at this point are just potential states - depending on the analysis
	// of the result, and the locking on the input states, the engine might decide to
	// abandon this attempt and just re-assemble later.
	postAssembly.OutputStatesPotential = res.AssembledTransaction.NewStates
	return nil
}

// Happens only on the sequencing node
func (dc *domainContract) WritePotentialStates(ctx context.Context, tx *components.PrivateTransaction) error {
	// Now we're confident enough about this transaction to (on the sequencer) to have allocated
	// it to a sequence, and we want to write the OutputStatesPotential array:
	// 1) Writing them to the DB (unflushed at this point)
	// 2) Storing their identifiers into the OutputStatesFull list
	//
	// Note: This only happens on the sequencer node - any endorsing nodes just take the Full states
	//       and write them directly to the sequence prior to endorsement

	postAssembly := tx.PostAssembly

	var newStatesToWrite []*statestore.StateUpsert
	domain := dc.d
	for i, s := range postAssembly.OutputStatesPotential {
		schema := domain.schemasByID[s.SchemaId]
		if schema == nil {
			schema = domain.schemasBySignature[s.SchemaId]
		}
		if schema == nil {
			return i18n.NewError(ctx, msgs.MsgDomainUnknownSchema, s.SchemaId)
		}
		newStatesToWrite[i] = &statestore.StateUpsert{
			SchemaID: schema.IDString(),
			Data:     types.RawJSON(s.StateDataJson),
		}
	}

	var states []*statestore.State
	err := dc.dm.stateStore.RunInDomainContext(domain.name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		states, err = dsi.CreateNewStates(*postAssembly.Sequence, newStatesToWrite)
		return err
	})
	if err != nil {
		return err
	}
	// Store the results on the TX
	postAssembly.OutputStates = make([]*components.FullState, len(states))
	for i, s := range states {
		postAssembly.OutputStates[i] = &components.FullState{
			ID:     s.ID,
			Schema: s.Schema,
			Data:   s.Data,
		}
	}
	return nil

}

// Happens on all nodes that are aware of the transaction and want to mask input states from other
// transactions being assembled on the same node.
func (dc *domainContract) LockStates(ctx context.Context)

func (dc *domainContract) EndorseTransaction(ctx context.Context, tx *components.PrivateTransaction) error {

	// Important responsibilities of this function
	// 1) to ensure all the states have been written (unflushed) to the DB, so that the calling code
	//    and be confident that at the end of the next successful flush we have reliably recorded
	//    all these private states in a way that we won't forget them in the future (including across crash/restart)
	// 2) to ensure all the states have been marked as "locked" for spending in this transaction,
	//    within this sequence. So that other transactions (on different sequences, or the same sequence)
	//    will not attempt to spend the same states.
	//
	// This function does NOT FLUSH before or after doing endorse. The assumption is that this
	// is being handled as part of an overall sequence of endorsements, and for performance it is
	// more important to reduce the total number of flushes (rather than focus on the latency of one TX).
	//
	// The engine must ensure the flush occurs before returning the endorsement back to the requester,
	// but for efficiency we can and should start the runtime exercise of endorsement + signing before
	// waiting for the DB TX to commit.
	//
	if err := dc.ensureTXInputStatesLoaded(ctx, tx); err != nil {
		return err
	}

	return nil
}

func (dc *domainContract) PrepareTransaction(ctx context.Context, tx *components.PrivateTransaction) error {
	return nil
}

func (dc *domainContract) addStatesForUpsert(ctx context.Context, states []*statestore.State, bySchema map[types.Bytes32][]*statestore.StateUpsert) {
	for _, s := range states {
		bySchema[s.Schema] = append(bySchema[s.Schema], &statestore.StateUpsert{
			SchemaID: string(s.Schema),
		})
	}
}

func (dc *domainContract) analyzeInMemStates(ctx context.Context, stateRefs []*prototk.StateRef, fullStates []*statestore.State, loadedStatesByID map[types.Bytes32]*statestore.State) (missingStatesBySchema map[string][]types.RawJSON, err error) {

	// Process the list, and check we haven't already loaded them on this engine
	missingStatesBySchema = make(map[string][]types.RawJSON)
	for i, s := range stateRefs {
		stateID, err := types.ParseBytes32(ctx, s.HashId)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidStateIDFromDomain, s.HashId, i)
		}
		if i < len(fullStates) && fullStates[i].ID.Equals(stateID) {
			// We already have this state loaded in memory
			loadedStatesByID[*stateID] = fullStates[i]
		} else {
			missingStatesBySchema[s.SchemaId] = append(missingStatesBySchema[s.SchemaId], types.JSONString(s.HashId))
		}
	}
	return missingStatesBySchema, nil
}

func (dc *domainContract) loadStates(ctx context.Context, refs []*prototk.StateRef) ([]*components.FullState, error) {
	rawIDsBySchema := make(map[string][]types.RawJSON)
	stateIDs := make([]types.Bytes32, len(refs))
	for i, s := range refs {
		stateID, err := types.ParseBytes32(ctx, s.HashId)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidStateIDFromDomain, s.HashId, i)
		}
		rawIDsBySchema[s.SchemaId] = append(rawIDsBySchema[s.SchemaId], types.JSONString(stateID.String()))
		stateIDs[i] = *stateID
	}
	statesByID := make(map[types.Bytes32]*statestore.State)
	err := dc.dm.stateStore.RunInDomainContext(dc.d.name, func(ctx context.Context, dsi statestore.DomainStateInterface) error {
		for schemaID, stateIDs := range rawIDsBySchema {
			statesForSchema, err := dsi.FindAvailableStates(schemaID, &filters.QueryJSON{
				Statements: filters.Statements{
					Ops: filters.Ops{
						In: []*filters.OpMultiVal{
							{Op: filters.Op{Field: ".id"}, Values: stateIDs},
						},
					},
				},
			})
			if err != nil {
				return err
			}
			for _, s := range statesForSchema {
				statesByID[s.ID] = s
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Check we found all the states, and restore the original order
	states := make([]*components.FullState, len(stateIDs))
	for i, id := range stateIDs {
		s := statesByID[id]
		if s == nil {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInputStateNotFound, i, id)
		}
		states[i] = &components.FullState{
			ID:     s.ID,
			Schema: s.Schema,
			Data:   s.Data,
		}
	}
	return states, nil

}
