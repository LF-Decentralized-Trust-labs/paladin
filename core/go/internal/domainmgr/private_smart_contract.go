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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type PrivateSmartContract struct {
	DeployTX        uuid.UUID          `json:"deployTransaction"   gorm:"column:deploy_tx"`
	RegistryAddress tktypes.EthAddress `json:"domainAddress"       gorm:"column:domain_address"`
	Address         tktypes.EthAddress `json:"address"             gorm:"column:address"`
	ConfigBytes     tktypes.HexBytes   `json:"configBytes"         gorm:"column:config_bytes"`
}

// TODO not really sure who owns this table
// domainmgr (who is responsible for listening for the deployment events and writing to the table)
// or txmgr (who is responsible for reading from the table and joining with the transactions and transaction_receipts tables)
type contractDeployment struct {
	DeployTransaction uuid.UUID          `json:"deployTransaction"     gorm:"column:deploy_transaction"`
	ContractAddress   tktypes.EthAddress `json:"contractAddress"  gorm:"column:contract_address"`
}

type domainContract struct {
	dm   *domainManager
	d    *domain
	api  components.DomainManagerToDomain
	info *PrivateSmartContract
}

func (d *domain) newSmartContract(def *PrivateSmartContract) *domainContract {
	dc := &domainContract{
		dm:   d.dm,
		d:    d,
		api:  d.api,
		info: def,
	}
	d.dm.contractCache.Set(dc.info.Address, dc)
	return dc
}

func (dc *domainContract) InitTransaction(ctx context.Context, tx *components.PrivateTransaction) error {
	if tx.Inputs == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteInitTransaction)
	}

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
		paramsJSON, err = tktypes.StandardABISerializer().SerializeJSONCtx(ctx, inputValues)
	}
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainInvalidFunctionParams, txi.Function.SolString())
	}

	txSpec := &prototk.TransactionSpecification{
		TransactionId:      tktypes.Bytes32UUIDFirst16(tx.ID).String(),
		ContractAddress:    dc.info.Address.String(),
		ContractConfig:     dc.info.ConfigBytes,
		From:               txi.From,
		FunctionAbiJson:    string(abiJSON),
		FunctionParamsJson: string(paramsJSON),
		FunctionSignature:  txi.Function.SolString(), // we use the proprietary "Solidity inspired" form that is very specific, including param names and nested struct defs
		BaseBlock:          int64(confirmedBlockHeight),
		Intent:             txi.Intent,
	}
	// Do the request with the domain
	res, err := dc.api.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: txSpec,
	})
	if err != nil {
		return err
	}

	// Store the response back on the TX
	preAssembly := &components.TransactionPreAssembly{
		TransactionSpecification: txSpec,
		RequiredVerifiers:        res.RequiredVerifiers,
	}
	tx.PreAssembly = preAssembly
	return nil
}

func (dc *domainContract) AssembleTransaction(ctx context.Context, tx *components.PrivateTransaction) error {
	if tx.Inputs == nil || tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteAssembleTransaction)
	}

	// Clear any previous assembly state out, as it's considered completely invalid
	// at this point if we're re-assembling.
	preAssembly := tx.PreAssembly

	// Now we have the required verifiers, we can ask the domain to do the heavy lifting
	// and assemble the transaction (using the state store interface we provide)
	res, err := dc.api.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction:       preAssembly.TransactionSpecification,
		ResolvedVerifiers: preAssembly.Verifiers,
	})
	if err != nil {
		return err
	}

	postAssembly := &components.TransactionPostAssembly{}
	// We hydrate the states on our side of the Manager<->Plugin divide at this point,
	// which provides back to the engine the full sequence locking information of the
	// states (inputs, and read)
	postAssembly.InputStates, err = dc.loadStates(ctx, res.AssembledTransaction.InputStates)
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
	postAssembly.AttestationPlan = res.AttestationPlan
	// Note the states at this point are just potential states - depending on the analysis
	// of the result, and the locking on the input states, the engine might decide to
	// abandon this attempt and just re-assemble later.
	postAssembly.OutputStatesPotential = res.AssembledTransaction.OutputStates
	postAssembly.ExtraData = res.AssembledTransaction.ExtraData
	tx.PostAssembly = postAssembly
	return nil
}

