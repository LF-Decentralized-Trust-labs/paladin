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
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"github.com/hyperledger-firefly/signer/pkg/abi"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type PrivateSmartContract struct {
	DeployTX           uuid.UUID           `json:"deployTransaction"   gorm:"column:deploy_tx"`
	RegistryAddress    pldtypes.EthAddress `json:"domainAddress"       gorm:"column:domain_address"`
	Address            pldtypes.EthAddress `json:"address"             gorm:"column:address"`
	ConfigBytes        pldtypes.HexBytes   `json:"configBytes"         gorm:"column:config_bytes"`
	ConfigLocation     *string             `json:"configLocation"      gorm:"column:config_location"`      // of the event that wrote ConfigBytes
	IndexedConfigBytes pldtypes.HexBytes   `json:"indexedConfigBytes"  gorm:"column:indexed_config_bytes"` // in force at the event stream's position
	Created            pldtypes.Timestamp  `json:"created"             gorm:"column:created;autoCreateTime:nano"`
}

type domainContract struct {
	dm     *domainManager
	d      *domain
	api    components.DomainManagerToDomain
	info   *PrivateSmartContract   // from the DB
	config *prototk.ContractConfig // from init processing in the domain
}

type pscLoadResult int

const (
	pscLoadError pscLoadResult = iota
	pscInitError
	pscNotFound
	pscDomainNotFound
	pscInvalid
	pscValid
)

func (d *domain) initSmartContract(ctx context.Context, dbTX persistence.DBTX, def *PrivateSmartContract) (pscLoadResult, *domainContract, error) {
	loadResult, dc, err := d.newDomainContract(ctx, dbTX, def, def.ConfigBytes)
	// Only cache valid ones
	if dc != nil {
		d.dm.contractCache.Set(dc.info.Address, dc)
	}
	return loadResult, dc, err
}

func (d *domain) newDomainContract(ctx context.Context, dbTX persistence.DBTX, def *PrivateSmartContract, configBytes pldtypes.HexBytes) (pscLoadResult, *domainContract, error) {
	dc := &domainContract{
		dm:   d.dm,
		d:    d,
		api:  d.api,
		info: def,
	}
	var privacyGroup *prototk.PrivacyGroup
	pg, err := d.dm.groupManager.QueryGroups(ctx, dbTX, query.NewQueryBuilder().Equal("domain", d.name).Equal("contractAddress", &def.Address).Limit(1).Query())
	if err != nil {
		return pscInitError, nil, err
	}
	if len(pg) > 0 {
		privacyGroup = mapPrivacyGroupToProto(pg[0].ID, pg[0].GenesisStateData())
	}

	res, err := d.api.InitContract(ctx, &prototk.InitContractRequest{
		ContractAddress: def.Address.String(),
		ContractConfig:  configBytes,
		PrivacyGroup:    privacyGroup,
	})
	if err != nil {
		log.L(ctx).Errorf("Error initializing smart contract address: %s with config %s :  %s", def.Address, configBytes.HexString(), err.Error())
		return pscInitError, nil, err
	}
	if !res.Valid {
		log.L(ctx).Warnf("smart contract %s has invalid configuration rejected by the domain", def.Address)
		return pscInvalid, nil, nil
	}
	dc.config = res.ContractConfig
	return pscValid, dc, nil
}

func (dc *domainContract) buildBlockContext(ctx context.Context) (*prototk.BlockContext, error) {
	b, err := dc.dm.blockIndexer.GetLatestConfirmedBlockMetadata(ctx)
	if err != nil {
		return nil, err
	}
	return &prototk.BlockContext{
		BlockNumber:    b.Number,
		BlockTimestamp: b.Timestamp,
	}, nil
}

