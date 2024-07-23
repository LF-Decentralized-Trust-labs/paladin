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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

func (ss *stateStore) RPCModule() *rpcserver.RPCModule {
	return ss.rpcModule
}

func (ss *stateStore) initRPC() {
	ss.rpcModule = rpcserver.NewRPCModule("pstate").
		Add("pstate_storeABISchema", ss.rpcStoreABISchema()).
		Add("pstate_listSchemas", ss.rpcListSchema()).
		Add("pstate_storeState", ss.rpcStoreState()).
		Add("pstate_queryStates", ss.rpcQuery())
}

func (ss *stateStore) rpcStoreABISchema() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		domain string,
		abiParam abi.Parameter,
	) (SchemaCommon, error) {
		s, err := newABISchema(ctx, domain, &abiParam)
		if err == nil {
			err = ss.PersistSchema(ctx, s)
		}
		return s, err
	})
}

func (ss *stateStore) rpcListSchema() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		domain string,
	) ([]SchemaCommon, error) {
		return ss.ListSchemas(ctx, domain)
	})
}

func (ss *stateStore) rpcStoreState() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		domain string,
		schema string,
		value types.RawJSON,
	) (*State, error) {
		var state *State
		newState, err := ss.PersistState(ctx, domain, schema, value)
		if err == nil {
			state = newState.State
		}
		return state, err
	})
}

func (ss *stateStore) rpcQuery() rpcserver.RPCHandler {
	return rpcserver.RPCMethod4(func(ctx context.Context,
		domain string,
		schema string,
		query filters.QueryJSON,
		status StateStatusQualifier,
	) ([]*State, error) {
		return ss.FindStates(ctx, domain, schema, &query, status)
	})
}
