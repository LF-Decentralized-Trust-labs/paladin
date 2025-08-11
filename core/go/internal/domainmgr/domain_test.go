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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
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

const fakeDownstreamPrivateABI = `{
	"type": "function",
    "name": "doTheNextThing",
	"inputs": [
		{
			"name": "thing",
			"type": "string"
		}
	]
}`

type fakeState struct {
	Salt   pldtypes.Bytes32     `json:"salt"`
	Owner  pldtypes.EthAddress  `json:"owner"`
	Amount *ethtypes.HexInteger `json:"amount"`
}

type fakeExecute struct {
	Inputs  []pldtypes.HexBytes `json:"inputs"`
	Outputs []pldtypes.HexBytes `json:"outputs"`
	Data    pldtypes.HexBytes   `json:"data"`
}

type testPlugin struct {
	plugintk.DomainAPIBase
	initialized  atomic.Bool
	d            *domain
	stateSchemas []*prototk.StateSchema
}

type testDomainContext struct {
	ctx             context.Context
	mdc             *componentsmocks.DomainContext
	dm              *domainManager
	d               *domain
	tp              *testPlugin
	c               *inFlightDomainRequest
	contractAddress pldtypes.EthAddress
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

func newTestDomain(t *testing.T, realDB bool, domainConfig *prototk.DomainConfig, extraSetup ...func(mc *mockComponents)) (*testDomainContext, func()) {

	ctx, dm, mc, dmDone := newTestDomainManager(t, realDB, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				Config:          map[string]any{"some": "conf"},
				RegistryAddress: pldtypes.RandHex(20),
				DefaultGasLimit: confutil.P(uint64(100000)),
				Init:            pldconf.DomainInitConfig{},
			},
		},
	}, extraSetup...)

	mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()

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

	var c *inFlightDomainRequest
	var mdc *componentsmocks.DomainContext
	addr := *pldtypes.RandAddress()
	if realDB {
		dCtx := dm.stateStore.NewDomainContext(ctx, tp.d, addr)
		c = tp.d.newInFlightDomainRequest(dm.persistence.NOTX(), dCtx, true /* readonly unless modified by test */)
	} else {
		mdc = componentsmocks.NewDomainContext(t)
		mdc.On("Ctx").Return(ctx).Maybe()
		mdc.On("Info").Return(components.DomainContextInfo{ID: uuid.New()}).Maybe()
		mdc.On("Close").Return()
		c = tp.d.newInFlightDomainRequest(dm.persistence.NOTX(), mdc, true /* readonly unless modified by test */)
		mc.stateStore.On("NewDomainContext", mock.Anything, tp.d, mock.Anything, mock.Anything).Return(mdc).Maybe()
	}

	return &testDomainContext{
			ctx:             ctx,
			dm:              dm,
			d:               tp.d,
			tp:              tp,
			c:               c,
			mdc:             mdc,
			contractAddress: addr,
		}, func() {
			c.close()
			c.dCtx.Close()
			if mdc != nil {
				mdc.Close()
			}
			dmDone()
		}
}

func registerTestDomain(t *testing.T, dm *domainManager, tp *testPlugin) {
	d, err := dm.registerDomain("test1", tp)
	require.NoError(t, err)

	// For unit tests, we want any errors to pop out - rather than the actual runtime behavior of infinite retry
	d.initRetry.UTSetMaxAttempts(1)

	// Kick off the init (as would happen in DomainRegistered callback otherwise)
	go d.init()

	da, err := dm.getDomainByName(context.Background(), "test1")
	require.NoError(t, err)
	tp.d = da
	<-tp.d.initDone
}

func goodDomainConf() *prototk.DomainConfig {
	return &prototk.DomainConfig{
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	}
}

func mockSchemas(schemas ...components.Schema) func(mc *mockComponents) {
	return func(mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(schemas, nil)
		mc.db.ExpectCommit()
	}
}

func TestDomainInitStates(t *testing.T) {

	domainConf := goodDomainConf()
	td, done := newTestDomain(t, true, domainConf)
	defer done()

	assert.Nil(t, td.d.initError.Load())
	assert.True(t, td.tp.initialized.Load())
	byAddr, err := td.dm.getDomainByAddress(td.ctx, td.d.RegistryAddress())
	require.NoError(t, err)
	assert.Equal(t, td.d, byAddr)
	assert.True(t, td.d.Initialized())

}

func mockUpsertABIOk(mc *mockComponents) {
	mc.txManager.On("UpsertABI", mock.Anything, mock.Anything, mock.Anything).Return(&pldapi.StoredABI{
		Hash: pldtypes.RandBytes32(),
	}, nil)
}

