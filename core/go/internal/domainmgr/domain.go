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
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/statestore"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type domain struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf *DomainConfig
	dm   *domainManager
	id   uuid.UUID
	name string
	api  components.DomainManagerToDomain

	stateLock              sync.Mutex
	initialized            atomic.Bool
	initRetry              *retry.Retry
	config                 *prototk.DomainConfig
	schemasBySignature     map[string]statestore.Schema
	schemasByID            map[string]statestore.Schema
	constructorABI         *abi.Entry
	factoryContractAddress *tktypes.EthAddress

	initError atomic.Pointer[error]
	initDone  chan struct{}
}

func (dm *domainManager) newDomain(id uuid.UUID, name string, conf *DomainConfig, toDomain components.DomainManagerToDomain) *domain {
	d := &domain{
		dm:        dm,
		conf:      conf,
		initRetry: retry.NewRetryIndefinite(&conf.Init.Retry),
		name:      name,
		id:        id,
		api:       toDomain,
		initDone:  make(chan struct{}),

		schemasByID:        make(map[string]statestore.Schema),
		schemasBySignature: make(map[string]statestore.Schema),
	}
	log.L(dm.bgCtx).Debugf("Domain %s configured. Config: %s", name, tktypes.JSONString(conf.Config))
	d.ctx, d.cancelCtx = context.WithCancel(log.WithLogField(dm.bgCtx, "domain", d.name))
	return d
}

