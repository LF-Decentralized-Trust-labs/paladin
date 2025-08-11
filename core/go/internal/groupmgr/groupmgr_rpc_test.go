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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
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

	contractAddr := pldtypes.RandAddress()
	ctx, gm, _, done := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{}, func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
			Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil)

		// Validate the init gets the correct data
		cpg := mc.domain.On("ConfigurePrivacyGroup", mock.Anything, mock.Anything)
		cpg.Run(func(args mock.Arguments) {
			inputconf := args[1].(map[string]string)
			inputconf["extra"] = "extra1"
			cpg.Return(inputconf, nil)
		})
		ipg := mc.domain.On("InitPrivacyGroup", mock.Anything, mock.Anything, mock.Anything)
		ipg.Run(func(args mock.Arguments) {
			spec := args[2].(*pldapi.PrivacyGroupGenesisState)
			require.Equal(t, map[string]string{"name": "secret things"}, spec.Properties.Map())
			require.Len(t, spec.Members, 2)
			ipg.Return(
				&pldapi.TransactionInput{
					TransactionBase: pldapi.TransactionBase{
						Type: pldapi.TransactionTypePrivate.Enum(),
					},
				},
				nil,
			)
		})

		deployTXID := uuid.New()
		mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).
			Return([]uuid.UUID{deployTXID}, nil).
			Run(func(args mock.Arguments) {
				tx := args[2].([]*pldapi.TransactionInput)[0]
				assert.Regexp(t, `domains\.domain1\.pgroupinit\.0x[0-9a-f]{32}`, tx.From)
				assert.Equal(t, "tx_1", tx.IdempotencyKey)
				assert.Equal(t, uint64(12345), tx.PublicTxOptions.Gas.Uint64())
			}).Once()

		// Validate the state send gets the correct data
		mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(rm []*pldapi.ReliableMessage) bool {
			return rm[0].MessageType.V() == pldapi.RMTPrivacyGroup
		})).Return(nil).Run(func(args mock.Arguments) {
			msg := args[2].([]*pldapi.ReliableMessage)[0]
			require.Equal(t, pldapi.RMTPrivacyGroup, msg.MessageType.V())
			var pgd *components.PrivacyGroupDistribution
			err := json.Unmarshal(msg.Metadata, &pgd)
			require.NoError(t, err)
			require.Equal(t, "domain1", pgd.GenesisState.Domain)
			require.Empty(t, pgd.GenesisState.ContractAddress)
			require.Equal(t, "you@node2", pgd.GenesisState.IdentityLocator)
			require.Equal(t, deployTXID, pgd.GenesisTransaction)
		})

		psc := componentsmocks.NewDomainSmartContract(t)
		mc.domainManager.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *contractAddr).Return(psc, nil)

		mwpgt1 := psc.On("WrapPrivacyGroupEVMTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Once()
		mwpgt1.Run(func(args mock.Arguments) {
			mwpgt1.Return(&pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					Type: pldapi.TransactionTypePrivate.Enum(),
					From: `my.key`,
					Data: pldtypes.RawJSON(`{"wrapped":"transaction"}`),
				},
			}, nil)
		})

		tx1ID := uuid.New()
		mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).
			Return([]uuid.UUID{tx1ID}, nil).
			Run(func(args mock.Arguments) {
				tx := args[2].([]*pldapi.TransactionInput)[0]
				assert.Regexp(t, `my.key`, tx.From)
				assert.Equal(t, "pgtx_deploy", tx.IdempotencyKey)
				assert.JSONEq(t, `{"wrapped":"transaction"}`, tx.Data.Pretty())
			}).Once()

		mwpgt2 := psc.On("WrapPrivacyGroupEVMTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		mwpgt2.Run(func(args mock.Arguments) {
			mwpgt2.Return(&pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					To:   pldtypes.RandAddress(),
					Type: pldapi.TransactionTypePrivate.Enum(),
					Data: pldtypes.RawJSON(`{"wrapped":"call"}`),
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
				res := args[2].(*pldtypes.RawJSON)
				*res = pldtypes.RawJSON(`{"call":"result"}`)
			}).Once()

		// Validate we also get a send reliable for the message
		mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(rm []*pldapi.ReliableMessage) bool {
			return rm[0].MessageType.V() == pldapi.RMTPrivacyGroupMessage
		})).Return(nil)
	})
	defer done()

	client := newTestRPCServer(t, ctx, gm)
	pgroupRPC := pldclient.Wrap(client).PrivacyGroups()

	group1, err := pgroupRPC.CreateGroup(ctx, &pldapi.PrivacyGroupInput{
		Domain:     "domain1",
		Name:       "secret.things",
		Members:    []string{"me@node1", "you@node2"},
		Properties: map[string]string{"name": "secret things"},
		TransactionOptions: &pldapi.PrivacyGroupTXOptions{
			IdempotencyKey: "tx_1",
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(pldtypes.HexUint64(12345)),
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group1)
	groupID := group1.ID
	require.Equal(t, []string{"me@node1", "you@node2"}, group1.Members)

	// Query it back - should be the only one
	groups, err := pgroupRPC.QueryGroups(ctx, query.NewQueryBuilder().Equal("domain", "domain1").Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "domain1", groups[0].Domain)
	require.Equal(t, groupID, groups[0].ID)
	require.Equal(t, []string{"me@node1", "you@node2"}, groups[0].Members) // enriched from members table

	// Simulate completion of the transaction so we have the contract address
	err = gm.p.DB().Exec(`INSERT INTO transaction_receipts ("transaction", domain, indexed, success, contract_address) VALUES ( ?, ?, ?, ?, ? )`,
		groups[0].GenesisTransaction,
		groups[0].Domain,
		pldtypes.TimestampNow(),
		true,
		contractAddr,
	).Error
	require.NoError(t, err)

	// Get it directly by ID
	fullGroup, err := pgroupRPC.GetGroupById(ctx, "domain1", groupID)
	require.NoError(t, err)
	require.NotNil(t, fullGroup)
	require.Equal(t, contractAddr, fullGroup.ContractAddress)

	// Get it by address
	group, err := pgroupRPC.GetGroupByAddress(ctx, *contractAddr)
	require.NoError(t, err)
	require.NotNil(t, group)

	// Get it by checking member
	groupsWithMember, err := pgroupRPC.QueryGroupsWithMember(ctx, "you@node2", query.NewQueryBuilder().Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, groupsWithMember, 1)

	// Search for it by name
	groups, err = pgroupRPC.QueryGroups(ctx, query.NewQueryBuilder().Equal("name", "secret.things").Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, contractAddr, groups[0].ContractAddress)
	require.Equal(t, "domain1", groups[0].Domain)
	require.Equal(t, groupID, groups[0].ID)
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
			Gas:      confutil.P(pldtypes.HexUint64(12345)),
			Value:    pldtypes.Int64ToInt256(123456789),
			Input:    pldtypes.RawJSON(`{"input1": "value1"}`),
			Bytecode: pldtypes.MustParseHexBytes(`0xfeedbeef`),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, tx1ID)

	// Do a call via it
	callData, err := pgroupRPC.Call(ctx, &pldapi.PrivacyGroupEVMCall{
		Domain: "domain1",
		Group:  group.ID,
		PrivacyGroupEVMTX: pldapi.PrivacyGroupEVMTX{
			To:       pldtypes.RandAddress(),
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
		Data:          pldtypes.JSONString("some data"),
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

func TestRCPMessageListenersCRUDRealDB(t *testing.T) {
	ctx, gm, _, done := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{})
	defer done()

	client := newTestRPCServer(t, ctx, gm)
	pgroupRPC := pldclient.Wrap(client).PrivacyGroups()

	// Create listener in default (started)
	boolRes, err := pgroupRPC.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name: "listener1",
	})
	require.NoError(t, err)
	require.True(t, boolRes)

	// Duplpicate
	_, err = pgroupRPC.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name: "listener1",
	})
	require.Regexp(t, "PD012507.*listener1", err)

	// should be queryable
	listeners, err := pgroupRPC.QueryMessageListeners(ctx, query.NewQueryBuilder().Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, listeners, 1)
	assert.Equal(t, listeners[0].Name, "listener1")

	// should be started
	l, err := pgroupRPC.GetMessageListener(ctx, "listener1")
	require.NoError(t, err)
	require.NotNil(t, l)
	assert.True(t, *l.Started)

	// delete listener
	_, err = pgroupRPC.DeleteMessageListener(ctx, "listener1")
	require.NoError(t, err)
	l, err = pgroupRPC.GetMessageListener(ctx, "listener1")
	require.NoError(t, err)
	require.Nil(t, l)

	// Create listener stopped
	boolRes, err = pgroupRPC.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)
	require.True(t, boolRes)

	// should be stopped
	l, err = pgroupRPC.GetMessageListener(ctx, "listener1")
	require.NoError(t, err)
	assert.False(t, *l.Started)

	// start it
	boolRes, err = pgroupRPC.StartMessageListener(ctx, "listener1")
	require.NoError(t, err)
	require.True(t, boolRes)

	// should be started
	l, err = pgroupRPC.GetMessageListener(ctx, "listener1")
	require.NoError(t, err)
	assert.True(t, *l.Started)

	// stop it
	boolRes, err = pgroupRPC.StopMessageListener(ctx, "listener1")
	require.NoError(t, err)
	require.True(t, boolRes)

	// should be stopped
	l, err = pgroupRPC.GetMessageListener(ctx, "listener1")
	require.NoError(t, err)
	assert.False(t, *l.Started)

	// Simulate restart so we can do startup processing
	gm.messagesInit()

	// Force persistent state to be started
	err = gm.p.DB().Model(&persistedMessageListener{}).
		Where("name = ?", "listener1").Update("started", true).Error
	require.NoError(t, err)

	// Load the listeners
	err = gm.loadMessageListeners()
	require.NoError(t, err)

	// Check it's not actually started (yet)
	require.Nil(t, gm.messageListeners["listener1"].done)

	// Do the startup
	gm.startMessageListeners()

	// Check it's started now
	l, err = pgroupRPC.GetMessageListener(ctx, "listener1")
	require.NoError(t, err)
	assert.True(t, *l.Started)

	// Check it's now actually started
	require.NotNil(t, gm.messageListeners["listener1"].done)

}