func (dc *domainContract) buildTransactionSpecification(ctx context.Context, localTx *components.ResolvedTransaction, intent prototk.TransactionSpecification_Intent) (*prototk.TransactionSpecification, error) {

	if localTx.Transaction == nil || localTx.Transaction.Data == nil || localTx.Function == nil ||
		localTx.Transaction.Domain != dc.Domain().Name() || *localTx.Transaction.To != dc.info.Address {
		log.L(ctx).Errorf("Invalid tx for domain %s/%s: %+v", dc.Domain().Name(), dc.info.Address, localTx.Transaction)
		return nil, i18n.NewError(ctx, msgs.MsgDomainTxnInputDefinitionInvalid)
	}

	var abiJSON []byte
	fnDef := localTx.Function.Definition
	inputValues, err := fnDef.Inputs.ParseJSONCtx(ctx, localTx.Transaction.Data)
	if err == nil {
		abiJSON, err = json.Marshal(fnDef)
	}
	var paramsJSON []byte
	if err == nil {
		// Serialize to standardized JSON before passing to domain
		paramsJSON, err = pldtypes.StandardABISerializer().SerializeJSONCtx(ctx, inputValues)
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidFunctionParams, fnDef.SolString())
	}

	return &prototk.TransactionSpecification{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    dc.info.Address.String(),
			ContractConfigJson: dc.config.ContractConfigJson,
		},
		From:               localTx.Transaction.From,
		FunctionAbiJson:    string(abiJSON),
		FunctionParamsJson: string(paramsJSON),
		FunctionSignature:  fnDef.SolString(), // we use the proprietary "Solidity inspired" form that is very specific, including param names and nested struct defs
		Intent:             intent,
	}, nil
}

func (dc *domainContract) ContractConfig() *prototk.ContractConfig {
	return dc.config
}

func (dc *domainContract) InitTransaction(ctx context.Context, tx *components.PrivateTransaction, localTx *components.ResolvedTransaction) error {

	txSpec, err := dc.buildTransactionSpecification(ctx, localTx, tx.Intent)
	if err != nil {
		return err
	}
	txSpec.TransactionId = pldtypes.Bytes32UUIDFirst16(tx.ID).String()
	tx.ID = *localTx.Transaction.ID
	tx.Domain = localTx.Transaction.Domain
	tx.Address = *localTx.Transaction.To

	// Do the request with the domain
	log.L(ctx).Infof("Initializing transaction=%s domain=%s contract-address=%s", tx.ID, dc.d.name, txSpec.ContractInfo.ContractAddress)
	res, err := dc.api.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: txSpec,
	})
	if err != nil {
		return err
	}

	// Store the response back on the TX
	tx.PreAssembly = &prototk.TransactionPreAssembly{
		TransactionSpecification: txSpec,
		RequiredVerifiers:        res.RequiredVerifiers,
		PublicTxOptions:          publicTxOptionsToProto(localTx.Transaction.PublicTxOptions),
	}
	return nil
}

func setLocalNode(localNode, lookup string) string {
	if strings.Contains(lookup, "@") {
		return lookup
	}
	return fmt.Sprintf("%s@%s", lookup, localNode)
}

func (dc *domainContract) fullyQualifyAssemblyIdentities(res *prototk.AssembleTransactionResponse) {
	localNode := dc.dm.transportMgr.LocalNodeName()
	for _, ap := range res.AttestationPlan {
		for i := range ap.Parties {
			ap.Parties[i] = setLocalNode(localNode, ap.Parties[i])
		}
	}
	for _, sp := range res.AssembledTransaction.OutputStates {
		for i := range sp.DistributionList {
			sp.DistributionList[i] = setLocalNode(localNode, sp.DistributionList[i])
		}
		for i := range sp.NullifierSpecs {
			sp.NullifierSpecs[i].Party = setLocalNode(localNode, sp.NullifierSpecs[i].Party)
		}
	}
	for _, sp := range res.AssembledTransaction.InfoStates {
		for i := range sp.DistributionList {
			sp.DistributionList[i] = setLocalNode(localNode, sp.DistributionList[i])
		}
		for i := range sp.NullifierSpecs {
			sp.NullifierSpecs[i].Party = setLocalNode(localNode, sp.NullifierSpecs[i].Party)
		}
	}
}

