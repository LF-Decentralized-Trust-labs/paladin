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

package transportmgr

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

func (tm *transportManager) RPCModule() *rpcserver.RPCModule {
	return tm.rpcModule
}

func (tm *transportManager) initRPC() {
	tm.rpcModule = rpcserver.NewRPCModule("transport").
		Add("transport_nodeName", tm.rpcNodeName()).
		Add("transport_localTransports", tm.rpcLocalTransports()).
		Add("transport_localTransportDetails", tm.rpcLocalTransportDetails()).
		Add("transport_peers", tm.rpcPeers()).
		Add("transport_peerInfo", tm.rpcPeerInfo()).
		Add("transport_queryReliableMessages", tm.rpcQueryReliableMessages()).
		Add("transport_queryReliableMessageAcks", tm.rpcQueryReliableMessageAcks())
}

func (tm *transportManager) rpcNodeName() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context,
	) (string, error) {
		return tm.localNodeName, nil
	})
}

func (tm *transportManager) rpcLocalTransports() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context,
	) ([]string, error) {
		return tm.getTransportNames(), nil
	})
}

func (tm *transportManager) rpcLocalTransportDetails() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		transportName string,
	) (string, error) {
		return tm.getLocalTransportDetails(ctx, transportName)
	})
}

func (tm *transportManager) rpcPeers() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context) ([]*pldapi.PeerInfo, error) {
		return tm.listActivePeerInfo(), nil
	})
}

func (tm *transportManager) rpcPeerInfo() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, nodeName string) (*pldapi.PeerInfo, error) {
		return tm.getPeerInfo(nodeName), nil
	})
}

func (tm *transportManager) rpcQueryReliableMessages() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, jq query.QueryJSON) ([]*pldapi.ReliableMessage, error) {
		return tm.QueryReliableMessages(ctx, tm.persistence.NOTX(), &jq)
	})
}

func (tm *transportManager) rpcQueryReliableMessageAcks() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context, jq query.QueryJSON) ([]*pldapi.ReliableMessageAck, error) {
		return tm.QueryReliableMessageAcks(ctx, tm.persistence.NOTX(), &jq)
	})
}
