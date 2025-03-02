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
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
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
	contractAddr := tktypes.RandAddress()
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
				&components.PreparedGroupInitTransaction{
					TX: &pldapi.TransactionInput{
						TransactionBase: pldapi.TransactionBase{
							Type: pldapi.TransactionTypePrivate.Enum(),
						},
					},
					GenesisState: tktypes.RawJSON(mergedGenesis),
					GenesisSchema: &abi.Parameter{
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
				},
				nil,
			)
		})

		deployTXID := uuid.New()
		mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).
			Return([]uuid.UUID{deployTXID}, nil).
			Run(func(args mock.Arguments) {
				tx := args[2].(*pldapi.TransactionInput)
				assert.Regexp(t, `domains\.domain1\.pgroupinit\.0x[0-9a-f]{32}`, tx.From)
				assert.Equal(t, "tx_1", tx.IdempotencyKey)
				assert.Equal(t, uint64(12345), tx.PublicTxOptions.Gas.Uint64())
			}).Once()

		// Validate the state send gets the correct data
		mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(rm *pldapi.ReliableMessage) bool {
			return rm.MessageType.V() == pldapi.RMTPrivacyGroup
		})).Return(nil).Run(func(args mock.Arguments) {
			msg := args[2].(*pldapi.ReliableMessage)
			require.Equal(t, pldapi.RMTPrivacyGroup, msg.MessageType.V())
			var pgd *components.PrivacyGroupDistribution
			err := json.Unmarshal(msg.Metadata, &pgd)
			require.NoError(t, err)
			require.Equal(t, "domain1", pgd.GenesisState.Domain)
			require.Empty(t, pgd.GenesisState.ContractAddress)
			require.Equal(t, "you@node2", pgd.GenesisState.IdentityLocator)
			require.Equal(t, deployTXID, pgd.GenesisTransaction)
		})

		psc := componentmocks.NewDomainSmartContract(t)
		mc.domainManager.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *contractAddr).Return(psc, nil)

		mwpgt1 := psc.On("WrapPrivacyGroupEVMTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Once()
		mwpgt1.Run(func(args mock.Arguments) {
			pg := args[1].(*pldapi.PrivacyGroupWithABI)
			require.NotNil(t, pg.GenesisABI)
			mwpgt1.Return(&pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					Domain: pg.Domain,
					Type:   pldapi.TransactionTypePrivate.Enum(),
					From:   `my.key`,
					Data:   tktypes.RawJSON(`{"wrapped":"transaction"}`),
				},
			}, nil)
		})

		tx1ID := uuid.New()
		mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).
			Return([]uuid.UUID{tx1ID}, nil).
			Run(func(args mock.Arguments) {
				tx := args[2].(*pldapi.TransactionInput)
				assert.Regexp(t, `my.key`, tx.From)
				assert.Equal(t, "pgtx_deploy", tx.IdempotencyKey)
				assert.JSONEq(t, `{"wrapped":"transaction"}`, tx.Data.Pretty())
			}).Once()

		mwpgt2 := psc.On("WrapPrivacyGroupEVMTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		mwpgt2.Run(func(args mock.Arguments) {
			pg := args[1].(*pldapi.PrivacyGroupWithABI)
			require.NotNil(t, pg.GenesisABI)
			mwpgt2.Return(&pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					Domain: pg.Domain,
					To:     tktypes.RandAddress(),
					Type:   pldapi.TransactionTypePrivate.Enum(),
					Data:   tktypes.RawJSON(`{"wrapped":"call"}`),
				},
			}, nil)
		})

		mc.txManager.On("CallTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil).
			Run(func(args mock.Arguments) {
				tx := args[3].(*pldapi.TransactionCall)
				assert.Empty(t, tx.From)
				assert.NotNil(t, tx.To)
				assert.JSONEq(t, `{"wrapped":"call"}`, tx.Data.Pretty())
				res := args[2].(*tktypes.RawJSON)
				*res = tktypes.RawJSON(`{"call":"result"}`)
			}).Once()

		// Validate we also get a send reliable for the message
		mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(rm *pldapi.ReliableMessage) bool {
			return rm.MessageType.V() == pldapi.RMTPrivacyGroupMessage
		})).Return(nil)
	})
	defer done()

	client := newTestRPCServer(t, ctx, gm)
	pgroupRPC := pldclient.Wrap(client).PrivacyGroups()

	group1, err := pgroupRPC.CreateGroup(ctx, &pldapi.PrivacyGroupInput{
		Domain:  "domain1",
		Members: []string{"me@node1", "you@node2"},
		Properties: tktypes.RawJSON(`{
			  "name": "secret things"
			}`),
		TransactionOptions: &pldapi.PrivacyGroupTXOptions{
			IdempotencyKey: "tx_1",
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(tktypes.HexUint64(12345)),
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group1)
	groupID := group1.ID
	require.Equal(t, []string{"me@node1", "you@node2"}, group1.Members)
	require.NotNil(t, group1.Genesis)

	// Query it back - should be the only one
	groups, err := pgroupRPC.QueryGroups(ctx, query.NewQueryBuilder().Equal("domain", "domain1").Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "domain1", groups[0].Domain)
	require.Equal(t, groupID, groups[0].ID)
	require.NotNil(t, groups[0].Genesis)
	require.JSONEq(t, mergedGenesis, string(groups[0].Genesis))            // enriched from state store
	require.Equal(t, []string{"me@node1", "you@node2"}, groups[0].Members) // enriched from members table

	// Simulate completion of the transaction so we have the contract address
	err = gm.p.DB().Exec(`INSERT INTO transaction_receipts ("transaction", domain, indexed, success, contract_address) VALUES ( ?, ?, ?, ?, ? )`,
		groups[0].GenesisTransaction,
		groups[0].Domain,
		tktypes.TimestampNow(),
		true,
		contractAddr,
	).Error
	require.NoError(t, err)

	// Get it directly by ID
	group, err := pgroupRPC.GetGroupById(ctx, "domain1", groupID)
	require.NoError(t, err)
	require.NotNil(t, group)
	require.Equal(t, contractAddr, group.ContractAddress)

	// Search for it by name
	groups, err = pgroupRPC.QueryGroupsByProperties(ctx, "domain1", group.GenesisSchema,
		query.NewQueryBuilder().Equal("name", "secret things").Equal("version", 200).Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, contractAddr, groups[0].ContractAddress)
	require.Equal(t, "domain1", groups[0].Domain)
	require.Equal(t, groupID, groups[0].ID)
	require.NotNil(t, groups[0].Genesis)
	require.JSONEq(t, mergedGenesis, string(groups[0].Genesis))
	require.Equal(t, []string{"me@node1", "you@node2"}, groups[0].Members)

	// Send a transaction to it
	tx1ID, err := pgroupRPC.SendTransaction(ctx, &pldapi.PrivacyGroupEVMTXInput{
		Domain:         "domain1",
		Group:          group.ID,
		IdempotencyKey: "pgtx_deploy",
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			From:     "my.key",
			To:       nil, // simulate is a deploy inside the privacy group
			Function: &abi.Entry{Type: abi.Constructor, Inputs: abi.ParameterArray{{Type: "string", Name: "input1"}}},
			Gas:      confutil.P(tktypes.HexUint64(12345)),
			Value:    tktypes.Int64ToInt256(123456789),
			Input:    tktypes.RawJSON(`{"input1": "value1"}`),
			Bytecode: tktypes.MustParseHexBytes(`0xfeedbeef`),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, tx1ID)

	// Do a call via it
	callData, err := pgroupRPC.Call(ctx, &pldapi.PrivacyGroupEVMCall{
		Domain: "domain1",
		Group:  group.ID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			To:       tktypes.RandAddress(),
			Function: &abi.Entry{Type: abi.Function, Name: "getThing"},
		},
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"call":"result"}`, callData.Pretty())

	// Send a messages
	cid := uuid.New()
	msgID, err := pgroupRPC.SendMessage(ctx, &pldapi.PrivacyGroupMessageInput{
		Domain:        "domain1",
		Group:         groupID,
		Data:          tktypes.JSONString("some data"),
		Topic:         "my/topic",
		CorrelationID: &cid,
	})
	require.NoError(t, err)

	// Query by ID
	msgByID, err := pgroupRPC.GetMessageById(ctx, msgID)
	require.NoError(t, err)
	require.Equal(t, msgID, msgByID.ID)
	require.NotEqual(t, msgID, (uuid.UUID{}))

	// Query by correlation ID
	msgByCID, err := pgroupRPC.QueryMessages(ctx, query.NewQueryBuilder().Equal("correlationId", cid).Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, msgByCID, 1)
	require.Equal(t, msgID, msgByCID[0].ID)

}