func (dc *domainContract) AssembleTransaction(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, txID uuid.UUID, preAssembly *prototk.TransactionPreAssembly, localTx *components.ResolvedTransaction, resolvedVerifiers []*prototk.ResolvedVerifier) (*prototk.TransactionPostAssembly, error) {
	if preAssembly.GetTransactionSpecification() == nil || localTx.Transaction == nil || localTx.Transaction.ID == nil || *localTx.Transaction.ID != txID {
		return nil, i18n.NewError(ctx, msgs.MsgDomainTXIncompleteAssembleTransaction)
	}
	// Assemble is a originator role operation, that must be performed only using the local details of the transaction as submitted
	// to this node by the application connected to that node. We cannot use any data that was received over the wire
	// from the coordinator as part of the assembly. The preassembly passed in here must have come from the originator.

	c := dc.d.newInFlightDomainRequest(readTX, dqc, true)
	defer c.close()

	blockCtx, err := dc.buildBlockContext(ctx)
	if err != nil {
		return nil, err
	}

	// Now we have the required verifiers, we can ask the domain to do the heavy lifting
	// and assemble the transaction (using the state store interface we provide)
	log.L(ctx).Info("Assembling transaction")
	res, err := dc.api.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		StateQueryContext: c.id,
		Transaction:       preAssembly.TransactionSpecification,
		ResolvedVerifiers: resolvedVerifiers,
		BlockContext:      blockCtx,
	})
	if err != nil {
		return nil, err
	}

	assemblyResponse := &prototk.TransactionPostAssembly{
		AssemblyResult:  res.AssemblyResult,
		BlockContext:    blockCtx,
		AttestationPlan: res.AttestationPlan,
		RevertReason:    res.RevertReason,
	}
	// If the result is not OK (e.g. there is a REVERT) then we return the situation to the private TX manager to handle
	if res.AssemblyResult == prototk.AssembleTransactionResponse_OK && res.AssembledTransaction != nil {
		// We hydrate the states on our side of the Manager<->Plugin divide at this point,
		// which provides back to the engine the full sequence locking information of the
		// states (inputs, and read)
		log.L(ctx).Debugf("Loading post-assembly input states from context")
		inputStates, err := dc.loadStatesFromContext(ctx, dqc, readTX, res.AssembledTransaction.InputStates)
		if err != nil {
			return nil, err
		}
		log.L(ctx).Debugf("Loading post-assembly read states from context")
		readStates, err := dc.loadStatesFromContext(ctx, dqc, readTX, res.AssembledTransaction.ReadStates)
		if err != nil {
			return nil, err
		}

		// At this point we resolve all verifiers to their fully qualified variants - as the local node needs to be the assembling node.
		// - Attestation plan identities
		// - State distributions
		// - Associated nullifier requests
		log.L(ctx).Info("Qualifying assembly identities")
		dc.fullyQualifyAssemblyIdentities(res)

		// Note the states at this point are just potential states - depending on the analysis
		// of the result, and the locking on the input states, the engine might decide to
		// abandon this attempt and just re-assemble later.
		log.L(ctx).Debugf("Number of potential output states: %+v", len(res.AssembledTransaction.OutputStates))
		log.L(ctx).Debugf("Number of potential info states: %+v", len(res.AssembledTransaction.InfoStates))
		assemblyResponse.OutputStatesPotential = res.AssembledTransaction.OutputStates
		assemblyResponse.InfoStatesPotential = res.AssembledTransaction.InfoStates
		assemblyResponse.DomainData = res.AssembledTransaction.DomainData
		assemblyResponse.InputStates = inputStates
		assemblyResponse.ReadStates = readStates
	}

	return assemblyResponse, nil
}

