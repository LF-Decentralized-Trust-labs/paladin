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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (gm *groupManager) RPCModule() *rpcserver.RPCModule {
	return gm.rpcModule
}

func (gm *groupManager) initRPC() {
	gm.rpcModule = rpcserver.NewRPCModule("pgroup").
		Add("pgroup_createGroup", gm.rpcCreateGroup()).
		Add("pgroup_getGroupById", gm.rpcGetGroupByID()).
		Add("pgroup_queryGroups", gm.rpcQueryGroups()).
		Add("pgroup_queryGroupsByProperties", gm.rpcQueryGroupsByProperties()).
		Add("pgroup_sendTransaction", gm.rpcSendTransaction()).
		Add("pgroup_call", gm.rpcCall()).
		Add("pgroup_sendMessage", gm.rpcSendMessage()).
		Add("pgroup_getMessageById", gm.rpcGetMessageByID()).
		Add("pgroup_queryMessages", gm.rpcQueryMessages()).
		AddAsync(gm.rpcEventStreams)
}

func (gm *groupManager) rpcCreateGroup() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, spec pldapi.PrivacyGroupInput) (id tktypes.HexBytes, err error) {
		err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			id, err = gm.CreateGroup(ctx, dbTX, &spec)
			return err
		})
		return id, err
	})
}

func (gm *groupManager) rpcGetGroupByID() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context, domainName string, id tktypes.HexBytes) (*pldapi.PrivacyGroupWithABI, error) {
		return gm.GetGroupByID(ctx, gm.p.NOTX(), domainName, id)
	})
}

func (gm *groupManager) rpcQueryGroups() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, jq query.QueryJSON) ([]*pldapi.PrivacyGroup, error) {
		return gm.QueryGroups(ctx, gm.p.NOTX(), &jq)
	})
}

func (gm *groupManager) rpcQueryGroupsByProperties() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context, domainName string, schemaID tktypes.Bytes32, jq query.QueryJSON) ([]*pldapi.PrivacyGroup, error) {
		return gm.QueryGroupsByProperties(ctx, gm.p.NOTX(), domainName, schemaID, &jq)
	})
}

func (gm *groupManager) rpcSendTransaction() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, tx *pldapi.PrivacyGroupTransactionInput) (txID *uuid.UUID, err error) {
		err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			txID, err = gm.SendTransaction(ctx, dbTX, tx)
			return err
		})
		return txID, err
	})
}

func (gm *groupManager) rpcCall() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, call *pldapi.PrivacyGroupTransactionCall) (result tktypes.RawJSON, err error) {
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
