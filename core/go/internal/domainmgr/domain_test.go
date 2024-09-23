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
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/statestore"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/persistence/mockpersistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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

const fakeCoinExecuteABI = `{
	"type": "function",
	"name": "execute",
	"inputs": [
		{
			"name": "inputs",
			"type": "bytes32[]"
		},
		{
			"name": "outputs",
			"type": "bytes32[]"
		},
		{
			"name": "data",
			"type": "bytes"
		}
	],
	"outputs": null
}`

const fakeCoinFactoryConstructorABI = `{
	"type": "constructor",
	"inputs": [
		{
			"name": "notary",
			"type": "address"
		},
		{
			"name": "data",
			"type": "bytes"
		}
	],
	"outputs": null
}`

const fakeCoinFactoryNewInstanceABI = `{
	"type": "function",
	"name": "newInstance",
	"inputs": [
		{
			"name": "notary",
			"type": "address"
		},
		{
			"name": "data",
			"type": "bytes"
		}
	],
	"outputs": null
}`

const fakeCoinEventsABI = `[{
	"type": "event",
    "name": "Transfer",
	"inputs": [
		{
			"name": "inputs",
			"type": "bytes32[]"
		},
		{
			"name": "outputs",
			"type": "bytes32[]"
		},
		{
			"name": "data",
			"type": "bytes"
		}
	]
}]`

type fakeState struct {
	Salt   tktypes.Bytes32      `json:"salt"`
	Owner  tktypes.EthAddress   `json:"owner"`
	Amount *ethtypes.HexInteger `json:"amount"`
}

type fakeExecute struct {
	Inputs  []tktypes.Bytes32 `json:"inputs"`
	Outputs []tktypes.Bytes32 `json:"outputs"`
	Data    tktypes.HexBytes  `json:"data"`
}

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
				Config:          map[string]any{"some": "conf"},
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	}, extraSetup...)

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			assert.Equal(t, "test1", cdr.Name)
			assert.JSONEq(t, `{"some":"conf"}`, cdr.ConfigJson)
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
	_, err := dm.DomainRegistered("test1", tp)
	require.NoError(t, err)

	da, err := dm.GetDomainByName(context.Background(), "test1")
	require.NoError(t, err)
	tp.d = da.(*domain)
	tp.d.initRetry.UTSetMaxAttempts(1)
	<-tp.d.initDone
}

func goodDomainConf() *prototk.DomainConfig {
	return &prototk.DomainConfig{
		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{
			SubmitMode:       prototk.BaseLedgerSubmitConfig_ONE_TIME_USE_KEYS,
			OneTimeUsePrefix: "one/time/keys/",
		},
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	}
}

func mockSchemas(schemas ...statestore.Schema) func(mc *mockComponents) {
	return func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, "test1", mock.Anything).Return(schemas, nil)
	}
}

func TestDomainInitStates(t *testing.T) {

	domainConf := goodDomainConf()
	ctx, dm, tp, done := newTestDomain(t, true, domainConf)
	defer done()

	assert.Nil(t, tp.d.initError.Load())
	assert.True(t, tp.initialized.Load())
	byAddr, err := dm.getDomainByAddress(ctx, tp.d.RegistryAddress())
	require.NoError(t, err)
	assert.Equal(t, tp.d, byAddr)
	assert.True(t, tp.d.Initialized())
	assert.NotNil(t, tp.d.Configuration().BaseLedgerSubmitConfig)

}

func TestDomainInitStatesWithEvents(t *testing.T) {

	domainConf := goodDomainConf()
	domainConf.AbiEventsJson = fakeCoinEventsABI
	ctx, dm, tp, done := newTestDomain(t, true, domainConf, func(mc *mockComponents) {
		mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything).Return(nil, nil)
	})
	defer done()

	assert.Nil(t, tp.d.initError.Load())
	assert.True(t, tp.initialized.Load())
	byAddr, err := dm.getDomainByAddress(ctx, tp.d.RegistryAddress())
	require.NoError(t, err)
	assert.Equal(t, tp.d, byAddr)
	assert.True(t, tp.d.Initialized())
	assert.NotNil(t, tp.d.Configuration().BaseLedgerSubmitConfig)

}

func TestDoubleRegisterReplaces(t *testing.T) {

	domainConf := goodDomainConf()
	ctx, dm, tp0, done := newTestDomain(t, false, domainConf, mockSchemas())
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
	byAddr, err := dm.getDomainByAddress(ctx, tp0.d.RegistryAddress())
	require.NoError(t, err)
	assert.Same(t, tp1.d, byAddr)
	byName, err := dm.GetDomainByName(ctx, "test1")
	require.NoError(t, err)
	assert.Same(t, tp1.d, byName)

}