func TestDomainInitStatesWithEvents(t *testing.T) {

	domainConf := goodDomainConf()
	domainConf.AbiEventsJson = fakeCoinEventsABI
	td, done := newTestDomain(t, true, domainConf, mockUpsertABIOk)
	defer done()

	assert.Nil(t, td.d.initError.Load())
	assert.True(t, td.tp.initialized.Load())
	byAddr, err := td.dm.getDomainByAddress(td.ctx, td.d.RegistryAddress())
	require.NoError(t, err)
	assert.Equal(t, td.d, byAddr)
	assert.True(t, td.d.Initialized())

}

func TestDoubleRegisterReplaces(t *testing.T) {

	domainConf := goodDomainConf()
	td, done := newTestDomain(t, true, domainConf)
	defer done()
	assert.Nil(t, td.tp.d.initError.Load())
	assert.True(t, td.tp.initialized.Load())

	// Register again
	tp1 := newTestPlugin(nil)
	tp1.Functions = td.tp.Functions
	registerTestDomain(t, td.dm, tp1)
	assert.Nil(t, tp1.d.initError.Load())
	assert.True(t, tp1.initialized.Load())

	// Check we get the second from all the maps
	byAddr, err := td.dm.getDomainByAddress(td.ctx, td.tp.d.RegistryAddress())
	require.NoError(t, err)
	assert.Same(t, tp1.d, byAddr)
	byName, err := td.dm.GetDomainByName(td.ctx, "test1")
	require.NoError(t, err)
	assert.Same(t, tp1.d, byName)

}

func mockBegin(mc *mockComponents) {
	mc.db.ExpectBegin()
}

func TestDomainInitBadSchemas(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{
			`!!! Wrong`,
		},
	}, mockBegin)
	defer done()
	assert.Regexp(t, "PD011602", *td.d.initError.Load())
	assert.False(t, td.tp.initialized.Load())
}

func TestDomainInitBadEventsJSON(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{},
		AbiEventsJson:       `!!! Wrong`,
	}, mockBegin)
	defer done()
	assert.Regexp(t, "PD011642", *td.d.initError.Load())
	assert.False(t, td.tp.initialized.Load())
}

func TestDomainInitBadEventsABI(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{},
		AbiEventsJson: `[
			{
				"type": "event",
				"name": "bad",
				"inputs": [{"type": "verywrong"}]
			}
		]`,
	}, mockBegin, mockUpsertABIOk)
	defer done()
	assert.Regexp(t, "FF22025", *td.d.initError.Load())
	assert.False(t, td.tp.initialized.Load())
}

func TestDomainInitUpsertEventsABIFail(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{},
		AbiEventsJson: `[
			{
				"type": "event",
				"name": "bad",
				"inputs": [{"type": "verywrong"}]
			}
		]`,
	}, mockBegin, func(mc *mockComponents) {
		mc.txManager.On("UpsertABI", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Regexp(t, "pop", *td.d.initError.Load())
	assert.False(t, td.tp.initialized.Load())
}

func TestDomainInitStreamFail(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{},
		AbiEventsJson:       fakeCoinEventsABI,
	}, mockBegin, mockUpsertABIOk, func(mc *mockComponents) {
		mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.EqualError(t, *td.d.initError.Load(), "pop")
	assert.False(t, td.tp.initialized.Load())
}

func TestDomainInitFactorySchemaStoreFail(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	}, mockBegin, func(mc *mockComponents) {
		mc.stateStore.On("EnsureABISchemas", mock.Anything, mock.Anything, "test1", mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Regexp(t, "pop", *td.d.initError.Load())
	assert.False(t, td.tp.initialized.Load())
}

func TestDomainConfigureFail(t *testing.T) {

	ctx, dm, _, done := newTestDomainManager(t, false, &pldconf.DomainManagerConfig{
		Domains: map[string]*pldconf.DomainConfig{
			"test1": {
				Config:          map[string]any{"some": "config"},
				RegistryAddress: pldtypes.RandHex(20),
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

	d, err := dm.getDomainByName(ctx, "test1")
	require.NoError(t, err)

	d.initRetry.UTSetMaxAttempts(1)
	<-d.initDone
	assert.Regexp(t, "pop", *d.initError.Load())
}

func TestDomainFindAvailableStatesNotInit(t *testing.T) {
	td, done := newTestDomain(t, false, &prototk.DomainConfig{
		AbiStateSchemasJson: []string{`{!!! invalid`},
	})
	defer done()
	assert.NotNil(t, *td.d.initError.Load())
	_, err := td.d.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          "12345",
	})
	assert.Regexp(t, "PD011601", err)
}

func TestDomainFindAvailableStatesBadSchema(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())
	_, err := td.d.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          "12345",
		QueryJson:         `{}`,
	})
	assert.Regexp(t, "PD011641", err)
}

func TestDomainFindAvailableStatesBadQuery(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())
	_, err := td.d.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          "12345",
		QueryJson:         `!!!{ wrong`,
	})
	assert.Regexp(t, "PD011608", err)
}

