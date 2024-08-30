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

package transportmgr

// import (
// 	"context"
// 	"fmt"
// 	"sync/atomic"
// 	"testing"

// 	"github.com/google/uuid"
// 	"github.com/kaleido-io/paladin/kata/internal/statestore"
// 	"github.com/kaleido-io/paladin/kata/pkg/types"
// 	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
// 	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/mock"
// )

// type testPlugin struct {
// 	plugintk.TransportAPIBase
// 	initialized  atomic.Bool
// 	d            *transport
// 	stateSchemas []*prototk.StateSchema
// }

// func (tp *testPlugin) Initialized() {
// 	tp.initialized.Store(true)
// }

// func newTestPlugin(transportFuncs *plugintk.TransportAPIFunctions) *testPlugin {
// 	return &testPlugin{
// 		TransportAPIBase: plugintk.TransportAPIBase{
// 			Functions: transportFuncs,
// 		},
// 	}
// }

// func newTestTransport(t *testing.T, realDB bool, transportConfig *prototk.TransportConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *transportManager, *testPlugin, func()) {
// 	ctx, dm, _, done := newTestTransportManager(t, realDB, &TransportManagerConfig{
// 		Transports: map[string]*TransportConfig{
// 			"test1": {
// 				Config: yamlNode(t, `{"some":"conf"}`),
// 			},
// 		},
// 	}, extraSetup...)

// 	tp := newTestPlugin(nil)
// 	tp.Functions = &plugintk.TransportAPIFunctions{
// 		ConfigureTransport: func(ctx context.Context, cdr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
// 			assert.Equal(t, "test1", cdr.Name)
// 			assert.YAMLEq(t, `{"some":"conf"}`, cdr.ConfigYaml)
// 			return &prototk.ConfigureTransportResponse{
// 				TransportConfig: transportConfig,
// 			}, nil
// 		},
// 		InitTransport: func(ctx context.Context, idr *prototk.InitTransportRequest) (*prototk.InitTransportResponse, error) {
// 			return &prototk.InitTransportResponse{}, nil
// 		},
// 	}

// 	registerTestTransport(t, dm, tp)
// 	return ctx, dm, tp, done
// }

// func registerTestTransport(t *testing.T, dm *transportManager, tp *testPlugin) {
// 	transportID := uuid.New()
// 	_, err := dm.TransportRegistered("test1", transportID, tp)
// 	assert.NoError(t, err)

// 	da, err := dm.GetTransportByName(context.Background(), "test1")
// 	assert.NoError(t, err)
// 	tp.d = da.(*transport)
// 	tp.d.initRetry.UTSetMaxAttempts(1)
// 	<-tp.d.initDone
// }

// func goodTransportConf() *prototk.TransportConfig {
// 	return &prototk.TransportConfig{
// 		// BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{
// 		// 	SubmitMode: prototk.BaseLedgerSubmitConfig_ONE_TIME_USE_KEYS,
// 		// },
// 		// ConstructorAbiJson:     fakeCoinConstructorABI,
// 		// FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
// 		// FactoryContractAbiJson: `[]`,
// 		// PrivateContractAbiJson: `[]`,
// 		// AbiStateSchemasJson: []string{
// 		// 	fakeCoinStateSchema,
// 		// },
// 	}
// }

// func TestTransportInitStates(t *testing.T) {

// 	transportConf := goodTransportConf()
// 	ctx, dm, tp, done := newTestTransport(t, true, transportConf)
// 	defer done()

// 	assert.Nil(t, tp.d.initError.Load())
// 	assert.True(t, tp.initialized.Load())
// 	byAddr, err := dm.getTransportByAddress(ctx, types.MustEthAddress(transportConf.FactoryContractAddress))
// 	assert.NoError(t, err)
// 	assert.Equal(t, tp.d, byAddr)

// }

// func TestDoubleRegisterReplaces(t *testing.T) {

// 	transportConf := goodTransportConf()
// 	ctx, dm, tp0, done := newTestTransport(t, false, transportConf, func(mc *mockComponents) {
// 		mc.transportStateInterface.On("EnsureABISchemas", mock.Anything).Return([]statestore.Schema{}, nil)
// 	})
// 	defer done()
// 	assert.Nil(t, tp0.d.initError.Load())
// 	assert.True(t, tp0.initialized.Load())

// 	// Register again
// 	tp1 := newTestPlugin(nil)
// 	tp1.Functions = tp0.Functions
// 	registerTestTransport(t, dm, tp1)
// 	assert.Nil(t, tp1.d.initError.Load())
// 	assert.True(t, tp1.initialized.Load())