func TestDomainInitBadSchemas(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
		AbiStateSchemasJson: []string{
			`!!! Wrong`,
		},
	})
	defer done()
	assert.Regexp(t, "PD011602", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitBadEventsJSON(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
		AbiStateSchemasJson:    []string{},
		AbiEventsJson:          `!!! Wrong`,
	})
	defer done()
	assert.Regexp(t, "PD011642", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitBadEventsABI(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
		AbiStateSchemasJson:    []string{},
		AbiEventsJson: `[
			{
				"type": "badbadwrong",
				"name": "bad",
				"inputs": [{"type": "verywrong"}]
			}
		]`,
	})
	defer done()
	assert.Regexp(t, "FF22025", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitStreamFail(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
		AbiStateSchemasJson:    []string{},
		AbiEventsJson:          fakeCoinEventsABI,
	}, func(mc *mockComponents) {
		mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.EqualError(t, *tp.d.initError.Load(), "pop")
	assert.False(t, tp.initialized.Load())
}

func TestDomainInitFactorySchemaStoreFail(t *testing.T) {
	_, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{
		BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{},
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	}, func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, "test1", mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Regexp(t, "pop", *tp.d.initError.Load())
	assert.False(t, tp.initialized.Load())
}

func TestDomainConfigureFail(t *testing.T) {

	ctx, dm, _, done := newTestDomainManager(t, false, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{
			"test1": {
				Config:          map[string]any{"some": "config"},
				RegistryAddress: tktypes.RandHex(20),
			},
		},
	})
	defer done()

	tp := newTestPlugin(&plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	})

	_, err := dm.DomainRegistered("test1", tp)
	require.NoError(t, err)

	da, err := dm.GetDomainByName(ctx, "test1")
	require.NoError(t, err)

	d := da.(*domain)
	d.initRetry.UTSetMaxAttempts(1)
	<-d.initDone
	assert.Regexp(t, "pop", *d.initError.Load())
}

func TestDomainFindAvailableStatesNotInit(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, &prototk.DomainConfig{})
	defer done()
	assert.NotNil(t, *tp.d.initError.Load())
	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{SchemaId: "12345"})
	assert.Regexp(t, "PD011601", err)
}

func TestDomainFindAvailableStatesBadQuery(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())
	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId:  "12345",
		QueryJson: `!!!{ wrong`,
	})
	assert.Regexp(t, "PD011608", err)
}

func TestDomainFindAvailableStatesBadAddress(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())
	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		SchemaId:        "12345",
		ContractAddress: "0x",
		QueryJson:       `{}`,
	})
	assert.Regexp(t, "PD011641", err)
}

func TestDomainFindAvailableStatesFail(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, "test1", mock.Anything).Return([]statestore.Schema{}, nil)
		mc.domainStateInterface.On("FindAvailableStates", "12345", mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Nil(t, tp.d.initError.Load())
	_, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		ContractAddress: tktypes.RandAddress().String(),
		SchemaId:        "12345",
		QueryJson:       `{}`,
	})
	assert.Regexp(t, "pop", err)
}

func storeState(t *testing.T, dm *domainManager, tp *testPlugin, contractAddress tktypes.EthAddress, txID uuid.UUID, amount *ethtypes.HexInteger) *fakeState {
	state := &fakeState{
		Salt:   tktypes.Bytes32(tktypes.RandBytes(32)),
		Owner:  tktypes.EthAddress(tktypes.RandBytes(20)),
		Amount: amount,
	}
	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)

	err = dm.stateStore.RunInDomainContextFlush("test1", contractAddress, func(ctx context.Context, dsi statestore.DomainStateInterface) error {
		newStates, err := dsi.UpsertStates(&txID, []*statestore.StateUpsert{
			{
				SchemaID: tp.stateSchemas[0].Id,
				Data:     stateJSON,
				Creating: true,
			},
		})
		assert.Len(t, newStates, 1)
		return err
	})
	require.NoError(t, err)
	return state
}