func TestDomainFindAvailableStatesBadQStateQueryContext(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())
	_, err := td.d.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: "wrong",
		SchemaId:          "12345",
		QueryJson:         `{}`,
	})
	assert.Regexp(t, "PD011649", err)
}

func TestDomainFindAvailableStatesFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	schemaID := pldtypes.RandBytes32()
	td.mdc.On("FindAvailableStates", mock.Anything, schemaID, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	assert.Nil(t, td.d.initError.Load())
	_, err := td.d.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          schemaID.String(),
		QueryJson:         `{}`,
	})
	assert.Regexp(t, "pop", err)
}

func storeTestState(t *testing.T, td *testDomainContext, txID uuid.UUID, amount *ethtypes.HexInteger) *fakeState {
	state := &fakeState{
		Salt:   pldtypes.RandBytes32(),
		Owner:  pldtypes.EthAddress(pldtypes.RandBytes(20)),
		Amount: amount,
	}
	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)

	// Call the real statestore
	_, err = td.c.dCtx.UpsertStates(td.c.dbTX, &components.StateUpsert{
		Schema:    pldtypes.MustParseBytes32(td.tp.stateSchemas[0].Id),
		Data:      stateJSON,
		CreatedBy: &txID,
	})
	require.NoError(t, err)
	return state
}

func TestDomainFindAvailableStatesOK(t *testing.T) {
	td, done := newTestDomain(t, true /* use real state store for this one */, goodDomainConf())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	txID := uuid.New()
	state1 := storeTestState(t, td, txID, ethtypes.NewHexIntegerU64(100000000))

	// Filter match
	states, err := td.d.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          td.tp.stateSchemas[0].Id,
		QueryJson: `{
		  "eq": [
		    { "field": "owner", "value": "` + state1.Owner.String() + `" }
		  ]
		}`,
	})
	require.NoError(t, err)
	assert.Len(t, states.States, 1)

	// Filter miss
	states, err = td.d.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          td.tp.stateSchemas[0].Id,
		QueryJson: `{
		  "eq": [
		    { "field": "owner", "value": "` + pldtypes.EthAddress(pldtypes.RandBytes(20)).String() + `" }
		  ]
		}`,
	})
	require.NoError(t, err)
	assert.Len(t, states.States, 0)

	// Nullifier miss
	useNullifiers := true
	states, err = td.d.FindAvailableStates(td.ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: td.c.id,
		SchemaId:          td.tp.stateSchemas[0].Id,
		QueryJson: `{
		  "eq": [
		    { "field": "owner", "value": "` + state1.Owner.String() + `" }
		  ]
		}`,
		UseNullifiers: &useNullifiers,
	})
	require.NoError(t, err)
	assert.Len(t, states.States, 0)
}

func TestDomainInitDeployOK(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	txID := uuid.MustParse("53BA5DD6-B708-444D-A2FA-50578B974FB7")
	td.tp.Functions.InitDeploy = func(ctx context.Context, idr *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
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
					Lookup:       "signer1",
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
		}, nil
	}

	domain := td.d
	tx := &components.PrivateContractDeploy{
		ID: txID,
		Inputs: pldtypes.RawJSON(`{
		  "notary": "notary1",
		  "name": "token1",
		  "symbol": "TKN1"
		}`),
	}
	err := domain.InitDeploy(td.ctx, tx)
	require.NoError(t, err)
	assert.Len(t, tx.RequiredVerifiers, 1)

}

func TestDomainInitDeployMissingInput(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	txID := uuid.MustParse("53BA5DD6-B708-444D-A2FA-50578B974FB7")
	domain := td.d
	tx := &components.PrivateContractDeploy{
		ID: txID,
	}
	err := domain.InitDeploy(td.ctx, tx)
	assert.Regexp(t, "PD011620", err)
	assert.Nil(t, tx.RequiredVerifiers)
}

