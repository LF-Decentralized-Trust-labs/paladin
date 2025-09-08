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

package pldclient

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/google/uuid"
)

type PrivacyGroups interface {
	RPCModule

	CreateGroup(ctx context.Context, spec *pldapi.PrivacyGroupInput) (group pldapi.PrivacyGroup, err error)
	GetGroupById(ctx context.Context, domainName string, id pldtypes.HexBytes) (group *pldapi.PrivacyGroup, err error)
	GetGroupByAddress(ctx context.Context, addr pldtypes.EthAddress) (group *pldapi.PrivacyGroup, err error)
	QueryGroups(ctx context.Context, jq *query.QueryJSON) (groups []*pldapi.PrivacyGroup, err error)
	QueryGroupsWithMember(ctx context.Context, member string, jq *query.QueryJSON) (groups []*pldapi.PrivacyGroup, err error)
	SendTransaction(ctx context.Context, tx *pldapi.PrivacyGroupEVMTXInput) (txID uuid.UUID, err error)
	Call(ctx context.Context, call *pldapi.PrivacyGroupEVMCall) (data pldtypes.RawJSON, err error)

	SendMessage(ctx context.Context, msg *pldapi.PrivacyGroupMessageInput) (msgID uuid.UUID, err error)
	GetMessageById(ctx context.Context, id uuid.UUID) (msg *pldapi.PrivacyGroupMessage, err error)
	QueryMessages(ctx context.Context, q *query.QueryJSON) (msgs []*pldapi.PrivacyGroupMessage, err error)

	CreateMessageListener(ctx context.Context, listener *pldapi.PrivacyGroupMessageListener) (success bool, err error)
	QueryMessageListeners(ctx context.Context, jq *query.QueryJSON) (listeners []*pldapi.PrivacyGroupMessageListener, err error)
	GetMessageListener(ctx context.Context, listenerName string) (listener *pldapi.PrivacyGroupMessageListener, err error)
	StartMessageListener(ctx context.Context, listenerName string) (success bool, err error)
	StopMessageListener(ctx context.Context, listenerName string) (success bool, err error)
	DeleteMessageListener(ctx context.Context, listenerName string) (success bool, err error)

	SubscribeMessages(ctx context.Context, listenerName string) (sub rpcclient.Subscription, err error)
}

var pgroupReceiptsSubscriptionConfig = rpcclient.SubscriptionConfig{
	SubscribeMethod:    "pgroup_subscribe",
	UnsubscribeMethod:  "pgroup_unsubscribe",
	NotificationMethod: "pgroup_subscription",
	AckMethod:          "pgroup_ack",
	NackMethod:         "pgroup_nack",
}

// This is necessary because there's no way to introspect function parameter names via reflection
var privacyGroupsInfo = &rpcModuleInfo{
	group: "pgroup",
	methodInfo: map[string]RPCMethodInfo{
		"pgroup_createGroup": {
			Inputs: []string{"spec"},
			Output: "group",
		},
		"pgroup_getGroupById": {
			Inputs: []string{"domainName", "id"},
			Output: "pgroup",
		},
		"pgroup_getGroupByAddress": {
			Inputs: []string{"address"},
			Output: "pgroup",
		},
		"pgroup_queryGroups": {
			Inputs: []string{"query"},
			Output: "pgroups",
		},
		"pgroup_queryGroupsWithMember": {
			Inputs: []string{"member", "query"},
			Output: "pgroups",
		},
		"pgroup_sendTransaction": {
			Inputs: []string{"tx"},
			Output: "transactionId",
		},
		"pgroup_call": {
			Inputs: []string{"call"},
			Output: "data",
		},
		"pgroup_sendMessage": {
			Inputs: []string{"msg"},
			Output: "msgId",
		},
		"pgroup_getMessageById": {
			Inputs: []string{"id"},
			Output: "msg",
		},
		"pgroup_queryMessages": {
			Inputs: []string{"query"},
			Output: "msgs",
		},
		"pgroup_createMessageListener": {
			Inputs: []string{"listener"},
			Output: "success",
		},
		"pgroup_queryMessageListeners": {
			Inputs: []string{"query"},
			Output: "listeners",
		},
		"pgroup_getMessageListener": {
			Inputs: []string{"listenerName"},
			Output: "listener",
		},
		"pgroup_startMessageListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
		"pgroup_stopMessageListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
		"pgroup_deleteMessageListener": {
			Inputs: []string{"listenerName"},
			Output: "success",
		},
	},
	subscriptions: []RPCSubscriptionInfo{
		{
			SubscriptionConfig: pgroupReceiptsSubscriptionConfig,
			FixedInputs:        []string{"messages"},
			Inputs:             []string{"listenerName"},
		},
	},
}

