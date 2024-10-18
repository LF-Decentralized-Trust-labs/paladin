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

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (ss *stateManager) RPCModule() *rpcserver.RPCModule {
	return ss.rpcModule
}

func (ss *stateManager) initRPC() {
	ss.rpcModule = rpcserver.NewRPCModule("pstate").
		Add("pstate_listSchemas", ss.rpcListSchema()).
		Add("pstate_storeState", ss.rpcStoreState()).
		Add("pstate_queryStates", ss.rpcQuery())
}

func (ss *stateManager) rpcListSchema() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		domain string,
	) ([]components.Schema, error) {
		return ss.ListSchemas(ctx, ss.p.DB(), domain)
	})
}

func (ss *stateManager) rpcStoreState() rpcserver.RPCHandler {
	return rpcserver.RPCMethod4(func(ctx context.Context,
		domain string,
		contractAddress tktypes.EthAddress,
		schema tktypes.Bytes32,
		data tktypes.RawJSON,
	) (*components.State, error) {
		var state *components.State
		newStates, err := ss.WriteReceivedStates(ctx, ss.p.DB(), domain, []*components.StateUpsertOutsideContext{
			{
				ContractAddress: contractAddress,
				SchemaID:        schema,
				Data:            data,
			},
		})
		if err == nil {
			state = newStates[0]
		}
		return state, err
	})
}

func (ss *stateManager) rpcQuery() rpcserver.RPCHandler {
	return rpcserver.RPCMethod5(func(ctx context.Context,
		domain string,
		contractAddress tktypes.EthAddress,
		schema tktypes.Bytes32,
		query query.QueryJSON,
		status StateStatusQualifier,
	) ([]*components.State, error) {
		return ss.FindStates(ctx, ss.p.DB(), domain, contractAddress, schema, &query, status)
	})
}