func TestDomainInitDeployError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	txID := uuid.MustParse("53BA5DD6-B708-444D-A2FA-50578B974FB7")
	td.tp.Functions.InitDeploy = func(ctx context.Context, idr *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	domain := td.d
	tx := &components.PrivateContractDeploy{
		ID: txID,
		Inputs: pldtypes.RawJSON(`{
		  "notary": "notary1",
		  "name": "token1",
		  "symbol": "TKN1"
		}`),
	}
	err := domain.InitDeploy(td.ctx, tx)
	assert.Regexp(t, "pop", err)
	assert.Nil(t, tx.RequiredVerifiers)
}

func goodTXForDeploy() *components.PrivateContractDeploy {
	txID := uuid.New()
	return &components.PrivateContractDeploy{
		ID:                       uuid.New(),
		Inputs:                   pldtypes.RawJSON(`{}`),
		TransactionSpecification: &prototk.DeployTransactionSpecification{TransactionId: pldtypes.Bytes32UUIDFirst16(txID).String()},
		Verifiers: []*prototk.ResolvedVerifier{
			{
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
				Lookup:       "notary",
				Verifier:     pldtypes.EthAddress(pldtypes.RandBytes(20)).String(),
			},
		},
	}
}

func TestDomainPrepareDeployInvokeTX(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()

	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		assert.Same(t, tx.TransactionSpecification, pdr.Transaction)
		return &prototk.PrepareDeployResponse{
			Transaction: &prototk.PreparedTransaction{
				FunctionAbiJson: fakeCoinFactoryNewInstanceABI,
				ParamsJson: `{
				  "notary": "` + pdr.ResolvedVerifiers[0].Verifier + `",
				  "data": "0xfeedbeef"
				}`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(td.ctx, tx)
	require.NoError(t, err)
	assert.Nil(t, tx.DeployTransaction)
	assert.Equal(t, "newInstance", tx.InvokeTransaction.FunctionABI.Name)
	assert.Equal(t, abi.Function, tx.InvokeTransaction.FunctionABI.Type)
	assert.NotNil(t, tx.InvokeTransaction.Inputs)
	assert.Empty(t, tx.Signer) // to be assigned by private TX manager
}

func TestDomainPrepareDeployDeployTXWithSigner(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()

	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
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

	err := domain.PrepareDeploy(td.ctx, tx)
	require.NoError(t, err)
	assert.Nil(t, tx.InvokeTransaction)
	assert.Equal(t, abi.Constructor, tx.DeployTransaction.ConstructorABI.Type)
	assert.NotNil(t, tx.DeployTransaction.Inputs)
	assert.Equal(t, "signer1", tx.Signer)
}

func TestDomainPrepareDeployMissingInput(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := &components.PrivateContractDeploy{
		ID: uuid.New(),
	}
	err := domain.PrepareDeploy(td.ctx, tx)
	assert.Regexp(t, "PD011621", err)
	assert.Nil(t, tx.RequiredVerifiers)
}

func TestDomainPrepareDeployError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()
	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	err := domain.PrepareDeploy(td.ctx, tx)
	assert.Regexp(t, "pop", err)
}

func TestDomainPrepareDeployBadParams(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()

	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Deploy: &prototk.BaseLedgerDeployTransaction{
				ConstructorAbiJson: fakeCoinFactoryConstructorABI,
				ParamsJson:         `{"missing":"expected things"}`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(td.ctx, tx)
	assert.Regexp(t, "FF22040", err)
}

func TestDomainPrepareDeployDefaultConstructor(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()

	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Deploy: &prototk.BaseLedgerDeployTransaction{},
		}, nil
	}

	err := domain.PrepareDeploy(td.ctx, tx)
	require.NoError(t, err)
}

