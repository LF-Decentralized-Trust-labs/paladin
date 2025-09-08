// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statemgr

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRPCServer(t *testing.T) (context.Context, *stateManager, rpcclient.Client, *mockComponents, func()) {
	ctx, ss, m, ssDone := newDBTestStateManager(t)

	s, err := rpcserver.NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)

	s.Register(ss.RPCModule())

	c := rpcclient.WrapRestyClient(resty.New().SetBaseURL(fmt.Sprintf("http://%s", s.HTTPAddr())))

	return ctx, ss, c, m, func() { s.Stop(); ssDone() }

}

func jsonTestLog(t *testing.T, desc string, f interface{}) {
	b, err := json.MarshalIndent(f, "", "  ")
	require.NoError(t, err)
	fmt.Printf(desc+": %s\n", b)
}

func TestRPC(t *testing.T) {

	ctx, ss, c, m, done := newTestRPCServer(t)
	defer done()

	_ = mockDomain(t, m, "domain1", false)
	mockStateCallback(m)

	var abiParam abi.Parameter
	err := json.Unmarshal([]byte(widgetABI), &abiParam)
	require.NoError(t, err)
	schema, err := newABISchema(ctx, "domain1", &abiParam)
	assert.NoError(t, err)
	err = ss.persistSchemas(ctx, ss.p.NOTX(), []*pldapi.Schema{schema.Schema})
	assert.NoError(t, err)

	var schemas []*pldapi.Schema
	rpcErr := c.CallRPC(ctx, &schemas, "pstate_listSchemas", "domain1")
	jsonTestLog(t, "pstate_listSchemas", schemas)
	assert.Nil(t, rpcErr)
	assert.Len(t, schemas, 1)
	assert.Equal(t, pldapi.SchemaTypeABI, schemas[0].Type.V())
	assert.Equal(t, "0x3612029bf239cbed1e27548e9211ecfe72496dfec4183fd3ea79a3a54eb126be", schemas[0].ID.String())

	var rpcSchema *pldapi.Schema
	rpcErr = c.CallRPC(ctx, &rpcSchema, "pstate_getSchemaById", "domain1", schemas[0].ID)
	require.NoError(t, rpcErr)
	require.NotNil(t, rpcSchema)

	contractAddress := pldtypes.RandAddress()
	var state *pldapi.State
	rpcErr = c.CallRPC(ctx, &state, "pstate_storeState", "domain1", contractAddress.String(), schemas[0].ID, pldtypes.RawJSON(`{
	    "salt": "fd2724ce91a859e24c228e50ae17b9443454514edce9a64437c208b0184d8910",
		"size": 10,
		"color": "blue",
		"price": "1230000000000000000"
	}`))
	jsonTestLog(t, "pstate_storeState", state)
	assert.Nil(t, rpcErr)
	if rpcErr != nil {
		assert.NoError(t, rpcErr)
	}
	assert.Equal(t, schemas[0].ID, state.Schema)
	assert.Equal(t, "domain1", state.DomainName)
	assert.Equal(t, "0x30e278bca8d876cdceb24520b0ebe736a64a9cb8019157f40fa5b03f083f824d", state.ID.String())

	var states []*pldapi.State
	rpcErr = c.CallRPC(ctx, &states, "pstate_queryContractStates", "domain1", contractAddress.String(), schemas[0].ID, pldtypes.RawJSON(`{
		"eq": [{
		  "field": "color",
		  "value": "blue"
		}]
	}`), "all")
	jsonTestLog(t, "pstate_queryContractStates", states)
	assert.Nil(t, rpcErr)
	assert.Len(t, states, 1)
	assert.Equal(t, state, states[0])

	rpcErr = c.CallRPC(ctx, &states, "pstate_queryStates", "domain1", schemas[0].ID, pldtypes.RawJSON(`{
		"eq": [{
		  "field": "color",
		  "value": "blue"
		}]
	}`), "all")
	jsonTestLog(t, "pstate_queryStates", states)
	assert.Nil(t, rpcErr)
	assert.Len(t, states, 1)
	assert.Equal(t, state, states[0])

	// Write some nullifiers and query them back
	nullifier1 := pldtypes.HexBytes(pldtypes.RandHex(32))
	err = ss.WriteNullifiersForReceivedStates(ctx, ss.p.NOTX(), "domain1", []*components.NullifierUpsert{
		{
			ID:    nullifier1,
			State: state.ID,
		},
	})
	require.NoError(t, err)

	rpcErr = c.CallRPC(ctx, &states, "pstate_queryContractNullifiers", "domain1", contractAddress.String(), schemas[0].ID, pldtypes.RawJSON(`{
		"eq": [{
		  "field": "color",
		  "value": "blue"
		}]
	}`), "all")
	jsonTestLog(t, "pstate_queryContractNullifiers", states)
	assert.Nil(t, rpcErr)
	assert.Len(t, states, 1)
	assert.Equal(t, state.ID, states[0].ID)
	assert.Equal(t, nullifier1, states[0].Nullifier.ID)

	rpcErr = c.CallRPC(ctx, &states, "pstate_queryNullifiers", "domain1", schemas[0].ID, pldtypes.RawJSON(`{
		"eq": [{
		  "field": "color",
		  "value": "blue"
		}]
	}`), "all")
	jsonTestLog(t, "pstate_queryNullifiers", states)
	assert.Nil(t, rpcErr)
	assert.Len(t, states, 1)
	assert.Equal(t, state.ID, states[0].ID)
	assert.Equal(t, nullifier1, states[0].Nullifier.ID)

}
