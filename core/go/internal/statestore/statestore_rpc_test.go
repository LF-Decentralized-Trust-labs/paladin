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

package statestore

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/internal/httpserver"
	"github.com/kaleido-io/paladin/core/internal/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRPCServer(t *testing.T) (context.Context, rpcbackend.Backend, func()) {
	ctx, ss, ssDone := newDBTestStateStore(t)

	s, err := rpcserver.NewRPCServer(ctx, &rpcserver.Config{
		HTTP: rpcserver.HTTPEndpointConfig{
			Config: httpserver.Config{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)},
		},
		WS: rpcserver.WSEndpointConfig{Disabled: true},
	})
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)

	s.Register(ss.RPCModule())

	c := rpcbackend.NewRPCClient(resty.New().SetBaseURL(fmt.Sprintf("http://%s", s.HTTPAddr())))

	return ctx, c, func() { s.Stop(); ssDone() }

}

func jsonTestLog(t *testing.T, desc string, f interface{}) {
	b, err := json.MarshalIndent(f, "", "  ")
	require.NoError(t, err)
	fmt.Printf(desc+": %s\n", b)
}

func TestRPC(t *testing.T) {

	ctx, c, done := newTestRPCServer(t)
	defer done()

	var schema tktypes.RawJSON
	rpcErr := c.CallRPC(ctx, &schema, "pstate_storeABISchema", "domain1", tktypes.RawJSON(widgetABI))
	jsonTestLog(t, "pstate_storeABISchema", schema)
	assert.Nil(t, rpcErr)

	var schemas []*SchemaPersisted
	rpcErr = c.CallRPC(ctx, &schemas, "pstate_listSchemas", "domain1")
	jsonTestLog(t, "pstate_listSchemas", schemas)
	assert.Nil(t, rpcErr)
	assert.Len(t, schemas, 1)
	assert.Equal(t, SchemaTypeABI, schemas[0].Type)
	assert.Equal(t, "0x3612029bf239cbed1e27548e9211ecfe72496dfec4183fd3ea79a3a54eb126be", schemas[0].ID.String())

	var state *State
	rpcErr = c.CallRPC(ctx, &state, "pstate_storeState", "domain1", schemas[0].ID, tktypes.RawJSON(`{
	    "salt": "fd2724ce91a859e24c228e50ae17b9443454514edce9a64437c208b0184d8910",
		"size": 10,
		"color": "blue",
		"price": "1230000000000000000"
	}`))
	jsonTestLog(t, "pstate_storeState", state)
	assert.Nil(t, rpcErr)
	assert.Equal(t, schemas[0].ID, state.Schema)
	assert.Equal(t, "domain1", state.DomainID)
	assert.Equal(t, "0x30e278bca8d876cdceb24520b0ebe736a64a9cb8019157f40fa5b03f083f824d", state.ID.String())

	var states []*State
	rpcErr = c.CallRPC(ctx, &states, "pstate_queryStates", "domain1", schemas[0].ID, tktypes.RawJSON(`{
		"eq": [{
		  "field": "color",
		  "value": "blue"
		}]
	}`), "all")
	jsonTestLog(t, "pstate_storeState", states)
	assert.Nil(t, rpcErr)
	assert.Len(t, states, 1)
	assert.Equal(t, state, states[0])

}