func TestDomainPrepareInvokeBadParams(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()

	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Transaction: &prototk.PreparedTransaction{
				FunctionAbiJson: fakeCoinFactoryNewInstanceABI,
				ParamsJson:      `{"missing":"expected things"}`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(td.ctx, tx)
	assert.Regexp(t, "FF22040", err)
}

func TestDomainPrepareDeployABIInvalid(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()

	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Deploy: &prototk.BaseLedgerDeployTransaction{
				ConstructorAbiJson: `!!!wrong`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(td.ctx, tx)
	assert.Regexp(t, "PD011605", err)
}

func TestDomainPrepareInvokeABIInvalid(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()

	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Transaction: &prototk.PreparedTransaction{
				FunctionAbiJson: `!!!wrong`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(td.ctx, tx)
	assert.Regexp(t, "PD011605", err)
}

func TestDomainPrepareInvokeAndDeploy(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	domain := td.d
	tx := goodTXForDeploy()

	td.tp.Functions.PrepareDeploy = func(ctx context.Context, pdr *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
		return &prototk.PrepareDeployResponse{
			Transaction: &prototk.PreparedTransaction{
				FunctionAbiJson: fakeCoinFactoryNewInstanceABI,
				ParamsJson:      `{}`,
			},
			Deploy: &prototk.BaseLedgerDeployTransaction{
				ParamsJson: `{}`,
			},
		}, nil
	}

	err := domain.PrepareDeploy(td.ctx, tx)
	assert.Regexp(t, "PD011611", err)
}

func TestEncodeABIDataFailCases(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := td.d
	ctx := td.ctx

	_, err := d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType(42),
	})
	assert.Regexp(t, "PD011635", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_FUNCTION_CALL_DATA,
		Body:         `{!!!`,
	})
	assert.Regexp(t, "PD011633", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_TUPLE,
		Body:         `{!!!`,
	})
	assert.Regexp(t, "PD011633", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_FUNCTION_CALL_DATA,
		Definition:   `{"inputs":[{"name":"int1","type":"uint256"}]}`,
		Body:         `{}`,
	})
	assert.Regexp(t, "PD011634.*int1", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_TUPLE,
		Definition:   `{"components":[{"name":"int1","type":"uint256"}]}`,
		Body:         `{}`,
	})
	assert.Regexp(t, "PD011634.*int1", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   `wrong`,
		Body:         `{"to":"0x92CB9e0086a774525469bbEde564729F277d2549"}`,
	})
	assert.Regexp(t, "PD011635", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Body:         `{!!!bad`,
	})
	assert.Regexp(t, "PD011633", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_TYPED_DATA_V4,
		Body:         `{}`,
	})
	assert.Regexp(t, "PD011640", err)
	_, err = d.EncodeData(ctx, &prototk.EncodeDataRequest{
		EncodingType: prototk.EncodingType_TYPED_DATA_V4,
		Body:         `{!!!bad`,
	})
	assert.Regexp(t, "PD011639", err)
}

func TestDecodeABIDataFailCases(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := td.d
	ctx := td.ctx

	_, err := d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType(42),
	})
	assert.Regexp(t, "PD011647", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_FUNCTION_CALL_DATA,
		Data:         []byte(`{!!!`),
	})
	assert.Regexp(t, "PD011645", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_TUPLE,
		Data:         []byte(`{!!!`),
	})
	assert.Regexp(t, "PD011645", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_EVENT_DATA,
		Data:         []byte(`{!!!`),
	})
	assert.Regexp(t, "PD011645", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_FUNCTION_CALL_DATA,
		Definition:   `{"inputs":[{"name":"int1","type":"uint256"}]}`,
		Data:         []byte(``),
	})
	assert.Regexp(t, "PD011646.*Insufficient bytes", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_TUPLE,
		Definition:   `{"components":[{"name":"int1","type":"uint256"}]}`,
		Data:         []byte(``),
	})
	assert.Regexp(t, "PD011646.*Insufficient bytes", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_EVENT_DATA,
		Definition:   `{"inputs":[{"name":"int1","type":"uint256"}]}`,
		Data:         []byte(``),
	})
	assert.Regexp(t, "PD011646.*Insufficient bytes", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "eip1559",
		Data:         []byte(``),
	})
	assert.Regexp(t, "PD011646.*FF22084", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION,
		Definition:   "eip155", // not supported for UNSIGNED round trip currently (supported for signed)
		Data:         []byte(``),
	})
	assert.Regexp(t, "PD011645", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION_SIGNED,
		Definition:   "wrong",
		Data:         []byte(``),
	})
	assert.Regexp(t, "PD011645", err)
	_, err = d.DecodeData(ctx, &prototk.DecodeDataRequest{
		EncodingType: prototk.EncodingType_ETH_TRANSACTION_SIGNED,
		Definition:   "eip1559",
		Data:         []byte{}, // Empty data to trigger decode error
	})
	assert.Regexp(t, "PD011646", err)
}

func TestRecoverSignerFailCases(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	d := td.d
	ctx := td.ctx

	_, err := d.RecoverSigner(ctx, &prototk.RecoverSignerRequest{
		Algorithm: "not supported",
	})
	assert.Regexp(t, "PD011637", err)
	_, err = d.RecoverSigner(ctx, &prototk.RecoverSignerRequest{
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Signature:   ([]byte)("not a signature RSV"),
	})
	assert.Regexp(t, "PD011638", err)
}

