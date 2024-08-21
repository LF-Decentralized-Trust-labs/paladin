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

	// Query the base block height to inform the assembly step that comes later
	confirmedBlockHeight, err := dc.dm.blockIndexer.GetConfirmedBlockHeight(ctx)
	if err != nil {
		return err
	}

	// Build the init request
	var abiJSON []byte
	var paramsJSON []byte
	inputValues, err := tx.Function.Inputs.ParseJSONCtx(ctx, tx.Inputs)
	if err == nil {
		abiJSON, err = json.Marshal(tx.Function)
	}
	if err == nil {
		// Serialize to standardized JSON before passing to domain
		paramsJSON, err = types.StandardABISerializer().SerializeJSONCtx(ctx, inputValues)
	}
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainInvalidFunctionParams, tx.Function.SolString())
	}

	txSpec := &prototk.TransactionSpecification{}
	tx.TransactionSpecification = txSpec
	txSpec.TransactionId = types.Bytes32UUIDLower16(tx.ID).String()
	txSpec.ContractAddress = dc.info.Address.String()
	txSpec.ContractConfig = dc.info.ConfigBytes
	txSpec.FunctionAbiJson = string(abiJSON)
	txSpec.FunctionParamsJson = string(paramsJSON)
	txSpec.FunctionSignature = tx.Function.SolString() // we use the proprietary "Solidity inspired" form that is very specific, including param names and nested struct defs
	txSpec.BaseBlock = int64(confirmedBlockHeight)

	// Do the request with the domain
	res, err := dc.api.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: txSpec,
	})
	if err != nil {
		return err
	}

	// Store the response back on the TX
	tx.RequiredVerifiers = res.RequiredVerifiers
	return nil
}

func (dc *domainContract) AssembleTransaction(ctx context.Context, tx *components.PrivateTransaction) error {

	// Now we have the required verifiers, we can ask the domain to do the heavy lifting
	// and assemble the transaction (using the state store interface we provide)
	res, err := dc.api.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx.TransactionSpecification,
		ResolvedVerifiers: tx.Verifiers,
	})
	if err != nil {
		return err
	}

	// We need to pass the assembly result back - it needs to be assigned to a sequence
	// before anything interesting can happen with the result here
	tx.SpendingStatesRefs = res.AssembledTransaction.SpentStates
	tx.ReadStatesRefs = res.AssembledTransaction.ReadStates
	tx.NewStatesAssembled = res.AssembledTransaction.NewStates
	return nil
}

// Note this DOES NOT FLUSH (not our responsibility)
func (dc *domainContract) WriteSequenceStates(ctx context.Context, tx *components.PrivateTransaction) error {

	domain := dc.d
	newStatesToWrite := make([]*statestore.NewState, len(tx.NewStatesAssembled))
	for i, s := range tx.NewStatesAssembled {
		schema := domain.schemasByID[s.SchemaId]
		if schema == nil {
			schema = domain.schemasBySignature[s.SchemaId]
		}
		if schema == nil {
			return i18n.NewError(ctx, msgs.MsgDomainUnknownSchema, s.SchemaId)
		}
		newStatesToWrite[i] = &statestore.NewState{
			SchemaID: schema.IDString(),
			Data:     types.RawJSON(s.StateDataJson),
		}
	}

	var states []*statestore.State
	err := dc.dm.stateStore.RunInDomainContext(domain.name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		states, err = dsi.CreateNewStates(*tx.Sequence, newStatesToWrite)
		return err
	})
	if err != nil {
		return err
	}
	newStateIDs := make([]*prototk.StateRef, len(states))
	for i, ws := range states {
		newStateIDs[i] = &prototk.StateRef{
			SchemaId: ws.Schema.String(),
			HashId:   ws.ID.String(),
		}
	}

	// Store the results on the TX
	tx.NewStatesStored = states
	tx.NewStatesRefs = newStateIDs
	return nil

}

func (dc *domainContract) EndorseTransaction(ctx context.Context, tx *components.PrivateTransaction) error {
	return nil
}

func (dc *domainContract) PrepareTransaction(ctx context.Context, tx *components.PrivateTransaction) error {
	return nil
}