func TestDomainFindAvailableStatesOK(t *testing.T) {
	ctx, dm, tp, done := newTestDomain(t, true /* use real state store for this one */, goodDomainConf())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	txID := uuid.New()
	contractAddress := tktypes.RandAddress()
	state1 := storeState(t, dm, tp, *contractAddress, txID, ethtypes.NewHexIntegerU64(100000000))

	// Filter match
	states, err := tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		ContractAddress: contractAddress.String(),
		SchemaId:        tp.stateSchemas[0].Id,
		QueryJson: `{
		  "eq": [
		    { "field": "owner", "value": "` + state1.Owner.String() + `" }
		  ]
		}`,
	})
	require.NoError(t, err)
	assert.Len(t, states.States, 1)

	// Filter miss
	states, err = tp.d.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		ContractAddress: contractAddress.String(),
		SchemaId:        tp.stateSchemas[0].Id,
		QueryJson: `{
		  "eq": [
		    { "field": "owner", "value": "` + tktypes.EthAddress(tktypes.RandBytes(20)).String() + `" }
		  ]
		}`,
	})
	require.NoError(t, err)
	assert.Len(t, states.States, 0)
}

func TestDomainInitDeployOK(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	txID := uuid.MustParse("53BA5DD6-B708-444D-A2FA-50578B974FB7")
	tp.Functions.InitDeploy = func(ctx context.Context, idr *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
		defer done()
		assert.Equal(t, "0x53ba5dd6b708444da2fa50578b974fb700000000000000000000000000000000", idr.Transaction.TransactionId)
		assert.JSONEq(t, `{
		  "notary": "notary1",
		  "name": "token1",
		  "symbol": "TKN1"
		}`, idr.Transaction.ConstructorParamsJson)
		return &prototk.InitDeployResponse{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{
					Lookup:    "signer1",
					Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
				},
			},
		}, nil
	}

	domain := tp.d
	tx := &components.PrivateContractDeploy{
		ID: txID,
		Inputs: tktypes.RawJSON(`{
		  "notary": "notary1",
		  "name": "token1",
		  "symbol": "TKN1"
		}`),
	}
	err := domain.InitDeploy(ctx, tx)
	require.NoError(t, err)
	assert.Len(t, tx.RequiredVerifiers, 1)

}

func TestDomainInitDeployMissingInput(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	txID := uuid.MustParse("53BA5DD6-B708-444D-A2FA-50578B974FB7")
	domain := tp.d
	tx := &components.PrivateContractDeploy{
		ID: txID,
	}
	err := domain.InitDeploy(ctx, tx)
	assert.Regexp(t, "PD011620", err)
	assert.Nil(t, tx.RequiredVerifiers)
}

