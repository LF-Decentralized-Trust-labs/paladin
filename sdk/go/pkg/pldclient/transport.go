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
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
)

type Transport interface {
	RPCModule

	NodeName(ctx context.Context) (nodeName string, err error)
	LocalTransports(ctx context.Context) (transportNames []string, err error)
	LocalTransportDetails(ctx context.Context, transportName string) (transportDetailsStr string, err error)
	Peers(ctx context.Context) (peers []*pldapi.PeerInfo, err error)
	PeerInfo(ctx context.Context, nodeName string) (peer *pldapi.PeerInfo, err error)
	QueryReliableMessages(ctx context.Context, query *query.QueryJSON) (reliableMessages []*pldapi.ReliableMessage, err error)
	QueryReliableMessageAcks(ctx context.Context, query *query.QueryJSON) (reliableMessageAcks []*pldapi.ReliableMessageAck, err error)
}

// This is necessary because there's no way to introspect function parameter names via reflection
var transportInfo = &rpcModuleInfo{
	group: "transport",
	methodInfo: map[string]RPCMethodInfo{
		"transport_nodeName": {
			Inputs: []string{},
			Output: "nodeName",
		},
		"transport_localTransports": {
			Inputs: []string{},
			Output: "transportNames",
		},
		"transport_localTransportDetails": {
			Inputs: []string{"transportName"},
			Output: "transportDetailsStr",
		},
		"transport_peers": {
			Inputs: []string{},
			Output: "peers",
		},
		"transport_peerInfo": {
			Inputs: []string{"nodeName"},
			Output: "peer",
		},
		"transport_queryReliableMessages": {
			Inputs: []string{"query"},
			Output: "reliableMessages",
		},
		"transport_queryReliableMessageAcks": {
			Inputs: []string{"query"},
			Output: "reliableMessageAcks",
		},
	},
}

var _ Transport = &transport{}

type transport struct {
	*rpcModuleInfo
	c *paladinClient
}

func (c *paladinClient) Transport() Transport {
	return &transport{rpcModuleInfo: transportInfo, c: c}
}

func (t *transport) NodeName(ctx context.Context) (name string, err error) {
	err = t.c.CallRPC(ctx, &name, "transport_nodeName")
	return
}

func (t *transport) LocalTransports(ctx context.Context) (transportNames []string, err error) {
	err = t.c.CallRPC(ctx, &transportNames, "transport_localTransports")
	return
}

func (t *transport) LocalTransportDetails(ctx context.Context, transportName string) (transportDetailsStr string, err error) {
	err = t.c.CallRPC(ctx, &transportDetailsStr, "transport_localTransportDetails", transportName)
	return
}

func (t *transport) Peers(ctx context.Context) (peers []*pldapi.PeerInfo, err error) {
	err = t.c.CallRPC(ctx, &peers, "transport_peers")
	return
}

func (t *transport) PeerInfo(ctx context.Context, nodeName string) (peer *pldapi.PeerInfo, err error) {
	err = t.c.CallRPC(ctx, &peer, "transport_peerInfo", nodeName)
	return
}

func (t *transport) QueryReliableMessages(ctx context.Context, query *query.QueryJSON) (reliableMessages []*pldapi.ReliableMessage, err error) {
	err = t.c.CallRPC(ctx, &reliableMessages, "transport_queryReliableMessages", query)
	return
}

func (t *transport) QueryReliableMessageAcks(ctx context.Context, query *query.QueryJSON) (reliableMessageAcks []*pldapi.ReliableMessageAck, err error) {
	err = t.c.CallRPC(ctx, &reliableMessageAcks, "transport_queryReliableMessageAcks", query)
	return
}