// Happens only on the sequencing node
func (dc *domainContract) WritePotentialStates(ctx context.Context, tx *components.PrivateTransaction) (err error) {
	if tx.Inputs == nil || tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil || tx.PostAssembly == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteWritePotentialStates)
	}

	// Now we're confident enough about this transaction to (on the sequencer) to have allocated
	// it to a sequence, and we want to write the OutputStatesPotential array:
	// 1) Writing them to the DB (unflushed at this point)
	// 2) Storing their identifiers into the OutputStatesFull list
	//
	// Note: This only happens on the sequencer node - any endorsing nodes just take the Full states
	//       and write them directly to the sequence prior to endorsement
	postAssembly := tx.PostAssembly

	newStatesToWrite := make([]*components.StateUpsert, len(postAssembly.OutputStatesPotential))
	domain := dc.d
	for i, s := range postAssembly.OutputStatesPotential {
		schema := domain.schemasByID[s.SchemaId]
		if schema == nil {
			schema = domain.schemasBySignature[s.SchemaId]
		}
		if schema == nil {
			return i18n.NewError(ctx, msgs.MsgDomainUnknownSchema, s.SchemaId)
		}
		var id tktypes.HexBytes
		if s.Id != nil {
			id, err = tktypes.ParseHexBytes(ctx, *s.Id)
			if err != nil {
				return err
			}
		}
		newStatesToWrite[i] = &components.StateUpsert{
			ID:       id,
			SchemaID: schema.IDString(),
			Data:     tktypes.RawJSON(s.StateDataJson),
			// These are marked as locked and creating in the transaction
			Creating: true,
		}
	}

	var states []*components.State
	if len(newStatesToWrite) > 0 {
		err := dc.dm.stateStore.RunInDomainContext(domain.name, dc.info.Address, func(ctx context.Context, dsi components.DomainStateInterface) (err error) {
			states, err = dsi.UpsertStates(&tx.ID, newStatesToWrite)
			return err
		})
		if err != nil {
			return err
		}
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
func (dc *domainContract) LockStates(ctx context.Context, tx *components.PrivateTransaction) error {
	if tx.Inputs == nil || tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil ||
		tx.PostAssembly == nil || tx.PostAssembly.InputStates == nil || tx.PostAssembly.OutputStates == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteLockStates)
	}

	// Important responsibilities of this function
	// 1) to ensure all the states have been written (unflushed) to the DB, so that the calling code
	//    and be confident that at the end of the next successful flush we have reliably recorded
	//    all these private states in a way that we won't forget them in the future (including across crash/restart)
	// 2) to ensure all the states have been marked as "locked" for spending in this transaction,
	//    within this sequence. So that other transactions (on different sequences, or the same sequence)
	//    will not attempt to spend the same states.
	postAssembly := tx.PostAssembly

	// Input and output states are locked to the transaction
	var txLockedStateUpserts []*components.StateUpsert
	for _, s := range postAssembly.InputStates {
		txLockedStateUpserts = append(txLockedStateUpserts, &components.StateUpsert{
			SchemaID: s.Schema.String(),
			Data:     s.Data,
			Spending: true,
		})
	}
	for _, s := range postAssembly.OutputStates {
		txLockedStateUpserts = append(txLockedStateUpserts, &components.StateUpsert{
			SchemaID: s.Schema.String(),
			Data:     s.Data,
			Creating: true,
		})
	}

	// Read states are just stored
	var readStateUpserts []*components.StateUpsert
	for _, s := range postAssembly.ReadStates {
		readStateUpserts = append(readStateUpserts, &components.StateUpsert{
			SchemaID: s.Schema.String(),
			Data:     s.Data,
		})
	}

	return dc.dm.stateStore.RunInDomainContext(dc.d.name, dc.info.Address, func(ctx context.Context, dsi components.DomainStateInterface) error {
		// Heavy lifting is all done for us by the state store
		_, err := dsi.UpsertStates(&tx.ID, txLockedStateUpserts)
		if err == nil && len(readStateUpserts) > 0 {
			_, err = dsi.UpsertStates(nil, readStateUpserts)
		}
		return err
	})

}

// Endorse is a little special, because it returns a payload rather than updating the transaction.
func (dc *domainContract) EndorseTransaction(ctx context.Context, req *components.PrivateTransactionEndorseRequest) (*components.EndorsementResult, error) {

	if req == nil ||
		req.TransactionSpecification == nil ||
		req.Verifiers == nil ||
		req.Signatures == nil ||
		req.InputStates == nil ||
		req.ReadStates == nil ||
		req.OutputStates == nil ||
		req.Endorsement == nil ||
		req.Endorser == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainReqIncompleteEndorseTransaction)
	}

	// This function does NOT FLUSH before or after doing endorse. The assumption is that this
	// is being handled as part of an overall sequence of endorsements, and for performance it is
	// more important to reduce the total number of flushes (rather than focus on the latency of one TX).
	//
	// The engine must ensure the flush occurs before returning the endorsement back to the requester,
	// but for efficiency we can and should start the runtime exercise of endorsement + signing before
	// waiting for the DB TX to commit.

	// Run the endorsement
	res, err := dc.api.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:         req.TransactionSpecification,
		ResolvedVerifiers:   req.Verifiers,
		Inputs:              req.InputStates,
		Reads:               req.ReadStates,
		Outputs:             req.OutputStates,
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

