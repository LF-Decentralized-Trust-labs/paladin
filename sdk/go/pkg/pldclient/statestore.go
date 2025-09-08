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
)

type StateStore interface {
	RPCModule

	ListSchemas(ctx context.Context, domain string) (schemas []*pldapi.Schema, err error)
	StoreState(ctx context.Context, domain string, contractAddress pldtypes.EthAddress, schemaRef pldtypes.Bytes32, data pldtypes.RawJSON) (state *pldapi.State, err error)
	QueryStates(ctx context.Context, domain string, schemaRef pldtypes.Bytes32, query *query.QueryJSON, qualifier pldapi.StateStatusQualifier) (states []*pldapi.State, err error)
	QueryContractStates(ctx context.Context, domain string, contractAddress pldtypes.EthAddress, schemaRef pldtypes.Bytes32, query *query.QueryJSON, qualifier pldapi.StateStatusQualifier) (states []*pldapi.State, err error)
	QueryNullifiers(ctx context.Context, domain string, schemaRef pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (states []*pldapi.State, err error)
	QueryContractNullifiers(ctx context.Context, domain string, contractAddress pldtypes.EthAddress, schemaRef pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (states []*pldapi.State, err error)
}

// This is necessary because there's no way to introspect function parameter names via reflection
var stateStoreInfo = &rpcModuleInfo{
	group: "pstate",
	methodInfo: map[string]RPCMethodInfo{
		"pstate_listSchemas": {
			Inputs: []string{"domain"},
			Output: "schemas",
		},
		"pstate_storeState": {
			Inputs: []string{"domain", "contractAddress", "schemaRef", "data"},
			Output: "state",
		},
		"pstate_queryStates": {
			Inputs: []string{"domain", "schemaRef", "query", "qualifier"},
			Output: "states",
		},
		"pstate_queryContractStates": {
			Inputs: []string{"domain", "contractAddress", "schemaRef", "query", "qualifier"},
			Output: "states",
		},
		"pstate_queryNullifiers": {
			Inputs: []string{"domain", "schemaRef", "query", "qualifier"},
			Output: "states",
		},
		"pstate_queryContractNullifiers": {
			Inputs: []string{"domain", "contractAddress", "schemaRef", "query", "qualifier"},
			Output: "states",
		},
	},
}

type stateStore struct {
	*rpcModuleInfo
	c *paladinClient
}

func (c *paladinClient) StateStore() StateStore {
	return &stateStore{rpcModuleInfo: stateStoreInfo, c: c}
}

func (r *stateStore) ListSchemas(ctx context.Context, domain string) (schemas []*pldapi.Schema, err error) {
	err = r.c.CallRPC(ctx, &schemas, "pstate_listSchemas", domain)
	return
}

func (r *stateStore) StoreState(ctx context.Context, domain string, contractAddress pldtypes.EthAddress, schemaRef pldtypes.Bytes32, data pldtypes.RawJSON) (state *pldapi.State, err error) {
	err = r.c.CallRPC(ctx, &state, "pstate_storeState", domain, contractAddress, schemaRef, data)
	return
}

func (r *stateStore) QueryStates(ctx context.Context, domain string, schemaRef pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (states []*pldapi.State, err error) {
	err = r.c.CallRPC(ctx, &states, "pstate_queryStates", domain, schemaRef, query)
	return
}

func (r *stateStore) QueryContractStates(ctx context.Context, domain string, contractAddress pldtypes.EthAddress, schemaRef pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (states []*pldapi.State, err error) {
	err = r.c.CallRPC(ctx, &states, "pstate_queryContractStates", domain, contractAddress, schemaRef, query)
	return
}

func (r *stateStore) QueryNullifiers(ctx context.Context, domain string, schemaRef pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (states []*pldapi.State, err error) {
	err = r.c.CallRPC(ctx, &states, "pstate_queryNullifiers", domain, schemaRef, query)
	return
}

func (r *stateStore) QueryContractNullifiers(ctx context.Context, domain string, contractAddress pldtypes.EthAddress, schemaRef pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (states []*pldapi.State, err error) {
	err = r.c.CallRPC(ctx, &states, "pstate_queryContractNullifiers", domain, contractAddress, schemaRef, query)
	return
}
