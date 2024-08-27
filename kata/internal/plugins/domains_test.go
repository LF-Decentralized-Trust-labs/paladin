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
package plugins

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

type testDomainManager struct {
	domains             map[string]plugintk.Plugin
	domainRegistered    func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (fromDomain plugintk.DomainCallbacks, err error)
	findAvailableStates func(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error)
}

func (tp *testDomainManager) ConfiguredDomains() map[string]*PluginConfig {
	pluginMap := make(map[string]*PluginConfig)
	for name := range tp.domains {
		pluginMap[name] = &PluginConfig{
			Type:     LibraryTypeCShared.Enum(),
			Location: "/tmp/not/applicable",
		}
	}
	return pluginMap
}

func (tp *testDomainManager) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	return tp.findAvailableStates(ctx, req)
}

func (tdm *testDomainManager) DomainRegistered(name string, id uuid.UUID, toDomain DomainManagerToDomain) (fromDomain plugintk.DomainCallbacks, err error) {
	return tdm.domainRegistered(name, id, toDomain)
}

func TestDomainRequestsOK(t *testing.T) {

	waitForAPI := make(chan DomainManagerToDomain, 1)
	waitForCallbacks := make(chan plugintk.DomainCallbacks, 1)

	var domainID string
	domainFunctions := &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			assert.Equal(t, int64(12345), cdr.ChainId)
			return &prototk.ConfigureDomainResponse{
				DomainConfig: &prototk.DomainConfig{
					ConstructorAbiJson: "ABI1",
				},
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			assert.Equal(t, domainID, idr.DomainUuid)
			return &prototk.InitDomainResponse{}, nil
		},
		InitDeploy: func(ctx context.Context, idr *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
			assert.Equal(t, "deploy_tx1_init", idr.Transaction.TransactionId)
			return &prototk.InitDeployResponse{RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "lookup1"},
			}}, nil
		},
		PrepareDeploy: func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
			assert.Equal(t, "deploy_tx1_prepare", pdr.Transaction.TransactionId)
			return &prototk.PrepareDeployResponse{
				Signer: confutil.P("signing1"),
			}, nil
		},
		InitTransaction: func(ctx context.Context, itr *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
			assert.Equal(t, "tx2_init", itr.Transaction.TransactionId)
			return &prototk.InitTransactionResponse{
				RequiredVerifiers: []*prototk.ResolveVerifierRequest{
					{Lookup: "lookup2"},
				},
			}, nil
		},
		AssembleTransaction: func(ctx context.Context, atr *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
			assert.Equal(t, "tx2_prepare", atr.Transaction.TransactionId)
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			}, nil
		},
		EndorseTransaction: func(ctx context.Context, etr *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
			assert.Equal(t, "tx2_endorse", etr.Transaction.TransactionId)
			return &prototk.EndorseTransactionResponse{
				EndorsementResult: prototk.EndorseTransactionResponse_SIGN,
			}, nil
		},
		PrepareTransaction: func(ctx context.Context, ptr *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
			assert.Equal(t, "tx2_prepare", ptr.Transaction.TransactionId)
			return &prototk.PrepareTransactionResponse{
				Transaction: &prototk.BaseLedgerTransaction{
					FunctionName: "func1",
				},
			}, nil
		},
	}

	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
				waitForCallbacks <- callbacks
				return &plugintk.DomainAPIBase{Functions: domainFunctions}
			}),
		},
	}
	tdm.domainRegistered = func(name string, id uuid.UUID, toDomain DomainManagerToDomain) (plugintk.DomainCallbacks, error) {
		assert.Equal(t, "domain1", name)
		domainID = id.String()
		waitForAPI <- toDomain
		return tdm, nil
	}

	tdm.findAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		assert.Equal(t, "schema1", req.SchemaId)
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{Id: "12345"},
			},
		}, nil
	}

	ctx, pc, done := newTestDomainPluginController(t, &testManagers{
		testDomainManager: tdm,
	})
	defer done()

	domainAPI := <-waitForAPI

	cdr, err := domainAPI.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{
		ChainId: int64(12345),
	})
	assert.NoError(t, err)
	assert.Equal(t, "ABI1", cdr.DomainConfig.ConstructorAbiJson)

	_, err = domainAPI.InitDomain(ctx, &prototk.InitDomainRequest{
		DomainUuid: domainID,
	})
	assert.NoError(t, err)

	// This is the point the domain manager would call us to say the domain is initialized
	// (once it's happy it's updated its internal state)
	domainAPI.Initialized()
	assert.NoError(t, pc.WaitForInit(ctx))

	idr, err := domainAPI.InitDeploy(ctx, &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			TransactionId: "deploy_tx1_init",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "lookup1", idr.RequiredVerifiers[0].Lookup)

	pdr, err := domainAPI.PrepareDeploy(ctx, &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			TransactionId: "deploy_tx1_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "signing1", *pdr.Signer)

	itr, err := domainAPI.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_init",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "lookup2", itr.RequiredVerifiers[0].Lookup)

	atr, err := domainAPI.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, atr.AssemblyResult)

	etr, err := domainAPI.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_endorse",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_SIGN, etr.EndorsementResult)

	ptr, err := domainAPI.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "tx2_prepare",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "func1", ptr.Transaction.FunctionName)

	callbacks := <-waitForCallbacks
	fas, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId: "schema1",
	})
	assert.NoError(t, err)
	assert.Equal(t, "12345", fas.States[0].Id)
}