// 	// Check we get the second from all the maps
// 	byAddr, err := dm.getTransportByAddress(ctx, types.MustEthAddress(transportConf.FactoryContractAddress))
// 	assert.NoError(t, err)
// 	assert.Same(t, tp1.d, byAddr)
// 	byName, err := dm.GetTransportByName(ctx, "test1")
// 	assert.NoError(t, err)
// 	assert.Same(t, tp1.d, byName)
// 	byUUID := dm.transportsByID[tp1.d.id]
// 	assert.NoError(t, err)
// 	assert.Same(t, tp1.d, byUUID)

// }

// func TestTransportInitBadSchemas(t *testing.T) {
// 	_, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
// 		ConstructorAbiJson:     fakeCoinConstructorABI,
// 		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
// 		FactoryContractAbiJson: `[]`,
// 		PrivateContractAbiJson: `[]`,
// 		AbiStateSchemasJson: []string{
// 			`!!! Wrong`,
// 		},
// 	})
// 	defer done()
// 	assert.Regexp(t, "PD011602", *tp.d.initError.Load())
// 	assert.False(t, tp.initialized.Load())
// }

// func TestTransportInitBadConstructor(t *testing.T) {
// 	_, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
// 		ConstructorAbiJson:     `!!!wrong`,
// 		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
// 		FactoryContractAbiJson: `[]`,
// 		PrivateContractAbiJson: `[]`,
// 		AbiStateSchemasJson: []string{
// 			fakeCoinStateSchema,
// 		},
// 	})
// 	defer done()
// 	assert.Regexp(t, "PD011603", *tp.d.initError.Load())
// 	assert.False(t, tp.initialized.Load())
// }

// func TestTransportInitBadConstructorType(t *testing.T) {
// 	_, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
// 		ConstructorAbiJson:     `{"type":"event"}`,
// 		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
// 		FactoryContractAbiJson: `[]`,
// 		PrivateContractAbiJson: `[]`,
// 		AbiStateSchemasJson: []string{
// 			fakeCoinStateSchema,
// 		},
// 	})
// 	defer done()
// 	assert.Regexp(t, "PD011604", *tp.d.initError.Load())
// 	assert.False(t, tp.initialized.Load())
// }

// func TestTransportInitSchemaStoreFail(t *testing.T) {
// 	_, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
// 		ConstructorAbiJson:     `{"type":"event"}`,
// 		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
// 		FactoryContractAbiJson: `[]`,
// 		PrivateContractAbiJson: `[]`,
// 		AbiStateSchemasJson: []string{
// 			fakeCoinStateSchema,
// 		},
// 	})
// 	defer done()
// 	assert.Regexp(t, "PD011604", *tp.d.initError.Load())
// 	assert.False(t, tp.initialized.Load())
// }

// func TestTransportInitBadAddress(t *testing.T) {
// 	_, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
// 		ConstructorAbiJson:     fakeCoinConstructorABI,
// 		FactoryContractAddress: `!wrong`,
// 		FactoryContractAbiJson: `[]`,
// 		PrivateContractAbiJson: `[]`,
// 		AbiStateSchemasJson: []string{
// 			fakeCoinStateSchema,
// 		},
// 	})
// 	defer done()
// 	assert.Regexp(t, "PD011606", *tp.d.initError.Load())
// 	assert.False(t, tp.initialized.Load())
// }

// func TestTransportInitFactoryABIInvalid(t *testing.T) {
// 	_, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
// 		ConstructorAbiJson:     fakeCoinConstructorABI,
// 		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
// 		FactoryContractAbiJson: `!!!wrong`,
// 		PrivateContractAbiJson: `[]`,
// 		AbiStateSchemasJson: []string{
// 			fakeCoinStateSchema,
// 		},
// 	})
// 	defer done()
// 	assert.Regexp(t, "PD011605", *tp.d.initError.Load())
// 	assert.False(t, tp.initialized.Load())
// }

// func TestTransportInitPrivateABIInvalid(t *testing.T) {
// 	_, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
// 		ConstructorAbiJson:     fakeCoinConstructorABI,
// 		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
// 		FactoryContractAbiJson: `[]`,
// 		PrivateContractAbiJson: `!!!wrong`,
// 		AbiStateSchemasJson: []string{
// 			fakeCoinStateSchema,
// 		},
// 	})
// 	defer done()
// 	assert.Regexp(t, "PD011607", *tp.d.initError.Load())
// 	assert.False(t, tp.initialized.Load())
// }

// func TestTransportInitFactorySchemaStoreFail(t *testing.T) {
// 	_, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
// 		ConstructorAbiJson:     fakeCoinConstructorABI,
// 		FactoryContractAddress: types.MustEthAddress(types.RandHex(20)).String(),
// 		FactoryContractAbiJson: `[]`,
// 		PrivateContractAbiJson: `[]`,
// 		AbiStateSchemasJson: []string{
// 			fakeCoinStateSchema,
// 		},
// 	}, func(mc *mockComponents) {
// 		mc.transportStateInterface.On("EnsureABISchemas", mock.Anything).Return(nil, fmt.Errorf("pop"))
// 	})
// 	defer done()
// 	assert.Regexp(t, "pop", *tp.d.initError.Load())
// 	assert.False(t, tp.initialized.Load())
// }

