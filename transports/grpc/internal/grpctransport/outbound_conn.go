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

package grpctransport

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/transports/grpc/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/transports/grpc/pkg/proto"
	"google.golang.org/grpc"
)

type outboundConn struct {
	t        *grpcTransport
	nodeName string
	client   proto.PaladinGRPCTransportClient
	peerInfo PeerInfo
	sendLock sync.Mutex
	stream   grpc.ClientStreamingClient[proto.Message, proto.Empty]
}

func (t *grpcTransport) newConnection(ctx context.Context, nodeName string, transportDetailsJSON string) (oc *outboundConn, peerInfoJSON []byte, err error) {

	// Parse the connection details
	var transportDetails PublishedTransportDetails
	err = json.Unmarshal([]byte(transportDetailsJSON), &transportDetails)
	if err == nil {
		oc = &outboundConn{
			t:        t,
			nodeName: nodeName,
			peerInfo: PeerInfo{
				Endpoint: transportDetails.Endpoint,
			},
		}
		peerInfoJSON, err = json.Marshal(&oc.peerInfo)
	}
	if err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgInvalidTransportDetails, nodeName)
	}

	// Create the gRPC connection (it's not actually connected until we use it)
	individualNodeVerifier := oc.t.peerVerifier.Clone().(*tlsVerifier)
	individualNodeVerifier.expectedNode = oc.nodeName
	grpcConn, err := grpc.NewClient(transportDetails.Endpoint,
		grpc.WithTransportCredentials(individualNodeVerifier),
	)
	if err == nil {
		oc.client = proto.NewPaladinGRPCTransportClient(grpcConn)
		err = oc.ensureStream()
	}
	if err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgConnectionFailed, transportDetails.Endpoint)
	}

	return oc, peerInfoJSON, nil
}

func (oc *outboundConn) close(ctx context.Context) {
	oc.sendLock.Lock()
	defer oc.sendLock.Unlock()

	log.L(ctx).Errorf("cleaning up connection to %s", oc.nodeName)

	if oc.stream != nil {
		_ = oc.stream.CloseSend()
		oc.stream = nil
	}
}

func (oc *outboundConn) ensureStream() (err error) {
	if oc.stream != nil {
		return nil
	}
	log.L(oc.t.bgCtx).Infof("GRPC establishing new stream to peer %s (endpoint=%s)", oc.nodeName, oc.peerInfo.Endpoint)
	oc.stream, err = oc.client.ConnectSendStream(oc.t.bgCtx)
	return err
}

func (oc *outboundConn) send(message *proto.Message) error {
	oc.sendLock.Lock()
	defer oc.sendLock.Unlock()

	err := oc.ensureStream()

	if err == nil {
		err = oc.stream.Send(message)
	}

	if err != nil {
		// Clean up the stream - we'll create a new one on next send
		_ = oc.stream.CloseSend()
		oc.stream = nil
	}
	return err
}