func TestDomainInitDeployError(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	txID := uuid.MustParse("53BA5DD6-B708-444D-A2FA-50578B974FB7")
	tp.Functions.InitDeploy = func(ctx context.Context, idr *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	domain := tp.d
	tx := &components.PrivateContractDeploy{
		ID: txID,
		Inputs: tktypes.RawJSON(`{
		  "notary": "notary1",
		  "name": "token1",
		  "symbol": "TKN1"
		}`),
	}
	err := domain.InitDeploy(ctx, tx)
	assert.Regexp(t, "pop", err)
	assert.Nil(t, tx.RequiredVerifiers)
}

func goodTXForDeploy() *components.PrivateContractDeploy {
	txID := uuid.New()
	return &components.PrivateContractDeploy{
		ID:                       uuid.New(),
		Inputs:                   tktypes.RawJSON(`{}`),
		TransactionSpecification: &prototk.DeployTransactionSpecification{TransactionId: tktypes.Bytes32UUIDFirst16(txID).String()},
		Verifiers: []*prototk.ResolvedVerifier{
			{
				Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
				Lookup:    "notary",
				Verifier:  tktypes.EthAddress(tktypes.RandBytes(20)).String(),
			},
		},
	}
}

func TestDomainPrepareDeployInvokeTX(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		assert.Same(t, tx.TransactionSpecification, pdr.Transaction)
		return &prototk.PrepareDeployResponse{
			Transaction: &prototk.BaseLedgerTransaction{
				FunctionAbiJson: fakeCoinFactoryNewInstanceABI,
				ParamsJson: `{
				  "notary": "` + pdr.ResolvedVerifiers[0].Verifier + `",
				  "data": "0xfeedbeef"
				}`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	require.NoError(t, err)
	assert.Nil(t, tx.DeployTransaction)
	assert.Equal(t, "newInstance", tx.InvokeTransaction.FunctionABI.Name)
	assert.Equal(t, abi.Function, tx.InvokeTransaction.FunctionABI.Type)
	assert.NotNil(t, tx.InvokeTransaction.Inputs)
	assert.Equal(t, "one/time/keys/"+tx.ID.String(), tx.Signer)
}

func TestDomainPrepareDeployDeployTXWithSigner(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		assert.Same(t, tx.TransactionSpecification, pdr.Transaction)
		return &prototk.PrepareDeployResponse{
			Deploy: &prototk.BaseLedgerDeployTransaction{
				ConstructorAbiJson: fakeCoinFactoryConstructorABI,
				ParamsJson: `{
				  "notary": "` + pdr.ResolvedVerifiers[0].Verifier + `",
				  "data": "0xfeedbeef"
				}`,
			},
			Signer: confutil.P("signer1"),
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	require.NoError(t, err)
	assert.Nil(t, tx.InvokeTransaction)
	assert.Equal(t, abi.Constructor, tx.DeployTransaction.ConstructorABI.Type)
	assert.NotNil(t, tx.DeployTransaction.Inputs)
	assert.Equal(t, "signer1", tx.Signer)
}

func TestDomainPrepareDeployMissingInput(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := &components.PrivateContractDeploy{
		ID: uuid.New(),
	}
	err := domain.PrepareDeploy(ctx, tx)
	assert.Regexp(t, "PD011621", err)
	assert.Nil(t, tx.RequiredVerifiers)
}

func TestDomainPrepareDeployError(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()
	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := domain.PrepareDeploy(ctx, tx)
	assert.Regexp(t, "pop", err)
}

func TestDomainPrepareDeployMissingSigner(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()
	tp.d.config.BaseLedgerSubmitConfig.SubmitMode = prototk.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Deploy: &prototk.BaseLedgerDeployTransaction{},
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	assert.Regexp(t, "PD011622", err)
}

func TestDomainPrepareDeployBadParams(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Deploy: &prototk.BaseLedgerDeployTransaction{
				ConstructorAbiJson: fakeCoinFactoryConstructorABI,
				ParamsJson:         `{"missing":"expected things"}`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	assert.Regexp(t, "FF22040", err)
}

func TestDomainPrepareDeployDefaultConstructor(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Deploy: &prototk.BaseLedgerDeployTransaction{},
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	require.NoError(t, err)
}

func TestDomainPrepareInvokeBadParams(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Transaction: &prototk.BaseLedgerTransaction{
				FunctionAbiJson: fakeCoinFactoryNewInstanceABI,
				ParamsJson:      `{"missing":"expected things"}`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	assert.Regexp(t, "FF22040", err)
}

func TestDomainPrepareDeployABIInvalid(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Deploy: &prototk.BaseLedgerDeployTransaction{
				ConstructorAbiJson: `!!!wrong`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	assert.Regexp(t, "PD011605", err)
}

func TestDomainPrepareInvokeABIInvalid(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Transaction: &prototk.BaseLedgerTransaction{
				FunctionAbiJson: `!!!wrong`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	assert.Regexp(t, "PD011605", err)
}

func TestDomainPrepareInvokeAndDeploy(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, tp.d.initError.Load())

	domain := tp.d
	tx := goodTXForDeploy()

	tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Transaction: &prototk.BaseLedgerTransaction{
				FunctionAbiJson: fakeCoinFactoryNewInstanceABI,
				ParamsJson:      `{}`,
			},
			Deploy: &prototk.BaseLedgerDeployTransaction{
				ParamsJson: `{}`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(ctx, tx)
	assert.Regexp(t, "PD011611", err)
}

func TestEncodeABIDataFailCases(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	_, err := d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_EncodingType(42),
	})
	assert.Regexp(t, "PD011635", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_FUNCTION_CALL_DATA,
		Body:         `{!!!`,
	})
	assert.Regexp(t, "PD011633", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_TUPLE,
		Body:         `{!!!`,
	})
	assert.Regexp(t, "PD011633", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_FUNCTION_CALL_DATA,
		Definition:   `{"inputs":[{"name":"int1","type":"uint256"}]}`,
		Body:         `{}`,
	})
	assert.Regexp(t, "PD011634.*int1", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_TUPLE,
		Definition:   `{"components":[{"name":"int1","type":"uint256"}]}`,
		Body:         `{}`,
	})
	assert.Regexp(t, "PD011634.*int1", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_ETH_TRANSACTION,
		Definition:   `wrong`,
		Body:         `{"to":"0x92CB9e0086a774525469bbEde564729F277d2549"}`,
	})
	assert.Regexp(t, "PD011635", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_ETH_TRANSACTION,
		Body:         `{!!!bad`,
	})
	assert.Regexp(t, "PD011633", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_TYPED_DATA_V4,
		Body:         `{}`,
	})
	assert.Regexp(t, "PD011640", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodeDataRequest_TYPED_DATA_V4,
		Body:         `{!!!bad`,
	})
	assert.Regexp(t, "PD011639", err)
}