func (dc *domainContract) ResolveDispatch(ctx context.Context, tx *components.PrivateTransaction) error {
	if tx.Inputs == nil || tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil ||
		tx.PostAssembly == nil || tx.PostAssembly.Endorsements == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteResolveDispatch)
	}

	for _, ar := range tx.PostAssembly.Endorsements {
		for _, c := range ar.Constraints {
			if c == prototk.AttestationResult_ENDORSER_MUST_SUBMIT {
				if tx.Signer != "" {
					// Multiple endorsers claiming it is an error
					return i18n.NewError(ctx, msgs.MsgDomainMultipleEndorsersSubmit)
				}
				tx.Signer = ar.Verifier.Lookup
			}
		}
	}
	if tx.Signer != "" {
		return nil
	}

	config := dc.d.Configuration()
	switch config.BaseLedgerSubmitConfig.SubmitMode {
	case prototk.BaseLedgerSubmitConfig_ONE_TIME_USE_KEYS:
		tx.Signer = config.BaseLedgerSubmitConfig.OneTimeUsePrefix + tx.ID.String()
		return nil
	case prototk.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION:
		// if there was an endorser, we'd have it above
		return i18n.NewError(ctx, msgs.MsgDomainNoEndorserSubmit)
	default:
		return i18n.NewError(ctx, msgs.MsgDomainInvalidSubmissionConfig, config.BaseLedgerSubmitConfig.SubmitMode)
	}

}

func (dc *domainContract) PrepareTransaction(ctx context.Context, tx *components.PrivateTransaction) error {
	if tx.Inputs == nil || tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil ||
		tx.PostAssembly == nil || tx.Signer == "" {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompletePrepareTransaction)
	}

	preAssembly := tx.PreAssembly
	postAssembly := tx.PostAssembly

	// Run the prepare
	res, err := dc.api.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       preAssembly.TransactionSpecification,
		InputStates:       dc.toEndorsableList(postAssembly.InputStates),
		ReadStates:        dc.toEndorsableList(postAssembly.ReadStates),
		OutputStates:      dc.toEndorsableList(postAssembly.OutputStates),
		AttestationResult: dc.allAttestations(tx),
		ResolvedVerifiers: preAssembly.Verifiers,
		ExtraData:         postAssembly.ExtraData,
	})
	if err != nil {
		return err
	}

	var functionABI abi.Entry
	if err := json.Unmarshal(([]byte)(res.Transaction.FunctionAbiJson), &functionABI); err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainPrivateAbiJsonInvalid)
	}
	inputs, err := functionABI.Inputs.ParseJSONCtx(ctx, emptyJSONIfBlank(res.Transaction.ParamsJson))
	if err != nil {
		return err
	}

	contractAddress := dc.Address()
	if res.Transaction.ContractAddress != nil {
		contractAddress = *tktypes.MustEthAddress(*res.Transaction.ContractAddress)
	}

	tx.PreparedTransaction = &components.EthTransaction{
		FunctionABI: &functionABI,
		To:          contractAddress,
		Inputs:      inputs,
	}
	return nil
}

func (dc *domainContract) Domain() components.Domain {
	return dc.d
}

func (dc *domainContract) Address() tktypes.EthAddress {
	return dc.info.Address
}

func (dc *domainContract) ConfigBytes() tktypes.HexBytes {
	return dc.info.ConfigBytes
}

func (dc *domainContract) allAttestations(tx *components.PrivateTransaction) []*prototk.AttestationResult {
	attestations := append([]*prototk.AttestationResult{}, tx.PostAssembly.Signatures...)
	attestations = append(attestations, tx.PostAssembly.Endorsements...)
	return attestations
}

func (dc *domainContract) loadStates(ctx context.Context, refs []*prototk.StateRef) ([]*components.FullState, error) {
	rawIDsBySchema := make(map[string][]tktypes.RawJSON)
	stateIDs := make([]tktypes.HexBytes, len(refs))
	for i, s := range refs {
		stateID, err := tktypes.ParseHexBytes(ctx, s.Id)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidStateIDFromDomain, s.Id, i)
		}
		rawIDsBySchema[s.SchemaId] = append(rawIDsBySchema[s.SchemaId], tktypes.JSONString(stateID.String()))
		stateIDs[i] = stateID
	}
	statesByID := make(map[string]*components.State)
	err := dc.dm.stateStore.RunInDomainContext(dc.d.name, dc.info.Address, func(ctx context.Context, dsi components.DomainStateInterface) error {
		for schemaID, stateIDs := range rawIDsBySchema {
			statesForSchema, err := dsi.FindAvailableStates(schemaID, &query.QueryJSON{
				Statements: query.Statements{
					Ops: query.Ops{
						In: []*query.OpMultiVal{
							{Op: query.Op{Field: ".id"}, Values: stateIDs},
						},
					},
				},
			})
			if err != nil {
				return err
			}
			for _, s := range statesForSchema {
				statesByID[s.ID.HexString()] = s
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
		s := statesByID[id.HexString()]
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

func (dc *domainContract) toEndorsableList(states []*components.FullState) []*prototk.EndorsableState {
	endorsableList := make([]*prototk.EndorsableState, len(states))
	for i, input := range states {
		endorsableList[i] = &prototk.EndorsableState{
			Id:            input.ID.String(),
			SchemaId:      input.Schema.String(),
			StateDataJson: string(input.Data),
		}
	}
	return endorsableList
}
