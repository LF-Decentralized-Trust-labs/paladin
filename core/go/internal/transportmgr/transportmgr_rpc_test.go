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
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRPCLocalDetails(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false)
	defer done()

	client, rpcDone := newTestRPCServer(t, ctx, tm)
	defer rpcDone()

	transportRPC := pldclient.Wrap(client).Transport()

	nodeName, rpcErr := transportRPC.NodeName(ctx)
	require.NoError(t, rpcErr)
	assert.Equal(t, "node1", nodeName)

	localTransports, rpcErr := transportRPC.LocalTransports(ctx)
	require.NoError(t, rpcErr)
	assert.Equal(t, []string{tp.t.name}, localTransports)

	tp.Functions.GetLocalDetails = func(ctx context.Context, gldr *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error) {
		return &prototk.GetLocalDetailsResponse{
			TransportDetails: "some details",
		}, nil
	}

	localTransportDetails, rpcErr := transportRPC.LocalTransportDetails(ctx, localTransports[0])
	require.NoError(t, rpcErr)
	assert.Equal(t, "some details", localTransportDetails)

	_, err := tm.getPeer(ctx, "node2", false)
	require.NoError(t, err)

	peers, rpcErr := transportRPC.Peers(ctx)
	require.NoError(t, rpcErr)
	require.Len(t, peers, 1)
	require.Equal(t, "node2", peers[0].Name)

	peer, rpcErr := transportRPC.PeerInfo(ctx, "node2")
	require.NoError(t, rpcErr)
	require.Equal(t, "node2", peer.Name)
	peer, rpcErr = transportRPC.PeerInfo(ctx, "node3")
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

func TestRPCReliableMessages(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, true, mockGoodTransport)
	defer done()

	tp.Functions.ActivatePeer = func(ctx context.Context, apr *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
		return &prototk.ActivatePeerResponse{}, nil
	}

	client, rpcDone := newTestRPCServer(t, ctx, tm)
	defer rpcDone()

	var msgID uuid.UUID
	err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		msg := &pldapi.ReliableMessage{
			MessageType: pldapi.RMTPrivacyGroup.Enum(),
			Node:        "node2",
			Metadata:    pldtypes.RawJSON(`{}`),
		}
		err := tm.SendReliable(ctx, dbTX, msg)
		msgID = msg.ID
		return err
	})
	require.NoError(t, err)

	transportRPC := pldclient.Wrap(client).Transport()

	// Wait for the message to get nack'd
	for {
		rmsgs, err := transportRPC.QueryReliableMessages(ctx, query.NewQueryBuilder().Equal("node", "node2").Limit(100).Query())
		require.NoError(t, err)
		if len(rmsgs) > 0 {
			require.Len(t, rmsgs, 1)
			require.Equal(t, msgID, rmsgs[0].ID)
			if rmsgs[0].Ack == nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			require.Regexp(t, "PD012016", rmsgs[0].Ack.Error)
			break
		}
	}

	// Get the ack directly
	acks, err := transportRPC.QueryReliableMessageAcks(ctx, query.NewQueryBuilder().Equal("messageId", msgID).Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, acks, 1)
	require.Regexp(t, "PD012016", acks[0].Error)

}