// Resolve the OutputStatesPotential+InfoStatesPotential arrays into validated and normalized
// proto EndorsableStates and StatesWithLabels for use in post assembly sequencing steps.
func (dc *domainContract) ResolvePotentialStates(ctx context.Context, readTX persistence.DBTX, tx *components.PrivateTransaction) (err error) {
	if tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil || tx.PostAssembly == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteWritePotentialStates)
	}

	postAssembly := tx.PostAssembly
	outputStatesPotential := postAssembly.AssembleResponse.GetOutputStatesPotential()
	infoStatesPotential := postAssembly.AssembleResponse.GetInfoStatesPotential()
	postAssembly.OutputStates, postAssembly.OutputStatesWithLabels, err = dc.resolvePotentialStates(ctx, readTX, outputStatesPotential)
	if err == nil {
		postAssembly.InfoStates, postAssembly.InfoStatesWithLabels, err = dc.resolvePotentialStates(ctx, readTX, infoStatesPotential)
	}
	if err == nil && log.IsDebugEnabled() {
		stateIDs := ""
		for _, state := range postAssembly.OutputStates {
			stateIDs += "," + state.GetId()
		}
		log.L(ctx).Debugf("transaction %s output state IDs: %s", tx.ID, stateIDs)
	}
	return err
}

// resolvePotentialStates validates the domain's potential states to compute their state IDs and
// label values, returning the proto states (with those IDs) alongside the states with labels.
// Both forms come from a single validation pass: the proto states carry the IDs onwards to the endorsers,
// the base ledger and other nodes, while the states with labels are what is eventually persisted.
func (dc *domainContract) resolvePotentialStates(ctx context.Context, readTX persistence.DBTX, potentialStates []*prototk.NewState) (endorsableStates []*prototk.EndorsableState, resolvedStates []*components.StateWithLabels, err error) {
	if len(potentialStates) == 0 {
		return nil, nil, nil
	}
	toValidate := make([]*prototk.EndorsableState, len(potentialStates))
	for i, s := range potentialStates {
		es := &prototk.EndorsableState{SchemaId: s.SchemaId, StateDataJson: s.StateDataJson}
		if s.Id != nil {
			es.Id = *s.Id
		}
		toValidate[i] = es
	}

	resolvedStates, err = dc.dm.stateStore.ValidateStatesWithLabels(ctx, readTX, dc.d, dc.info.Address, toValidate...)
	if err != nil {
		return nil, nil, err
	}

	endorsableStates = make([]*prototk.EndorsableState, len(resolvedStates))
	for i, s := range resolvedStates {
		// convert the resolved state back to endorsable form as well, so the
		// consumer can store both forms instead of performing repeat conversion.
		// Part of the validation was normalization of the state data against the
		// state ABI schema, so the data converted back into this new endorsable
		// state is not necessarily the same as the state data passed into the
		// validate function, even if both are prototk.EndorsableState types.
		endorsableStates[i] = &prototk.EndorsableState{
			Id:            s.ID.String(),
			SchemaId:      s.Schema.String(),
			StateDataJson: string(s.Data),
		}
	}
	return endorsableStates, resolvedStates, nil
}