func TestRecoverSignerFailCases(t *testing.T) {
	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	_, err := d.RecoverSigner(ctx, &prototk.RecoverSignerRequest{
		Algorithm: "not supported",
	})
	assert.Regexp(t, "PD011637", err)
	_, err = d.RecoverSigner(ctx, &prototk.RecoverSignerRequest{
		Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
		Signature: ([]byte)("not a signature RSV"),
	})
	assert.Regexp(t, "PD011638", err)
}

func TestHandleEventBatch(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := tktypes.Bytes32UUIDFirst16(txID)
	contract1 := tktypes.RandAddress()
	contract2 := tktypes.RandAddress()
	stateSpent := tktypes.RandHex(32)
	stateConfirmed := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.domainStateInterface.On("MarkStatesSpent", txID, []string{stateSpent}).Return(nil)
		mc.domainStateInterface.On("MarkStatesConfirmed", txID, []string{stateConfirmed}).Return(nil)
		mc.domainStateInterface.On("UpsertStates", &txID, mock.Anything).Return(nil, nil)
	})
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	// First contract is unrecognized, second is recognized
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{},
	))
	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract2, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		assert.Equal(t, batchID.String(), req.BatchId)
		var events []*blockindexer.EventWithData
		err := json.Unmarshal([]byte(req.JsonEvents), &events)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, contract2.String(), events[0].Address.String())
		return &prototk.HandleEventBatchResponse{
			TransactionsComplete: []string{
				txIDBytes32.String(),
			},
			SpentStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: txIDBytes32.String(),
				},
			},
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateConfirmed,
					TransactionId: txIDBytes32.String(),
				},
			},
			NewStates: []*prototk.NewLocalState{
				{
					StateDataJson: `{"color": "blue"}`,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}

	cb, err := d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
			{
				Address: *contract2,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.NoError(t, err)

	req := d.dm.transactionWaiter.AddInflight(ctx, txID)
	cb()
	_, err = req.Wait()
	assert.NoError(t, err)
}

func TestHandleEventBatchContractLookupFail(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchDomainError(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchSpentBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()
	stateSpent := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			SpentStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchConfirmBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()
	stateSpent := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchNewBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewLocalState{
				{
					TransactionId: "badnotgood",
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchBadTransactionID(t *testing.T) {
	batchID := uuid.New()
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			TransactionsComplete: []string{
				"badnotgood",
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.ErrorContains(t, err, "PD020007")
}

func TestHandleEventBatchMarkSpentFail(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := tktypes.Bytes32UUIDFirst16(txID)
	contract1 := tktypes.RandAddress()
	stateSpent := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.domainStateInterface.On("MarkStatesSpent", txID, []string{stateSpent}).Return(fmt.Errorf("pop"))
	})
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			SpentStates: []*prototk.StateUpdate{
				{
					Id:            stateSpent,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchMarkConfirmedFail(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := tktypes.Bytes32UUIDFirst16(txID)
	contract1 := tktypes.RandAddress()
	stateConfirmed := tktypes.RandHex(32)

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.domainStateInterface.On("MarkStatesConfirmed", txID, []string{stateConfirmed}).Return(fmt.Errorf("pop"))
	})
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			ConfirmedStates: []*prototk.StateUpdate{
				{
					Id:            stateConfirmed,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestHandleEventBatchUpsertFail(t *testing.T) {
	batchID := uuid.New()
	txID := uuid.New()
	txIDBytes32 := tktypes.Bytes32UUIDFirst16(txID)
	contract1 := tktypes.RandAddress()

	ctx, _, tp, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.domainStateInterface.On("UpsertStates", &txID, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	d := tp.d

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)

	mp.Mock.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows(
		[]string{"address", "domain_address"},
	).AddRow(contract1, d.registryAddress))

	tp.Functions.HandleEventBatch = func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
		return &prototk.HandleEventBatchResponse{
			NewStates: []*prototk.NewLocalState{
				{
					StateDataJson: `{"color": "blue"}`,
					TransactionId: txIDBytes32.String(),
				},
			},
		}, nil
	}

	_, err = d.handleEventBatch(ctx, mp.P.DB(), &blockindexer.EventDeliveryBatch{
		BatchID: batchID,
		Events: []*blockindexer.EventWithData{
			{
				Address: *contract1,
				Data:    tktypes.RawJSON(`{"result": "success"}`),
			},
		},
	})
	assert.EqualError(t, err, "pop")
}