func TestSendTransactionFailCases(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	_, err := td.d.SendTransaction(td.ctx, &prototk.SendTransactionRequest{
		StateQueryContext: td.c.id,
	})
	require.ErrorContains(t, err, "PD011663")

	td.c.readOnly = false

	_, err = td.d.SendTransaction(td.ctx, &prototk.SendTransactionRequest{
		StateQueryContext: td.c.id,
		Transaction: &prototk.TransactionInput{
			ContractAddress: "badnotgood",
			FunctionAbiJson: `{}`,
			ParamsJson:      `{}`,
		},
	})
	require.ErrorContains(t, err, "bad address")

	_, err = td.d.SendTransaction(td.ctx, &prototk.SendTransactionRequest{
		StateQueryContext: td.c.id,
		Transaction: &prototk.TransactionInput{
			ContractAddress: "0x05d936207F04D81a85881b72A0D17854Ee8BE45A",
			FunctionAbiJson: `bad`,
			ParamsJson:      `{}`,
		},
	})
	require.ErrorContains(t, err, "invalid character")
}

func TestGetStatesFailCases(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()

	_, err := td.d.GetStatesByID(td.ctx, &prototk.GetStatesByIDRequest{
		StateQueryContext: "bad",
	})
	require.ErrorContains(t, err, "PD011649")

	_, err = td.d.GetStatesByID(td.ctx, &prototk.GetStatesByIDRequest{
		StateQueryContext: td.c.id,
		SchemaId:          "bad",
	})
	require.ErrorContains(t, err, "PD011641")

	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))
	td.mdc.On("GetStatesByID", mock.Anything, schemaID, []string{"id1"}).Return(nil, nil, fmt.Errorf("pop"))

	_, err = td.d.GetStatesByID(td.ctx, &prototk.GetStatesByIDRequest{
		StateQueryContext: td.c.id,
		SchemaId:          schemaID.String(),
		StateIds:          []string{"id1"},
	})
	require.EqualError(t, err, "pop")
}

func TestMapStateLockType(t *testing.T) {
	for _, pldType := range pldapi.StateLockType("").Options() {
		assert.NotNil(t, mapStateLockType(pldapi.StateLockType(pldType)))
	}
	assert.Panics(t, func() {
		_ = mapStateLockType(pldapi.StateLockType("wrong"))
	})
}

func TestDomainValidateStateHashesOK(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	td.tp.Functions.ValidateStateHashes = func(ctx context.Context, vshr *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
		assert.Equal(t, stateID1.String(), vshr.States[0].Id)
		assert.Empty(t, vshr.States[1].Id)
		return &prototk.ValidateStateHashesResponse{
			StateIds: []string{stateID1.String(), stateID2.String()},
		}, nil
	}

	// no-op
	validatedIDs, err := td.d.ValidateStateHashes(td.ctx, []*components.FullState{})
	require.NoError(t, err)
	assert.Equal(t, []pldtypes.HexBytes{}, validatedIDs)

	// Success
	validatedIDs, err = td.d.ValidateStateHashes(td.ctx, []*components.FullState{
		{ID: stateID1}, {ID: nil /* mocking domain calculation */},
	})
	require.NoError(t, err)
	assert.Equal(t, []pldtypes.HexBytes{stateID1, stateID2}, validatedIDs)
}

func TestDomainValidateStateHashesFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	td.tp.Functions.ValidateStateHashes = func(ctx context.Context, vshr *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := td.d.ValidateStateHashes(td.ctx, []*components.FullState{{ID: stateID1}})
	require.Regexp(t, "PD011651.*pop", err)
}

func TestDomainValidateStateHashesWrongLen(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	td.tp.Functions.ValidateStateHashes = func(ctx context.Context, vshr *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
		return &prototk.ValidateStateHashesResponse{
			StateIds: []string{stateID1.String(), stateID1.String()},
		}, nil
	}

	_, err := td.d.ValidateStateHashes(td.ctx, []*components.FullState{{ID: stateID1}})
	require.Regexp(t, "PD011652", err)
}

func TestDomainValidateStateHashesMisMatch(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	td.tp.Functions.ValidateStateHashes = func(ctx context.Context, vshr *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
		return &prototk.ValidateStateHashesResponse{
			StateIds: []string{stateID1.String(), stateID1.String() /* should be stateID2 */},
		}, nil
	}

	_, err := td.d.ValidateStateHashes(td.ctx, []*components.FullState{{ID: stateID1}, {ID: stateID2}})
	require.Regexp(t, "PD011652", err)
}