// Endorse is a little special, because it returns a payload rather than updating the transaction.
func (dc *domainContract) EndorseTransaction(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, req *components.PrivateTransactionEndorseRequest) (*components.EndorsementResult, error) {

	if req == nil ||
		req.TransactionSpecification == nil ||
		req.Endorsement == nil ||
		req.Endorser == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainReqIncompleteEndorseTransaction)
	}

	c := dc.d.newInFlightDomainRequest(readTX, dqc, true)
	defer c.close()

	// This function does NOT FLUSH before or after doing endorse. The assumption is that this
	// is being handled as part of an overall sequence of endorsements, and for performance it is
	// more important to reduce the total number of flushes (rather than focus on the latency of one TX).
	//
	// The engine must ensure the flush occurs before returning the endorsement back to the requester,
	// but for efficiency we can and should start the runtime exercise of endorsement + signing before
	// waiting for the DB TX to commit.

	// Run the endorsement
	log.L(ctx).Infof("Running endorse transaction=%s domain=%s contract-address=%s",
		req.TransactionSpecification.TransactionId, dc.d.name, req.TransactionSpecification.ContractInfo.ContractAddress)
	res, err := dc.api.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		StateQueryContext:   c.id,
		Transaction:         req.TransactionSpecification,
		BlockContext:        req.BlockContext,
		ResolvedVerifiers:   req.Verifiers,
		Inputs:              req.InputStates,
		Reads:               req.ReadStates,
		Outputs:             req.OutputStates,
		Info:                req.InfoStates,
		Signatures:          req.Signatures,
		EndorsementRequest:  req.Endorsement,
		EndorsementVerifier: req.Endorser,
	})
	// We don't do any processing - as the result is not directly processable by us.
	// It is an instruction to the engine - such as an authority to sign an endorsement,
	// or a constraint on submission to the chain
	if err != nil {
		return nil, err
	}
	return &components.EndorsementResult{
		Endorser:     req.Endorser,
		Result:       res.EndorsementResult,
		Payload:      res.Payload,
		RevertReason: res.RevertReason,
	}, nil
}

func (dc *domainContract) PrepareTransaction(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, tx *components.PrivateTransaction) (*components.PrepareTransactionResult, error) {
	if tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil ||
		tx.PostAssembly == nil || tx.Signer == "" {
		return nil, i18n.NewError(ctx, msgs.MsgDomainTXIncompletePrepareTransaction)
	}

	preAssembly := tx.PreAssembly
	postAssembly := tx.PostAssembly

	c := dc.d.newInFlightDomainRequest(readTX, dqc, true)
	defer c.close()

	// Run the prepare
	log.L(ctx).Info("Preparing transaction")
	res, err := dc.api.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		StateQueryContext: c.id,
		Transaction:       preAssembly.TransactionSpecification,
		InputStates:       postAssembly.AssembleResponse.GetInputStates(),
		ReadStates:        postAssembly.AssembleResponse.GetReadStates(),
		OutputStates:      postAssembly.OutputStates,
		InfoStates:        postAssembly.InfoStates,
		AttestationResult: dc.allAttestations(tx),
		ResolvedVerifiers: postAssembly.AssembleResponse.GetResolvedVerifiers(),
		DomainData:        postAssembly.AssembleResponse.DomainData,
	})
	if err != nil {
		return nil, err
	}

	var functionABI abi.Entry
	if err := json.Unmarshal(([]byte)(res.Transaction.FunctionAbiJson), &functionABI); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgDomainPrivateAbiJsonInvalid)
	}

	contractAddress := &dc.info.Address
	if res.Transaction.ContractAddress != nil {
		contractAddress, err = pldtypes.ParseEthAddress(*res.Transaction.ContractAddress)
		if err != nil {
			return nil, err
		}
	}

	prep := &components.PrepareTransactionResult{Signer: tx.Signer}
	if res.Transaction.RequiredSigner != nil && len(*res.Transaction.RequiredSigner) > 0 {
		prep.Signer = *res.Transaction.RequiredSigner
	}

	if res.Transaction.Type == prototk.PreparedTransaction_PRIVATE {
		psc, err := dc.dm.GetSmartContractByAddress(ctx, readTX, *contractAddress)
		if err != nil {
			return nil, err
		}
		prep.PreparedPrivateTransaction = &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				IdempotencyKey: fmt.Sprintf("%s_%s", tx.ID, functionABI.Name),
				Type:           pldapi.TransactionTypePrivate.Enum(),
				Function:       functionABI.String(),
				From:           prep.Signer,
				To:             contractAddress,
				Data:           pldtypes.RawJSON(res.Transaction.ParamsJson),
				Domain:         psc.Domain().Name(),
			},
			ABI: abi.ABI{&functionABI},
		}
	} else {
		prep.PreparedPublicTransaction = &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type:            pldapi.TransactionTypePublic.Enum(),
				Function:        functionABI.String(),
				From:            prep.Signer,
				To:              contractAddress,
				Data:            pldtypes.RawJSON(res.Transaction.ParamsJson),
				PublicTxOptions: publicTxOptionsFromProto(tx.PreAssembly.PublicTxOptions),
			},
			ABI: abi.ABI{&functionABI},
		}
		// We cannot fall back to eth_estimateGas, as we queue up multiple transactions for dispatch that chain together.
		// As such our transactions are not always executable in isolation, and would revert (due to consuming non-existent UTXO states)
		// if we attempted to do gas estimation or call.
		if prep.PreparedPublicTransaction.Gas == nil {
			prep.PreparedPublicTransaction.Gas = &dc.d.defaultGasLimit
		}
	}
	if res.Metadata != nil {
		prep.PreparedMetadata = pldtypes.RawJSON(*res.Metadata)
	}
	return prep, nil
}

