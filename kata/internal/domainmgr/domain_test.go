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
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const fakeCoinConstructorABI = `{
	"type": "constructor",
	"inputs": [
	  {
		"name": "notary",
		"type": "string"
	  },
	  {
		"name": "name",
		"type": "string"
	  },
	  {
		"name": "symbol",
		"type": "string"
	  }
	],
	"outputs": null
}`

const fakeCoinStateSchema = `{
	"type": "tuple",
	"internalType": "struct FakeCoin",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "owner",
			"type": "address",
			"indexed": true
		},
		{
			"name": "amount",
			"type": "uint256",
			"indexed": true
		}
	]
}`

type testPlugin struct {
	plugintk.DomainAPIBase
	initialized  atomic.Bool
	d            *domain
	stateSchemas []*prototk.StateSchema
}

func (tp *testPlugin) Initialized() {
	tp.initialized.Store(true)
}

func newTestPlugin(domainFuncs *plugintk.DomainAPIFunctions) *testPlugin {
	return &testPlugin{
		DomainAPIBase: plugintk.DomainAPIBase{
			Functions: domainFuncs,
		},
	}
}

func newTestDomain(t *testing.T, realDB bool, domainConfig *prototk.DomainConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *domainManager, *testPlugin, func()) {

	ctx, dm, _, done := newTestDomainManager(t, realDB, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{
			"test1": {
				Config: yamlNode(t, `{"some":"conf"}`),
			},
		},
	}, extraSetup...)

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			assert.Equal(t, "test1", cdr.Name)
			assert.YAMLEq(t, `{"some":"conf"}`, cdr.ConfigYaml)
			return &prototk.ConfigureDomainResponse{
				DomainConfig: domainConfig,
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			tp.stateSchemas = idr.AbiStateSchemas
			return &prototk.InitDomainResponse{}, nil
		},
	}

	registerTestDomain(t, dm, tp)
	return ctx, dm, tp, done
}

func registerTestDomain(t *testing.T, dm *domainManager, tp *testPlugin) {
	domainID := uuid.New()
	_, err := dm.DomainRegistered("test1", domainID, tp)
	assert.NoError(t, err)

	da, err := dm.GetDomainByName(context.Background(), "test1")
	assert.NoError(t, err)
	tp.d = da.(*domain)
	tp.d.initRetry.UTSetMaxAttempts(1)
	<-tp.d.initDone
}

func goodDomainConf() *prototk.DomainConfig {
	return &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		PrivateContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	}
}

func TestDomainInitStates(t *testing.T) {

	domainConf := goodDomainConf()
	ctx, dm, tp, done := newTestDomain(t, true, domainConf)
	defer done()

	assert.Nil(t, tp.d.initError.Load())
	assert.True(t, tp.initialized.Load())
	byAddr, err := dm.getDomainByAddress(ctx, types.MustEthAddress(domainConf.FactoryContractAddress))
	assert.NoError(t, err)
	assert.Equal(t, tp.d, byAddr)

}

func TestDoubleRegisterReplaces(t *testing.T) {

	domainConf := goodDomainConf()
	ctx, dm, tp0, done := newTestDomain(t, false, domainConf, func(mc *mockComponents) {
		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return([]statestore.Schema{}, nil)
	})
	defer done()
	assert.Nil(t, tp0.d.initError.Load())
	assert.True(t, tp0.initialized.Load())

	// Register again
	tp1 := newTestPlugin(nil)
	tp1.Functions = tp0.Functions
	registerTestDomain(t, dm, tp1)
	assert.Nil(t, tp1.d.initError.Load())
	assert.True(t, tp1.initialized.Load())

	// Check we get the second from all the maps
	byAddr, err := dm.getDomainByAddress(ctx, types.MustEthAddress(domainConf.FactoryContractAddress))
	assert.NoError(t, err)
	assert.Same(t, tp1.d, byAddr)
	byName, err := dm.GetDomainByName(ctx, "test1")
	assert.NoError(t, err)
	assert.Same(t, tp1.d, byName)
	byUUID := dm.domainsByID[tp1.d.id]
	assert.NoError(t, err)
	assert.Same(t, tp1.d, byUUID)

}