func TestDomainValidateStateHashesBadHex(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	td.tp.Functions.ValidateStateHashes = func(ctx context.Context, vshr *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
		return &prototk.ValidateStateHashesResponse{
			StateIds: []string{"wrong"},
		}, nil
	}

	_, err := td.d.ValidateStateHashes(td.ctx, []*components.FullState{{ID: stateID1}})
	require.Regexp(t, "PD011652", err)
}

func TestGetDomainReceiptAllAvailable(t *testing.T) {
	txID := uuid.New()

	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID3 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID4 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
			Return(&pldapi.TransactionStates{
				Spent:     []*pldapi.StateBase{{ID: stateID1}},
				Read:      []*pldapi.StateBase{{ID: stateID2}},
				Confirmed: []*pldapi.StateBase{{ID: stateID3}},
				Info:      []*pldapi.StateBase{{ID: stateID4}},
			}, nil)
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.tp.Functions.BuildReceipt = func(ctx context.Context, req *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
		require.True(t, req.Complete)
		require.Len(t, req.InputStates, 1)
		require.Equal(t, stateID1.String(), req.InputStates[0].Id)
		require.Len(t, req.ReadStates, 1)
		require.Equal(t, stateID2.String(), req.ReadStates[0].Id)
		require.Len(t, req.OutputStates, 1)
		require.Equal(t, stateID3.String(), req.OutputStates[0].Id)
		require.Len(t, req.InfoStates, 1)
		require.Equal(t, stateID4.String(), req.InfoStates[0].Id)

		return &prototk.BuildReceiptResponse{
			ReceiptJson: `{"some":"receipt"}`,
		}, nil
	}

	resData, err := td.d.GetDomainReceipt(td.ctx, td.c.dbTX, txID)
	require.NoError(t, err)
	require.JSONEq(t, `{"some":"receipt"}`, resData.Pretty())

}

func TestGetDomainReceiptIncomplete(t *testing.T) {
	txID := uuid.New()

	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
			Return(&pldapi.TransactionStates{
				Spent: []*pldapi.StateBase{{ID: stateID1}},
				Unavailable: &pldapi.UnavailableStates{
					Confirmed: []pldtypes.HexBytes{stateID2},
				},
			}, nil)
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.tp.Functions.BuildReceipt = func(ctx context.Context, req *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
		require.False(t, req.Complete)
		require.Len(t, req.InputStates, 1)

		return &prototk.BuildReceiptResponse{
			ReceiptJson: `{"some":"receipt"}`,
		}, nil
	}

	resData, err := td.d.GetDomainReceipt(td.ctx, td.c.dbTX, txID)
	require.NoError(t, err)
	require.JSONEq(t, `{"some":"receipt"}`, resData.Pretty())

}

func TestGetDomainReceiptFail(t *testing.T) {
	txID := uuid.New()

	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
			Return(&pldapi.TransactionStates{
				Spent: []*pldapi.StateBase{{ID: stateID1}},
			}, nil)
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.tp.Functions.BuildReceipt = func(ctx context.Context, req *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := td.d.GetDomainReceipt(td.ctx, td.c.dbTX, txID)
	assert.Regexp(t, "pop", err)

}

func TestGetDomainReceiptNotIndexed(t *testing.T) {
	txID := uuid.New()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
			Return(&pldapi.TransactionStates{None: true}, nil)
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	_, err := td.d.GetDomainReceipt(td.ctx, td.c.dbTX, txID)
	assert.Regexp(t, "PD011657", err) // no state confirmations available

}

func TestGetDomainReceiptMissingData(t *testing.T) {
	txID := uuid.New()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
			Return(&pldapi.TransactionStates{}, nil)
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	_, err := td.d.GetDomainReceipt(td.ctx, td.c.dbTX, txID)
	assert.Regexp(t, "PD011658", err) // no private states available

}

func TestGetDomainReceiptLookupError(t *testing.T) {
	txID := uuid.New()

	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas(), func(mc *mockComponents) {
		mc.stateStore.On("GetTransactionStates", mock.Anything, mock.Anything, txID).
			Return(nil, fmt.Errorf("pop"))
	})
	defer done()
	assert.Nil(t, td.d.initError.Load())

	_, err := td.d.GetDomainReceipt(td.ctx, td.c.dbTX, txID)
	assert.Regexp(t, "pop", err)

}

func TestDomainConfigurePrivacyGroupOk(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.tp.Functions.ConfigurePrivacyGroup = func(ctx context.Context, cpgr *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error) {
		require.Equal(t, map[string]string{"prop1": "value1"}, cpgr.InputConfiguration)
		return &prototk.ConfigurePrivacyGroupResponse{
			Configuration: map[string]string{
				"prop1": "value1",
				"prop2": "value2",
			},
		}, nil
	}

	domain := td.d
	props, err := domain.ConfigurePrivacyGroup(td.ctx, map[string]string{"prop1": "value1"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"prop1": "value1",
		"prop2": "value2",
	}, props)

}