type pgroup struct {
	*rpcModuleInfo
	c *paladinClient
}

func (c *paladinClient) PrivacyGroups() PrivacyGroups {
	return &pgroup{rpcModuleInfo: privacyGroupsInfo, c: c}
}

func (r *pgroup) CreateGroup(ctx context.Context, spec *pldapi.PrivacyGroupInput) (group pldapi.PrivacyGroup, err error) {
	err = r.c.CallRPC(ctx, &group, "pgroup_createGroup", spec)
	return
}

func (r *pgroup) GetGroupById(ctx context.Context, domainName string, id pldtypes.HexBytes) (group *pldapi.PrivacyGroup, err error) {
	err = r.c.CallRPC(ctx, &group, "pgroup_getGroupById", domainName, id)
	return
}

func (r *pgroup) GetGroupByAddress(ctx context.Context, addr pldtypes.EthAddress) (group *pldapi.PrivacyGroup, err error) {
	err = r.c.CallRPC(ctx, &group, "pgroup_getGroupByAddress", addr)
	return
}

func (r *pgroup) QueryGroups(ctx context.Context, jq *query.QueryJSON) (groups []*pldapi.PrivacyGroup, err error) {
	err = r.c.CallRPC(ctx, &groups, "pgroup_queryGroups", jq)
	return
}

func (r *pgroup) QueryGroupsWithMember(ctx context.Context, member string, jq *query.QueryJSON) (groups []*pldapi.PrivacyGroup, err error) {
	err = r.c.CallRPC(ctx, &groups, "pgroup_queryGroupsWithMember", member, jq)
	return
}

func (r *pgroup) SendTransaction(ctx context.Context, tx *pldapi.PrivacyGroupEVMTXInput) (txID uuid.UUID, err error) {
	err = r.c.CallRPC(ctx, &txID, "pgroup_sendTransaction", tx)
	return
}

func (r *pgroup) Call(ctx context.Context, call *pldapi.PrivacyGroupEVMCall) (data pldtypes.RawJSON, err error) {
	err = r.c.CallRPC(ctx, &data, "pgroup_call", call)
	return
}

func (r *pgroup) SendMessage(ctx context.Context, msg *pldapi.PrivacyGroupMessageInput) (msgID uuid.UUID, err error) {
	err = r.c.CallRPC(ctx, &msgID, "pgroup_sendMessage", msg)
	return
}

func (r *pgroup) GetMessageById(ctx context.Context, id uuid.UUID) (msg *pldapi.PrivacyGroupMessage, err error) {
	err = r.c.CallRPC(ctx, &msg, "pgroup_getMessageById", id)
	return
}

func (r *pgroup) QueryMessages(ctx context.Context, jq *query.QueryJSON) (msgs []*pldapi.PrivacyGroupMessage, err error) {
	err = r.c.CallRPC(ctx, &msgs, "pgroup_queryMessages", jq)
	return
}

func (r *pgroup) CreateMessageListener(ctx context.Context, listener *pldapi.PrivacyGroupMessageListener) (success bool, err error) {
	err = r.c.CallRPC(ctx, &success, "pgroup_createMessageListener", listener)
	return
}

func (r *pgroup) QueryMessageListeners(ctx context.Context, jq *query.QueryJSON) (listeners []*pldapi.PrivacyGroupMessageListener, err error) {
	err = r.c.CallRPC(ctx, &listeners, "pgroup_queryMessageListeners", jq)
	return
}

func (r *pgroup) GetMessageListener(ctx context.Context, listenerName string) (listener *pldapi.PrivacyGroupMessageListener, err error) {
	err = r.c.CallRPC(ctx, &listener, "pgroup_getMessageListener", listenerName)
	return
}

func (r *pgroup) StartMessageListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = r.c.CallRPC(ctx, &success, "pgroup_startMessageListener", listenerName)
	return
}

func (r *pgroup) StopMessageListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = r.c.CallRPC(ctx, &success, "pgroup_stopMessageListener", listenerName)
	return
}

func (r *pgroup) DeleteMessageListener(ctx context.Context, listenerName string) (success bool, err error) {
	err = r.c.CallRPC(ctx, &success, "pgroup_deleteMessageListener", listenerName)
	return
}

func (r *pgroup) SubscribeMessages(ctx context.Context, listenerName string) (sub rpcclient.Subscription, err error) {
	ws, err := r.c.WSClient(ctx)
	if err != nil {
		return nil, err
	}
	return ws.Subscribe(ctx, pgroupReceiptsSubscriptionConfig, "messages", listenerName)
}