func (d *domain) processDomainConfig(confRes *prototk.ConfigureDomainResponse) (*prototk.InitDomainRequest, error) {
	d.stateLock.Lock()
	defer d.stateLock.Unlock()

	// Parse all the schemas
	d.config = confRes.DomainConfig
	if d.config.BaseLedgerSubmitConfig == nil {
		return nil, i18n.NewError(d.ctx, msgs.MsgDomainBaseLedgerSubmitInvalid)
	}
	abiSchemas := make([]*abi.Parameter, len(d.config.AbiStateSchemasJson))
	for i, schemaJSON := range d.config.AbiStateSchemasJson {
		if err := json.Unmarshal([]byte(schemaJSON), &abiSchemas[i]); err != nil {
			return nil, i18n.WrapError(d.ctx, err, msgs.MsgDomainInvalidSchema, i)
		}
	}

	err := json.Unmarshal(([]byte)(d.config.ConstructorAbiJson), &d.constructorABI)
	if err != nil {
		return nil, i18n.WrapError(d.ctx, err, msgs.MsgDomainConstructorAbiJsonInvalid)
	}
	if d.constructorABI.Type != abi.Constructor {
		return nil, i18n.NewError(d.ctx, msgs.MsgDomainConstructorABITypeWrong, d.constructorABI.Type)
	}

	d.factoryContractAddress, err = tktypes.ParseEthAddress(d.config.FactoryContractAddress)
	if err != nil {
		return nil, i18n.WrapError(d.ctx, err, msgs.MsgDomainFactoryAddressInvalid)
	}

	// Ensure all the schemas are recorded to the DB
	// This is a special case where we need a synchronous flush to ensure they're all established
	var schemas []statestore.Schema
	err = d.dm.stateStore.RunInDomainContextFlush(d.name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		schemas, err = dsi.EnsureABISchemas(abiSchemas)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Build the request to the init
	schemasProto := make([]*prototk.StateSchema, len(schemas))
	for i, s := range schemas {
		schemaID := s.IDString()
		d.schemasByID[schemaID] = s
		d.schemasBySignature[s.Signature()] = s
		schemasProto[i] = &prototk.StateSchema{
			Id:        schemaID,
			Signature: s.Signature(),
		}
	}
	return &prototk.InitDomainRequest{
		DomainUuid:      d.id.String(),
		AbiStateSchemas: schemasProto,
	}, nil
}

func (d *domain) init() {
	defer close(d.initDone)

	// We block retrying each part of init until we succeed, or are cancelled
	// (which the plugin manager will do if the domain disconnects)
	err := d.initRetry.Do(d.ctx, func(attempt int) (bool, error) {

		// Send the configuration to the domain for processing
		confRes, err := d.api.ConfigureDomain(d.ctx, &prototk.ConfigureDomainRequest{
			Name:       d.name,
			ChainId:    d.dm.ethClientFactory.ChainID(),
			ConfigJson: tktypes.JSONString(d.conf.Config).String(),
		})
		if err != nil {
			return true, err
		}

		// Process the configuration, so we can move onto init
		initReq, err := d.processDomainConfig(confRes)
		if err != nil {
			return true, err
		}

		// Complete the initialization
		_, err = d.api.InitDomain(d.ctx, initReq)
		return true, err
	})
	if err != nil {
		log.L(d.ctx).Debugf("domain initialization cancelled before completion: %s", err)
		d.initError.Store(&err)
	} else {
		log.L(d.ctx).Debugf("domain initialization complete")
		d.dm.setDomainAddress(d)
		d.initialized.Store(true)
		// Inform the plugin manager callback
		d.api.Initialized()
	}
}

func (d *domain) checkInit(ctx context.Context) error {
	if !d.initialized.Load() {
		return i18n.NewError(ctx, msgs.MsgDomainNotInitialized)
	}
	return nil
}

func (d *domain) Initialized() bool {
	return d.initialized.Load()
}

func (d *domain) Name() string {
	return d.name
}

func (d *domain) Address() *tktypes.EthAddress {
	return d.factoryContractAddress
}

func (d *domain) Configuration() *prototk.DomainConfig {
	return d.config
}

// Domain callback to query the state store
func (d *domain) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	if err := d.checkInit(ctx); err != nil {
		return nil, err
	}

	var query filters.QueryJSON
	err := json.Unmarshal([]byte(req.QueryJson), &query)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidQueryJSON)
	}

	var states []*statestore.State
	err = d.dm.stateStore.RunInDomainContext(d.name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		states, err = dsi.FindAvailableStates(req.SchemaId, &query)
		return err
	})
	if err != nil {
		return nil, err
	}

	pbStates := make([]*prototk.StoredState, len(states))
	for i, s := range states {
		pbStates[i] = &prototk.StoredState{
			Id:       s.ID.String(),
			SchemaId: s.Schema.String(),
			StoredAt: s.CreatedAt.UnixNano(),
			DataJson: string(s.Data),
		}
		if s.Locked != nil {
			pbStates[i].Lock = &prototk.StateLock{
				Transaction: s.Locked.Transaction.String(),
				Creating:    s.Locked.Creating,
				Spending:    s.Locked.Spending,
			}
		}
	}
	return &prototk.FindAvailableStatesResponse{
		States: pbStates,
	}, nil

}

func (d *domain) InitDeploy(ctx context.Context, tx *components.PrivateContractDeploy) error {
	if tx.Inputs == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteInitDeploy)
	}

	// Build the init request
	var abiJSON []byte
	var paramsJSON []byte
	constructorValues, err := d.constructorABI.Inputs.ParseJSONCtx(ctx, tx.Inputs)
	if err == nil {
		abiJSON, err = json.Marshal(d.constructorABI)
	}
	if err == nil {
		// Serialize to standardized JSON before passing to domain
		paramsJSON, err = tktypes.StandardABISerializer().SerializeJSONCtx(ctx, constructorValues)
	}
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainInvalidConstructorParams, d.constructorABI.SolString())
	}

	txSpec := &prototk.DeployTransactionSpecification{}
	tx.TransactionSpecification = txSpec
	txSpec.TransactionId = tktypes.Bytes32UUIDFirst16(tx.ID).String()
	txSpec.ConstructorAbi = string(abiJSON)
	txSpec.ConstructorParamsJson = string(paramsJSON)

	// Do the request with the domain
	res, err := d.api.InitDeploy(ctx, &prototk.InitDeployRequest{
		Transaction: txSpec,
	})
	if err != nil {
		return err
	}

	// Store the response back on the TX
	tx.RequiredVerifiers = res.RequiredVerifiers
	return nil
}

