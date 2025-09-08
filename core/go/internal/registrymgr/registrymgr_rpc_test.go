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

package registrymgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRPCQuery(t *testing.T) {
	ctx, rm, tp, _, done := newTestRegistry(t, true)
	defer done()

	rpc, rpcDone := newTestRPCServer(t, ctx, rm)
	defer rpcDone()

	var registryNames []string
	err := rpc.CallRPC(ctx, &registryNames, "reg_registries")
	require.NoError(t, err)
	assert.Equal(t, []string{tp.r.name}, registryNames)

	// Register something to query
	entry1 := &prototk.RegistryEntry{Id: randID(), Name: "entry1", Active: true}
	res, regErr := tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Entries:    []*prototk.RegistryEntry{entry1},
		Properties: []*prototk.RegistryProperty{newPropFor(entry1.Id, "prop1", "value1")},
	})
	require.NoError(t, regErr)
	assert.NotNil(t, res)

	var entries []*pldapi.RegistryEntry
	err = rpc.CallRPC(ctx, &entries, "reg_queryEntries", tp.r.name,
		query.NewQueryBuilder().Equal(".name", "entry1").Null(".parentId").Equal("prop1", "value1").Limit(1).Query(), "active")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "entry1", entries[0].Name)

	err = rpc.CallRPC(ctx, &entries, "reg_queryEntries", "unknown", query.NewQueryBuilder().Limit(1).Query(), "active")
	assert.Regexp(t, "PD012101", err)

	var entriesWithProps []*pldapi.RegistryEntryWithProperties
	err = rpc.CallRPC(ctx, &entriesWithProps, "reg_queryEntriesWithProps", tp.r.name,
		query.NewQueryBuilder().Equal(".name", "entry1").Null(".parentId").Equal("prop1", "value1").Limit(1).Query(), "active")
	require.NoError(t, err)
	require.Len(t, entriesWithProps, 1)
	require.Equal(t, "entry1", entriesWithProps[0].Name)
	require.Equal(t, "value1", entriesWithProps[0].Properties["prop1"])

	var props []*pldapi.RegistryProperty
	err = rpc.CallRPC(ctx, &props, "reg_getEntryProperties", tp.r.name, entries[0].ID, "active")
	require.NoError(t, err)
	require.Len(t, props, 1)
	require.Equal(t, "prop1", props[0].Name)
	require.Equal(t, "value1", props[0].Value)

}

func newTestRPCServer(t *testing.T, ctx context.Context, rm *registryManager) (rpcclient.Client, func()) {

	s, err := rpcserver.NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)

	s.Register(rm.RPCModule())

	c := rpcclient.WrapRestyClient(resty.New().SetBaseURL(fmt.Sprintf("http://%s", s.HTTPAddr())))

	return c, s.Stop

}