func TestDomainConfigurePrivacyGroupFail(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.tp.Functions.ConfigurePrivacyGroup = func(ctx context.Context, cpgr *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	domain := td.d
	_, err := domain.ConfigurePrivacyGroup(td.ctx, map[string]string{"prop1": "value1"})
	require.Regexp(t, "pop", err)
}

func TestDomainInitPrivacyGroupOk(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	pgID := pldtypes.RandBytes(32)
	pgGenesis := &pldapi.PrivacyGroupGenesisState{
		Name:        "pg1",
		GenesisSalt: pldtypes.RandBytes32(),
		Members:     []string{"me@node1", "you@node2"},
		Properties: pldapi.KeyValueStringProperties{
			{Key: "prop1", Value: "value1"},
		},
		Configuration: pldapi.KeyValueStringProperties{
			{Key: "confA", Value: "valueA"},
		},
	}

	functionABI := &abi.Entry{Type: abi.Function, Name: "initPrivacyGroup"}
	addr := pldtypes.RandAddress()
	td.tp.Functions.InitPrivacyGroup = func(ctx context.Context, ipgr *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
		require.Equal(t, "pg1", ipgr.PrivacyGroup.Name)
		require.Equal(t, pgGenesis.GenesisSalt.String(), ipgr.PrivacyGroup.GenesisSalt)
		require.Equal(t, pgGenesis.Members, ipgr.PrivacyGroup.Members)
		require.Equal(t, pgGenesis.Properties.Map(), ipgr.PrivacyGroup.Properties)
		require.Equal(t, pgGenesis.Configuration.Map(), ipgr.PrivacyGroup.Configuration)
		return &prototk.InitPrivacyGroupResponse{
			Transaction: &prototk.PreparedTransaction{
				Type:            prototk.PreparedTransaction_PUBLIC, // less likely than private
				ContractAddress: confutil.P(addr.String()),          // less likely than deploy
				RequiredSigner:  confutil.P("some.signer"),          // less likely than rndom assignment
				ParamsJson:      `{"tx": "input"}`,
				FunctionAbiJson: pldtypes.JSONString(functionABI).Pretty(),
			},
		}, nil
	}

	domain := td.d
	tx, err := domain.InitPrivacyGroup(td.ctx, pgID, pgGenesis)
	require.NoError(t, err)
	require.Equal(t, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:   "some.signer",
			To:     addr,
			Type:   pldapi.TransactionTypePublic.Enum(),
			Data:   pldtypes.RawJSON(`{"tx": "input"}`),
			Domain: "test1",
		},
		ABI: abi.ABI{functionABI},
	}, tx)
}

func TestDomainInitPrivacyGroupError(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.tp.Functions.InitPrivacyGroup = func(ctx context.Context, ipgr *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	domain := td.d
	_, err := domain.InitPrivacyGroup(td.ctx, pldtypes.RandBytes(32), &pldapi.PrivacyGroupGenesisState{})
	assert.Regexp(t, "pop", err)

}

func TestDomainInitPrivacyGroupBadResFunctionABI(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.tp.Functions.InitPrivacyGroup = func(ctx context.Context, ipgr *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
		return &prototk.InitPrivacyGroupResponse{
			Transaction: &prototk.PreparedTransaction{},
		}, nil
	}

	domain := td.d
	_, err := domain.InitPrivacyGroup(td.ctx, pldtypes.RandBytes(32), &pldapi.PrivacyGroupGenesisState{})
	assert.Regexp(t, "PD011607", err)

}

func TestDomainInitPrivacyGroupBadResFromAddr(t *testing.T) {
	td, done := newTestDomain(t, false, goodDomainConf(), mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	td.tp.Functions.InitPrivacyGroup = func(ctx context.Context, ipgr *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
		return &prototk.InitPrivacyGroupResponse{
			Transaction: &prototk.PreparedTransaction{
				FunctionAbiJson: pldtypes.JSONString(&abi.Entry{Type: abi.Function, Name: "initPrivacyGroup"}).Pretty(),
				ContractAddress: confutil.P("wrong"),
			},
		}, nil
	}

	domain := td.d
	_, err := domain.InitPrivacyGroup(td.ctx, pldtypes.RandBytes(32), &pldapi.PrivacyGroupGenesisState{})
	assert.Regexp(t, "bad address", err)

}
