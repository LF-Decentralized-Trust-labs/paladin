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

package transportmgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRPCLocalDetails(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false)
	defer done()

	rpc, rpcDone := newTestRPCServer(t, ctx, tm)
	defer rpcDone()

	var nodeName string
	rpcErr := rpc.CallRPC(ctx, &nodeName, "transport_nodeName")
	require.NoError(t, rpcErr)
	assert.Equal(t, "node1", nodeName)

	var localTransports []string
	rpcErr = rpc.CallRPC(ctx, &localTransports, "transport_localTransports")
	require.NoError(t, rpcErr)
	assert.Equal(t, []string{tp.t.name}, localTransports)

	tp.Functions.GetLocalDetails = func(ctx context.Context, gldr *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error) {
		return &prototk.GetLocalDetailsResponse{
			TransportDetails: "some details",
		}, nil
	}

	var localTransportDetails string
	rpcErr = rpc.CallRPC(ctx, &localTransportDetails, "transport_localTransportDetails", localTransports[0])
	require.NoError(t, rpcErr)
	assert.Equal(t, "some details", localTransportDetails)

	_, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	var peers []*pldapi.PeerInfo
	rpcErr = rpc.CallRPC(ctx, &peers, "transport_peers")
	require.NoError(t, rpcErr)
	require.Len(t, peers, 1)
	require.Equal(t, "node2", peers[0].Name)

	var peer *pldapi.PeerInfo
	rpcErr = rpc.CallRPC(ctx, &peer, "transport_peerInfo", "node2")
	require.NoError(t, rpcErr)
	require.Equal(t, "node2", peer.Name)
	rpcErr = rpc.CallRPC(ctx, &peer, "transport_peerInfo", "node3")
	require.NoError(t, rpcErr)
	require.Nil(t, peer)

}

func newTestRPCServer(t *testing.T, ctx context.Context, tm *transportManager) (rpcclient.Client, func()) {

	s, err := rpcserver.NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)

	s.Register(tm.RPCModule())

	c := rpcclient.WrapRestyClient(resty.New().SetBaseURL(fmt.Sprintf("http://%s", s.HTTPAddr())))

	return c, s.Stop

}