func (dc *domainContract) InitCall(ctx context.Context, callTx *components.ResolvedTransaction) ([]*prototk.ResolveVerifierRequest, error) {

	txSpec, err := dc.buildTransactionSpecification(ctx, callTx, prototk.TransactionSpecification_CALL)
	if err != nil {
		return nil, err
	}

	// Call the domain
	res, err := dc.api.InitCall(ctx, &prototk.InitCallRequest{
		Transaction: txSpec,
	})
	if err != nil {
		return nil, err
	}

	return res.RequiredVerifiers, nil

}

func (dc *domainContract) ExecCall(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, callTx *components.ResolvedTransaction, verifiers []*prototk.ResolvedVerifier) (*abi.ComponentValue, error) {

	txSpec, err := dc.buildTransactionSpecification(ctx, callTx, prototk.TransactionSpecification_CALL)
	if err != nil {
		return nil, err
	}

	blockCtx, err := dc.buildBlockContext(ctx)
	if err != nil {
		return nil, err
	}

	// We expect queries to the state store during this call
	c := dc.d.newInFlightDomainRequest(readTX, dqc, true)
	defer c.close()

	// Call the domain
	res, err := dc.api.ExecCall(ctx, &prototk.ExecCallRequest{
		StateQueryContext: c.id,
		ResolvedVerifiers: verifiers,
		Transaction:       txSpec,
		BlockContext:      blockCtx,
	})
	if err != nil {
		return nil, err
	}

	// The outputs must conform to the spec
	outputDef := callTx.Function.Definition.Outputs
	if outputDef == nil {
		outputDef = abi.ParameterArray{}
	}
	if res.ResultJson == "" {
		res.ResultJson = `[]`
	}
	cv, err := outputDef.ParseJSONCtx(ctx, []byte(res.ResultJson))
	if err != nil {
		log.L(ctx).Errorf("Invalid data from domain for %s: %s", callTx.Function.Definition.SolString(), res.ResultJson)
		return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidDataFromDomain)
	}

	return cv, nil
}

func (dc *domainContract) Domain() components.Domain {
	return dc.d
}

func (dc *domainContract) Address() pldtypes.EthAddress {
	return dc.info.Address
}

func (dc *domainContract) allAttestations(tx *components.PrivateTransaction) []*prototk.AttestationResult {
	attestations := append([]*prototk.AttestationResult{}, tx.PostAssembly.AssembleResponse.GetSignatures()...)
	attestations = append(attestations, tx.PostAssembly.CollectedEndorsements...)
	return attestations
}

