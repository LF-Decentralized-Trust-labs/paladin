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

package groupmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestRPCServer(t *testing.T, ctx context.Context, gm *groupManager) rpcclient.Client {

	s, err := rpcserver.NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)

	s.Register(gm.RPCModule())

	c := rpcclient.WrapRestyClient(resty.New().SetBaseURL(fmt.Sprintf("http://%s", s.HTTPAddr())))

	t.Cleanup(s.Stop)
	return c

}

func TestPrivacyGroupRPCLifecycleRealDB(t *testing.T) {

	mergedGenesis := `{
		"name": "secret things",
		"version": "200"
	}`
	ctx, gm, _, done := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{}, func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
			Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil)

		// Validate the init gets the correct data
		ipg := mc.domain.On("InitPrivacyGroup", mock.Anything, mock.Anything)
		ipg.Run(func(args mock.Arguments) {
			spec := args[1].(*pldapi.PrivacyGroupInput)
			require.Equal(t, "domain1", spec.Domain)
			require.JSONEq(t, `{"name": "secret things"}`, spec.Properties.Pretty())
			require.Len(t, spec.Members, 2)
			ipg.Return(
				tktypes.RawJSON(mergedGenesis),
				&abi.Parameter{
					Name:         "TestPrivacyGroup",
					Type:         "tuple",
					InternalType: "struct TestPrivacyGroup;",
					Indexed:      true,
					Components: append(spec.PropertiesABI, &abi.Parameter{
						Name:    "version",
						Type:    "uint256",
						Indexed: true,
					}),
				},
				nil,
			)
		})

		// Validate the state send gets the correct data
		mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			msg := args[2].(*components.ReliableMessage)
			require.Equal(t, components.RMTPrivacyGroup, msg.MessageType.V())
			var sd *components.StateDistribution
			err := json.Unmarshal(msg.Metadata, &sd)
			require.NoError(t, err)
			require.Equal(t, "domain1", sd.Domain)
			require.Empty(t, sd.ContractAddress)
			require.Equal(t, "you@node2", sd.IdentityLocator)
		})
	})
	defer done()

	client := newTestRPCServer(t, ctx, gm)
	pgroupRPC := pldclient.Wrap(client).PrivacyGroups()

	groupID, err := pgroupRPC.CreateGroup(ctx, &pldapi.PrivacyGroupInput{
		Domain:  "domain1",
		Members: []string{"me@node1", "you@node2"},
		Properties: tktypes.RawJSON(`{
			  "name": "secret things"
			}`),
	})
	require.NoError(t, err)
	require.NotNil(t, groupID)

	// Query it back - should be the only one
	groups, err := pgroupRPC.QueryGroups(ctx, query.NewQueryBuilder().Equal("domain", "domain1").Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "domain1", groups[0].Domain)
	require.Equal(t, groupID, groups[0].ID)
	require.NotNil(t, groups[0].Genesis)
	require.JSONEq(t, mergedGenesis, string(groups[0].Genesis))            // enriched from state store
	require.Equal(t, []string{"me@node1", "you@node2"}, groups[0].Members) // enriched from members table

	// Get it directly by ID
	group, err := pgroupRPC.GetGroupById(ctx, "domain1", groupID)
	require.NoError(t, err)
	require.NotNil(t, group)

	// Search for it by name
	groups, err = pgroupRPC.QueryGroupsByProperties(ctx, "domain1", group.GenesisSchema,
		query.NewQueryBuilder().Equal("name", "secret things").Equal("version", 200).Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "domain1", groups[0].Domain)
	require.Equal(t, groupID, groups[0].ID)
	require.NotNil(t, groups[0].Genesis)
	require.JSONEq(t, mergedGenesis, string(groups[0].Genesis))
	require.Equal(t, []string{"me@node1", "you@node2"}, groups[0].Members)

}