func TestDomainInitBadSchemas(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		PrivateContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			`!!! Wrong`,
		},
	})
	defer done()
	assert.Regexp(t, "PD011602", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitBadConstructor(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     `!!!wrong`,
		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		PrivateContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011603", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitBadConstructorType(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     `{"type":"event"}`,
		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		PrivateContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011604", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitSchemaStoreFail(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     `{"type":"event"}`,
		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		PrivateContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011604", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitBadAddress(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: `!wrong`,
		FactoryContractAbiJson: `[]`,
		PrivateContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011606", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitFactoryABIInvalid(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `!!!wrong`,
		PrivateContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011605", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitPrivateABIInvalid(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		PrivateContractAbiJson: `!!!wrong`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011607", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitFactorySchemaStoreFail(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		PrivateContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	}, func(mc *mockComponents) {
		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Regexp(t, "pop", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainConfigureFail(t *testing.T) {

	ctx, dm, _, done := newTestDomainManager(t, false, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{
			"test1": {
				Config: yamlNode(t, `{"some":"conf"}`),
			},
		},
	})
	defer done()

	tp := newTestPlugin(&plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	})

	domainID := uuid.New()
	_, err := dm.DomainRegistered("test1", domainID, tp)
	assert.NoError(t, err)

	da, err := dm.GetDomainByName(ctx, "test1")
	assert.NoError(t, err)

	d := da.(*domain)
	d.initRetry.UTSetMaxAttempts(1)
	<-d.initDone
	assert.Regexp(t, "pop", *d.initError.Load())
}

func TestDomainFindAvailableStatesNotInit(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		FactoryContractAbiJson: `!!!WRONG`,
	})
	defer done()
	assert.NotNil(t, *tp.d.initError.Load())
	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{SchemaId: "12345"})
	assert.Regexp(t, "PD011601", err)
}

func TestDomainFindAvailableStatesBadQuery(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return([]statestore.Schema{}, nil)
	})
	defer done()
	assert.Nil(t, tp.d.initError.Load())
	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId:  "12345",
		QueryJson: `!!!{ wrong`,
	})
	assert.Regexp(t, "PD011608", err)
}

func TestDomainFindAvailableStatesFail(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return([]statestore.Schema{}, nil)
		mc.domainStateInterface.On("FindAvailableStates", "12345", mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Nil(t, tp.d.initError.Load())
	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId:  "12345",
		QueryJson: `{}`,
	})
	assert.Regexp(t, "pop", err)
}

func TestDomainFindAvailableStatesOK(t *testing.T) {
	ctx, dm, tp, done := newTestDomain(t, true /* use real state store for this one */, goodDomainConf())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	err := dm.stateStore.RunInDomainContextFlush("test1", func(ctx context.Context, dsi statestore.DomainStateInterface) error {
		newStates, err := dsi.CreateNewStates(uuid.New(), []*statestore.NewState{
			{
				SchemaID: tp.stateSchemas[0].Id,
				Data: types.RawJSON(`{
					"salt": "5541b2383d8e2726d9318a29b62a44717535e3204257c698ce60c7c8ff093953",
					"owner": "0x8d06f71D68216b31e9019C162528241F44fA0fD9",
					"amount": "0x3033"
				}`),
			},
		})
		assert.Len(t, newStates, 1)
		return err
	})
	assert.NoError(t, err)

	// Filter match
	states, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId: tp.stateSchemas[0].Id,
		QueryJson: `{
		  "eq": [
		    { "field": "owner", "value": "0x8d06f71D68216b31e9019C162528241F44fA0fD9" }
		  ]
		}`,
	})
	assert.NoError(t, err)
	assert.Len(t, states.States, 1)

	// Filter miss
	states, err = tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId: tp.stateSchemas[0].Id,
		QueryJson: `{
		  "eq": [
		    { "field": "owner", "value": "0xc2C6aABDEb29cB53F164a3d631Af5CDC32A942BF" }
		  ]
		}`,
	})
	assert.NoError(t, err)
	assert.Len(t, states.States, 0)
}