// func TestTransportConfigureFail(t *testing.T) {

// 	ctx, dm, _, done := newTestTransportManager(t, false, &TransportManagerConfig{
// 		Transports: map[string]*TransportConfig{
// 			"test1": {
// 				Config: yamlNode(t, `{"some":"conf"}`),
// 			},
// 		},
// 	})
// 	defer done()

// 	tp := newTestPlugin(&plugintk.TransportAPIFunctions{
// 		ConfigureTransport: func(ctx context.Context, cdr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
// 			return nil, fmt.Errorf("pop")
// 		},
// 	})

// 	transportID := uuid.New()
// 	_, err := dm.TransportRegistered("test1", transportID, tp)
// 	assert.NoError(t, err)

// 	da, err := dm.GetTransportByName(ctx, "test1")
// 	assert.NoError(t, err)

// 	d := da.(*transport)
// 	d.initRetry.UTSetMaxAttempts(1)
// 	<-d.initDone
// 	assert.Regexp(t, "pop", *d.initError.Load())
// }

// func TestTransportFindAvailableStatesNotInit(t *testing.T) {
// 	ctx, _, tp, done := newTestTransport(t, false, &prototk.TransportConfig{
// 		FactoryContractAbiJson: `!!!WRONG`,
// 	})
// 	defer done()
// 	assert.NotNil(t, *tp.d.initError.Load())
// 	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{SchemaId: "12345"})
// 	assert.Regexp(t, "PD011601", err)
// }

// func TestTransportFindAvailableStatesBadQuery(t *testing.T) {
// 	ctx, _, tp, done := newTestTransport(t, false, goodTransportConf(), func(mc *mockComponents) {
// 		mc.transportStateInterface.On("EnsureABISchemas", mock.Anything).Return([]statestore.Schema{}, nil)
// 	})
// 	defer done()
// 	assert.Nil(t, tp.d.initError.Load())
// 	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
// 		SchemaId:  "12345",
// 		QueryJson: `!!!{ wrong`,
// 	})
// 	assert.Regexp(t, "PD011608", err)
// }

// func TestTransportFindAvailableStatesFail(t *testing.T) {
// 	ctx, _, tp, done := newTestTransport(t, false, goodTransportConf(), func(mc *mockComponents) {
// 		mc.transportStateInterface.On("EnsureABISchemas", mock.Anything).Return([]statestore.Schema{}, nil)
// 		mc.transportStateInterface.On("FindAvailableStates", "12345", mock.Anything).Return(nil, fmt.Errorf("pop"))
// 	})
// 	defer done()
// 	assert.Nil(t, tp.d.initError.Load())
// 	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
// 		SchemaId:  "12345",
// 		QueryJson: `{}`,
// 	})
// 	assert.Regexp(t, "pop", err)
// }

// func TestTransportFindAvailableStatesOK(t *testing.T) {
// 	ctx, dm, tp, done := newTestTransport(t, true /* use real state store for this one */, goodTransportConf())
// 	defer done()
// 	assert.Nil(t, tp.d.initError.Load())

// 	txID := uuid.New()
// 	err := dm.stateStore.RunInTransportContextFlush("test1", func(ctx context.Context, dsi statestore.TransportStateInterface) error {
// 		newStates, err := dsi.UpsertStates(&txID, []*statestore.StateUpsert{
// 			{
// 				SchemaID: tp.stateSchemas[0].Id,
// 				Data: types.RawJSON(`{
// 					"salt": "5541b2383d8e2726d9318a29b62a44717535e3204257c698ce60c7c8ff093953",
// 					"owner": "0x8d06f71D68216b31e9019C162528241F44fA0fD9",
// 					"amount": "0x3033"
// 				}`),
// 				Creating: true,
// 			},
// 		})
// 		assert.Len(t, newStates, 1)
// 		return err
// 	})
// 	assert.NoError(t, err)

// 	// Filter match
// 	states, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
// 		SchemaId: tp.stateSchemas[0].Id,
// 		QueryJson: `{
// 		"eq": [
// 			{ "field": "owner", "value": "0x8d06f71D68216b31e9019C162528241F44fA0fD9" }
// 		]
// 		}`,
// 	})
// 	assert.NoError(t, err)
// 	assert.Len(t, states.States, 1)

// 	// Filter miss
// 	states, err = tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
// 		SchemaId: tp.stateSchemas[0].Id,
// 		QueryJson: `{
// 		"eq": [
// 			{ "field": "owner", "value": "0xc2C6aABDEb29cB53F164a3d631Af5CDC32A942BF" }
// 		]
// 		}`,
// 	})
// 	assert.NoError(t, err)
// 	assert.Len(t, states.States, 0)
// }