func (dc *domainContract) loadStatesFromContext(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, refs []*prototk.StateRef) ([]*prototk.EndorsableState, error) {
	rawIDsBySchema := make(map[pldtypes.Bytes32][]pldtypes.RawJSON)
	stateIDs := make([]pldtypes.HexBytes, len(refs))
	for i, s := range refs {
		stateID, err := pldtypes.ParseHexBytes(ctx, s.Id)
		var schemaID pldtypes.Bytes32
		if err == nil {
			schemaID, err = pldtypes.ParseBytes32(s.SchemaId)
		}
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidStateIDFromDomain, s.Id, i)
		}
		rawIDsBySchema[schemaID] = append(rawIDsBySchema[schemaID], pldtypes.JSONString(stateID.String()))
		stateIDs[i] = stateID
	}
	statesByID := make(map[string]*pldapi.State)
	for schemaID, stateIDs := range rawIDsBySchema {
		log.L(ctx).Debugf("Finding available states for state IDs %+v", stateIDs)
		_, statesForSchema, err := dqc.FindAvailableStates(ctx, readTX, schemaID, &query.QueryJSON{
			Statements: query.Statements{
				Ops: query.Ops{
					In: []*query.OpMultiVal{
						{Op: query.Op{Field: ".id"}, Values: stateIDs},
					},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		for _, s := range statesForSchema {
			statesByID[s.ID.HexString()] = s
		}
	}

	// Check we found all the states, and restore the original order
	states := make([]*prototk.EndorsableState, len(stateIDs))
	for i, id := range stateIDs {
		s := statesByID[id.HexString()]
		if s == nil {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInputStateNotFound, i, id)
		}
		states[i] = &prototk.EndorsableState{
			Id:            s.ID.String(),
			SchemaId:      s.Schema.String(),
			StateDataJson: string(s.Data),
		}
	}
	log.L(ctx).Debugf("Found available states %+v", states)
	return states, nil

}

func mapPrivacyGroupToProto(stateID pldtypes.HexBytes, pg *pldapi.PrivacyGroupGenesisState) *prototk.PrivacyGroup {
	return &prototk.PrivacyGroup{
		Id:            stateID.String(),
		GenesisSalt:   pg.GenesisSalt.String(),
		Name:          pg.Name,
		Members:       pg.Members,
		Properties:    pg.Properties.Map(),
		Configuration: pg.Configuration.Map(),
	}
}

func (dc *domainContract) WrapPrivacyGroupEVMTX(ctx context.Context, pg *pldapi.PrivacyGroup, pgTX *pldapi.PrivacyGroupEVMTX) (ptx *pldapi.TransactionInput, err error) {

	// We do nothing apart from type conversion here, as the domain is going to wrap this call
	// and return the private transaction, such that it can be validated fully there.

	var pToAddr *string
	if pgTX.To != nil {
		toAddr := pgTX.To.String()
		pToAddr = &toAddr
	}
	var pGas *string
	if pgTX.Gas != nil {
		gasStr := pgTX.Gas.String()
		pGas = &gasStr
	}
	var pValue *string
	if pgTX.Value != nil {
		valueStr := pgTX.Value.String()
		pValue = &valueStr
	}
	var pInput *string
	if pgTX.Input != nil {
		inputStr := pgTX.Input.String()
		pInput = &inputStr
	}
	var pABI *string
	if pgTX.Function != nil {
		abiStr := pldtypes.JSONString(pgTX.Function).String()
		pABI = &abiStr
	}

	// Call the domain to do the work
	res, err := dc.api.WrapPrivacyGroupEVMTX(ctx, &prototk.WrapPrivacyGroupEVMTXRequest{
		PrivacyGroup: mapPrivacyGroupToProto(pg.ID, pg.GenesisStateData()),
		Transaction: &prototk.PrivacyGroupEVMTX{
			ContractInfo: &prototk.ContractInfo{
				ContractAddress:    dc.info.Address.String(),
				ContractConfigJson: dc.config.ContractConfigJson,
			},
			From:            pgTX.From,
			To:              pToAddr,
			Gas:             pGas,
			Value:           pValue,
			InputJson:       pInput,
			FunctionAbiJson: pABI,
			Bytecode:        pgTX.Bytecode,
		},
	})
	if err != nil {
		return nil, err
	}

	// Function returned must be private
	if res.Transaction.Type != prototk.PreparedTransaction_PRIVATE {
		return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidPGroupTxTypeNotPrivate, res.Transaction.Type)
	}

	ptx = &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:   pldapi.TransactionTypePrivate.Enum(),
			Domain: dc.d.name,
		},
	}

	pscAddr := dc.Address()
	if res.Transaction.ContractAddress != nil {
		addr, err := pldtypes.ParseEthAddress(*res.Transaction.ContractAddress)
		if err != nil {
			return nil, err
		}
		if *addr != pscAddr {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidPGroupTxCannotRedirect, pscAddr, addr)
		}
	}
	ptx.To = &pscAddr

	// Always set the function definition
	var wrappedFnABI abi.Entry
	if err := json.Unmarshal([]byte(res.Transaction.FunctionAbiJson), &wrappedFnABI); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidFunctionParams, res.Transaction.FunctionAbiJson)
	}
	ptx.Function = wrappedFnABI.Name
	ptx.ABIReference = nil
	ptx.ABI = abi.ABI{&wrappedFnABI}

	// And the inputs
	ptx.Data = pldtypes.RawJSON(res.Transaction.ParamsJson)

	// Only update the signer if returned
	if res.Transaction.RequiredSigner != nil && len(*res.Transaction.RequiredSigner) > 0 {
		ptx.From = *res.Transaction.RequiredSigner
	}

	return ptx, nil

}
func (dc *domainContract) IsBaseLedgerRevertRetryable(ctx context.Context, revertData []byte) (bool, string, error) {
	res, err := dc.api.IsBaseLedgerRevertRetryable(ctx, &prototk.IsBaseLedgerRevertRetryableRequest{
		RevertData: revertData,
	})
	if err != nil {
		return false, "", err
	}
	return res.Retryable, res.DecodedReason, nil
}