func (d *domain) PrepareDeploy(ctx context.Context, tx *components.PrivateContractDeploy) error {
	if tx.Inputs == nil || tx.TransactionSpecification == nil || tx.Verifiers == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompletePrepareDeploy)
	}

	// All the work is done for us by the engine in resolving the verifiers
	// after InitDeploy, so we just pass it along
	res, err := d.api.PrepareDeploy(ctx, &prototk.PrepareDeployRequest{
		Transaction:       tx.TransactionSpecification,
		ResolvedVerifiers: tx.Verifiers,
	})
	if err != nil {
		return err
	}

	if res.Signer != nil && *res.Signer != "" {
		tx.Signer = *res.Signer
	} else {
		switch d.config.BaseLedgerSubmitConfig.SubmitMode {
		case prototk.BaseLedgerSubmitConfig_ONE_TIME_USE_KEYS:
			tx.Signer = d.config.BaseLedgerSubmitConfig.OneTimeUsePrefix + tx.ID.String()
		default:
			log.L(ctx).Errorf("Signer mode %s and no signer returned", d.config.BaseLedgerSubmitConfig.SubmitMode)
			return i18n.NewError(ctx, msgs.MsgDomainDeployNoSigner)
		}
	}
	if res.Transaction != nil && res.Deploy == nil {
		var contractABI abi.ABI
		if err := json.Unmarshal(([]byte)(res.Transaction.ContractAbiJson), &contractABI); err != nil {
			return i18n.WrapError(d.ctx, err, msgs.MsgDomainFactoryAbiJsonInvalid)
		}
		functionABI := contractABI.Functions()[res.Transaction.FunctionName]
		if functionABI == nil {
			return i18n.NewError(ctx, msgs.MsgDomainFunctionNotFound, res.Transaction.FunctionName)
		}
		inputs, err := functionABI.Inputs.ParseJSONCtx(ctx, emptyJSONIfBlank(res.Transaction.ParamsJson))
		if err != nil {
			return err
		}
		tx.DeployTransaction = nil
		tx.InvokeTransaction = &components.EthTransaction{
			FunctionABI: functionABI,
			To:          *d.Address(),
			Inputs:      inputs,
		}
	} else if res.Deploy != nil && res.Transaction == nil {
		var contractABI abi.ABI
		if err := json.Unmarshal(([]byte)(res.Deploy.ContractAbiJson), &contractABI); err != nil {
			return i18n.WrapError(d.ctx, err, msgs.MsgDomainFactoryAbiJsonInvalid)
		}
		functionABI := contractABI.Constructor()
		if functionABI == nil {
			// default constructor
			functionABI = &abi.Entry{Type: abi.Constructor, Inputs: abi.ParameterArray{}}
		}
		inputs, err := functionABI.Inputs.ParseJSONCtx(ctx, emptyJSONIfBlank(res.Deploy.ParamsJson))
		if err != nil {
			return err
		}
		tx.DeployTransaction = &components.EthDeployTransaction{
			ConstructorABI: functionABI,
			Bytecode:       res.Deploy.Bytecode,
			Inputs:         inputs,
		}
		tx.InvokeTransaction = nil
	} else {
		// Must specify exactly one of the two types of transaction
		return i18n.NewError(ctx, msgs.MsgDomainInvalidPrepareDeployResult)
	}
	return nil
}

func emptyJSONIfBlank(js string) []byte {
	if len(js) == 0 {
		return []byte(`{}`)
	}
	return []byte(js)
}

func (d *domain) close() {
	d.cancelCtx()
	<-d.initDone
}
