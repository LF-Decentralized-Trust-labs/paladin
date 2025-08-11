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

package groupmgr

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/google/uuid"
)

func (gm *groupManager) RPCModule() *rpcserver.RPCModule {
	return gm.rpcModule
}

func (gm *groupManager) initRPC() {
	gm.rpcModule = rpcserver.NewRPCModule("pgroup").
		Add("pgroup_createGroup", gm.rpcCreateGroup()).
		Add("pgroup_getGroupById", gm.rpcGetGroupByID()).
		Add("pgroup_getGroupByAddress", gm.rpcGetGroupByAddress()).
		Add("pgroup_queryGroups", gm.rpcQueryGroups()).
		Add("pgroup_queryGroupsWithMember", gm.rpcQueryGroupsWithMember()).
		Add("pgroup_sendTransaction", gm.rpcSendTransaction()).
		Add("pgroup_call", gm.rpcCall()).
		Add("pgroup_createMessageListener", gm.rpcCreateMessageListener()).
		Add("pgroup_queryMessageListeners", gm.rpcQueryMessageListeners()).
		Add("pgroup_getMessageListener", gm.rpcGetMessageListener()).
		Add("pgroup_startMessageListener", gm.rpcStartMessageListener()).
		Add("pgroup_stopMessageListener", gm.rpcStopMessageListener()).
		Add("pgroup_deleteMessageListener", gm.rpcDeleteMessageListener()).
		Add("pgroup_sendMessage", gm.rpcSendMessage()).
		Add("pgroup_getMessageById", gm.rpcGetMessageByID()).
		Add("pgroup_queryMessages", gm.rpcQueryMessages()).
		AddAsync(gm.rpcEventStreams)
}

func (gm *groupManager) rpcCreateGroup() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, spec pldapi.PrivacyGroupInput) (group *pldapi.PrivacyGroup, err error) {
		err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			group, err = gm.CreateGroup(ctx, dbTX, &spec)
			return err
		})
		return group, err
	})
}

func (gm *groupManager) rpcGetGroupByID() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context, domainName string, id pldtypes.HexBytes) (*pldapi.PrivacyGroup, error) {
		return gm.GetGroupByID(ctx, gm.p.NOTX(), domainName, id)
	})
}

func (gm *groupManager) rpcGetGroupByAddress() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, addr pldtypes.EthAddress) (*pldapi.PrivacyGroup, error) {
		return gm.GetGroupByAddress(ctx, gm.p.NOTX(), &addr)
	})
}

func (gm *groupManager) rpcQueryGroups() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, jq query.QueryJSON) ([]*pldapi.PrivacyGroup, error) {
		return gm.QueryGroups(ctx, gm.p.NOTX(), &jq)
	})
}

func (gm *groupManager) rpcQueryGroupsWithMember() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context, member string, jq query.QueryJSON) ([]*pldapi.PrivacyGroup, error) {
		return gm.QueryGroupsWithMember(ctx, gm.p.NOTX(), member, &jq)
	})
}

func (gm *groupManager) rpcSendTransaction() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, tx *pldapi.PrivacyGroupEVMTXInput) (txID *uuid.UUID, err error) {
		err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			txID, err = gm.SendTransaction(ctx, dbTX, tx)
			return err
		})
		return txID, err
	})
}

func (gm *groupManager) rpcCall() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, call *pldapi.PrivacyGroupEVMCall) (result pldtypes.RawJSON, err error) {
		err = gm.Call(ctx, gm.p.NOTX(), &result, call)
		return result, err
	})
}

func (gm *groupManager) rpcSendMessage() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, msg *pldapi.PrivacyGroupMessageInput) (msgID *uuid.UUID, err error) {
		err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			msgID, err = gm.SendMessage(ctx, dbTX, msg)
			return err
		})
		return msgID, err
	})
}

func (gm *groupManager) rpcGetMessageByID() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, id uuid.UUID) (msg *pldapi.PrivacyGroupMessage, err error) {
		return gm.GetMessageByID(ctx, gm.p.NOTX(), id, false)
	})
}

func (gm *groupManager) rpcQueryMessages() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, jq query.QueryJSON) (msgs []*pldapi.PrivacyGroupMessage, err error) {
		return gm.QueryMessages(ctx, gm.p.NOTX(), &jq)
	})
}

func (gm *groupManager) rpcCreateMessageListener() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		listener *pldapi.PrivacyGroupMessageListener,
	) (bool, error) {
		err := gm.CreateMessageListener(ctx, listener)
		return err == nil, err
	})
}

func (gm *groupManager) rpcQueryMessageListeners() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		query query.QueryJSON,
	) ([]*pldapi.PrivacyGroupMessageListener, error) {
		return gm.QueryMessageListeners(ctx, gm.p.NOTX(), &query)
	})
}

func (gm *groupManager) rpcGetMessageListener() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		name string,
	) (*pldapi.PrivacyGroupMessageListener, error) {
		return gm.GetMessageListener(ctx, name), nil
	})
}

func (gm *groupManager) rpcStartMessageListener() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		name string,
	) (bool, error) {
		return true, gm.StartMessageListener(ctx, name)
	})
}

func (gm *groupManager) rpcStopMessageListener() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		name string,
	) (bool, error) {
		return true, gm.StopMessageListener(ctx, name)
	})
}

func (gm *groupManager) rpcDeleteMessageListener() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		name string,
	) (bool, error) {
		return true, gm.DeleteMessageListener(ctx, name)
	})
}