func (dc *domainContract) InvokeRPC(ctx context.Context, dqc components.DomainQueryContext, dbTX persistence.DBTX, rpcCall pldapi.DomainInvokeRPC) (pldtypes.RawJSON, error) {
	c := dc.d.newInFlightDomainRequest(dbTX, dqc, false)
	defer c.close()
	res, err := dc.api.InvokeRPC(ctx, &prototk.InvokeRPCRequest{
		StateQueryContext: c.id,
		Method:            rpcCall.Method,
		ParamsJson:        string(rpcCall.Params),
	})
	if err != nil {
		return nil, err
	}
	return pldtypes.RawJSON(res.ResultJson), nil
}

func publicTxOptionsToProto(opts pldapi.PublicTxOptions) *prototk.PublicTxOptions {
	p := &prototk.PublicTxOptions{}
	if opts.Gas != nil {
		s := opts.Gas.String()
		p.Gas = &s
	}
	if opts.Value != nil {
		s := opts.Value.String()
		p.Value = &s
	}
	if opts.MaxFeePerGas != nil {
		s := opts.MaxFeePerGas.String()
		p.MaxFeePerGas = &s
	}
	if opts.MaxPriorityFeePerGas != nil {
		s := opts.MaxPriorityFeePerGas.String()
		p.MaxPriorityFeePerGas = &s
	}
	return p
}

func publicTxOptionsFromProto(p *prototk.PublicTxOptions) pldapi.PublicTxOptions {
	if p == nil {
		return pldapi.PublicTxOptions{}
	}
	opts := pldapi.PublicTxOptions{}
	if p.Gas != nil {
		if v, err := pldtypes.ParseHexUint64(context.Background(), *p.Gas); err == nil {
			opts.Gas = &v
		}
	}
	if p.Value != nil {
		if v, err := pldtypes.ParseHexUint256(context.Background(), *p.Value); err == nil {
			opts.Value = v
		}
	}
	if p.MaxFeePerGas != nil {
		if v, err := pldtypes.ParseHexUint256(context.Background(), *p.MaxFeePerGas); err == nil {
			opts.MaxFeePerGas = v
		}
	}
	if p.MaxPriorityFeePerGas != nil {
		if v, err := pldtypes.ParseHexUint256(context.Background(), *p.MaxPriorityFeePerGas); err == nil {
			opts.MaxPriorityFeePerGas = v
		}
	}
	return opts
}
