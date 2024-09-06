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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/tlsconf"
	"github.com/kaleido-io/paladin/transports/grpc/internal/msgs"
	"github.com/kaleido-io/paladin/transports/grpc/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

type Server interface {
	Start() error
	Stop()
}

type grpcTransport struct {
	proto.UnimplementedPaladinGRPCTransportServer

	bgCtx     context.Context
	callbacks plugintk.TransportCallbacks

	name         string
	listener     net.Listener
	grpcServer   *grpc.Server
	serverDone   chan struct{}
	peerVerifier *tlsVerifier

	conf                Config
	connLock            sync.Cond
	outboundConnections map[string]*outboundConn
}

type outboundConn struct {
	nodeName   string
	connecting bool
	sendLock   sync.Mutex
	waiting    int
	connError  error
	stream     grpc.ClientStreamingClient[proto.Message, proto.Empty]
}

func NewPlugin(ctx context.Context) plugintk.PluginBase {
	return plugintk.NewTransport(grpcTransportFactory)
}

func grpcTransportFactory(callbacks plugintk.TransportCallbacks) plugintk.TransportAPI {
	return &grpcTransport{
		bgCtx:               context.Background(),
		callbacks:           callbacks,
		connLock:            *sync.NewCond(new(sync.Mutex)),
		outboundConnections: make(map[string]*outboundConn),
	}
}

func (t *grpcTransport) ConfigureTransport(ctx context.Context, req *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
	// Hold the connlock while setting our state (as we'll read it when creating new conns)
	t.connLock.L.Lock()
	defer t.connLock.L.Unlock()

	t.name = req.Name

	err := json.Unmarshal([]byte(req.ConfigJson), &t.conf)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidTransportConfig)
	}

	listenAddr := confutil.StringOrEmpty(t.conf.Address, "")
	if t.conf.Port == nil || listenAddr == "" {
		return nil, i18n.NewError(ctx, msgs.MsgListenerPortAndAddressRequired)
	}
	listenAddr = fmt.Sprintf("%s:%d", listenAddr, *t.conf.Port)

	var subjectMatchRegex *regexp.Regexp
	certSubjectMatcher := confutil.StringOrEmpty(t.conf.CertSubjectMatcher, "")
	if certSubjectMatcher != "" {
		if subjectMatchRegex, err = regexp.Compile(*t.conf.CertSubjectMatcher); err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidSubjectRegexp, *t.conf.CertSubjectMatcher)
		}
	}

	// We only support mutual-TLS in this transport (with direct trust of certificates via registry, or use of a CA)
	t.conf.TLS.Enabled = true
	t.conf.TLS.ClientAuth = true // Note if this is unset the ClientCAs will not be configured
	baseTLSConfig, err := tlsconf.BuildTLSConfig(ctx, &t.conf.TLS, tlsconf.ServerType)
	if err != nil {
		return nil, err
	}

	directCertVerification := confutil.Bool(t.conf.DirectCertVerification, *ConfigDefaults.DirectCertVerification)
	baseTLSConfig.ClientAuth = tls.RequireAndVerifyClientCert
	if directCertVerification {
		// Check the tls default settings haven't been set with conflicting config
		if t.conf.TLS.CAFile != "" || t.conf.TLS.CA != "" || t.conf.TLS.InsecureSkipHostVerify || len(t.conf.TLS.RequiredDNAttributes) > 0 {
			return nil, i18n.NewError(ctx, msgs.MsgConfIncompatibleWithDirectCertVerify)
		}
		// Set InsecureSkipVerify and RequireAnyClientCert to skip the default
		// validation we are replacing. This will not disable VerifyConnection.
		baseTLSConfig.InsecureSkipVerify = true
		baseTLSConfig.ClientAuth = tls.RequireAnyClientCert
	}

	t.listener, err = net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	t.peerVerifier = &tlsVerifier{
		tlsVerifierStatic: tlsVerifierStatic{
			t:                      t,
			directCertVerification: directCertVerification,
			subjectMatchRegex:      subjectMatchRegex,
		},
		baseTLSConfig: baseTLSConfig,
	}
	t.grpcServer = grpc.NewServer(grpc.Creds(t.peerVerifier))
	proto.RegisterPaladinGRPCTransportServer(t.grpcServer, t)

	// Kick off the gRPC listener
	if t.serverDone == nil {
		t.serverDone = make(chan struct{})
		go t.serve()
	}

	return &prototk.ConfigureTransportResponse{}, nil
}

func (t *grpcTransport) serve() {
	defer close(t.serverDone)

	log.L(t.bgCtx).Infof("gRPC server for plugin %s starting on %s", t.name, t.listener.Addr())
	err := t.grpcServer.Serve(t.listener)
	log.L(t.bgCtx).Infof("gRPC server for plugin %s stopped (err=%v)", t.name, err)
}

// The server side of a send-stream, which receives messages from the client and delivers them
// to our local Paladin server
func (t *grpcTransport) ConnectSendStream(stream grpc.ClientStreamingServer[proto.Message, proto.Empty]) error {

	// The TLS authentication will have done its job by this point, and we can pop it out of the context
	// where it is the AuthInfo() provider on the peer.
	ctx := stream.Context()
	var ai *tlsVerifierAuthInfo
	peer, ok := peer.FromContext(ctx)
	if ok && peer.AuthInfo != nil {
		ai, ok = peer.AuthInfo.(*tlsVerifierAuthInfo)
	}
	if !ok || ai == nil {
		return i18n.NewError(ctx, msgs.MsgAuthContextNotAvailable)
	}

	// Go into the long-lived receive loop until the client disconnects
	ctx = log.WithLogField(log.WithLogField(ctx, "remote", ai.remoteAddr), "node", ai.verifiedNodeName)
	log.L(ctx).Infof("GRPC message stream established from node %s (authType=%s)", ai.verifiedNodeName, peer.AuthInfo.AuthType())
	for {
		msg, err := stream.Recv()
		if err != nil {
			log.L(ctx).Infof("GRPC message stream from %s closing (err=%v)", ai.verifiedNodeName, err)
			return err
		}

		// Check the message is from the node we expect.
		// Note the destination node is checked by Paladin - just just have to verify the sender.
		replyToNode, err := tktypes.PrivateIdentityLocator(msg.ReplyTo).Node(ctx, false)
		if err == nil && replyToNode != ai.verifiedNodeName {
			err = i18n.NewError(ctx, msgs.MsgInvalidReplyToNode)
		}
		if err != nil {
			log.L(ctx).Errorf("Invalid replyTo (err=%s): %s", err, tktypes.ProtoToJSON(msg))
			return err
		}

		// Deliver it to Paladin
		_, err = t.callbacks.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
			Message: &prototk.Message{
				MessageId:     msg.MessageId,
				CorrelationId: msg.CorrelationId,
				Destination:   msg.Destination,
				ReplyTo:       msg.ReplyTo,
				MessageType:   msg.MessageType,
				Payload:       msg.Payload,
			},
		})
		if err != nil {
			log.L(ctx).Errorf("Receive failed (err=%s): %s", err, tktypes.ProtoToJSON(msg))
			return err
		}

	}
}

func (t *grpcTransport) getTransportDetails(ctx context.Context, node string) (transportDetails *PublishedTransportDetails, err error) {
	gtdr, err := t.callbacks.GetTransportDetails(ctx, &prototk.GetTransportDetailsRequest{
		Node: node,
	})
	if err != nil {
		log.L(ctx).Errorf("lookup failed for node %s: %s", node, err)
		return nil, err
	}

	// Parse the details
	if err = json.Unmarshal([]byte(gtdr.TransportDetails), &transportDetails); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgPeerTransportDetailsInvalid, node)
	}

	return transportDetails, nil
}

func (t *grpcTransport) waitExistingOrNewConn(nodeName string) (bool, *outboundConn, error) {
	t.connLock.L.Lock()
	defer t.connLock.L.Unlock()
	existing := t.outboundConnections[nodeName]
	if existing != nil {
		// Multiple routines might try to connect concurrently, so we have a condition
		existing.waiting++
		for existing.connecting {
			t.connLock.Wait()
		}
		return false, existing, existing.connError
	}
	// We need to create the connection - put the placeholder in the map
	newConn := &outboundConn{nodeName: nodeName, connecting: true}
	t.outboundConnections[nodeName] = newConn
	return true, newConn, nil
}

func (t *grpcTransport) send(ctx context.Context, oc *outboundConn, message *proto.Message) (err error) {
	oc.sendLock.Lock()
	defer func() {
		if err != nil {
			// Close this stream and remove it before dropping the lock (unsafe to call concurrent to send)
			log.L(ctx).Errorf("closing stream to %s due to send err: %s", oc.nodeName, err)
			_ = oc.stream.CloseSend()
			// Drop the send lock before taking conn lock to remove from the connections
			oc.sendLock.Unlock()
			t.connLock.L.Lock()
			defer t.connLock.L.Unlock()
			delete(t.outboundConnections, oc.nodeName)
		} else {
			// Just drop the lock and return
			oc.sendLock.Unlock()
		}
	}()
	err = oc.stream.Send(message)
	return
}

func (t *grpcTransport) getConnection(ctx context.Context, nodeName string) (*outboundConn, error) {

	isNew, oc, err := t.waitExistingOrNewConn(nodeName)
	if !isNew || err != nil {
		return oc, err
	}

	// We must ensure we complete the newConn (for good or bad)
	// and notify everyone waiting to check status before we return
	defer func() {
		t.connLock.L.Lock()
		oc.connecting = false
		if err != nil {
			// copy our error to anyone queuing - everybody fails
			oc.connError = err
			// remove this entry, so the next one will try again
			delete(t.outboundConnections, nodeName)
		}
		t.connLock.Broadcast()
		t.connLock.L.Unlock()
	}()

	// We need to get the connection details
	transportDetails, err := t.getTransportDetails(ctx, nodeName)
	if err != nil {
		return nil, err
	}

	// Ok - try connecting
	individualNodeVerifier := t.peerVerifier.Clone().(*tlsVerifier)
	individualNodeVerifier.expectedNode = nodeName
	conn, err := grpc.NewClient(transportDetails.Endpoint,
		grpc.WithTransportCredentials(individualNodeVerifier),
	)
	if err == nil {
		client := proto.NewPaladinGRPCTransportClient(conn)
		oc.stream, err = client.ConnectSendStream(ctx)
	}
	return oc, err
}

func (t *grpcTransport) SendMessage(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
	node, err := tktypes.PrivateIdentityLocator(req.Message.Destination).Node(ctx, false)
	if err != nil {
		return nil, err
	}
	oc, err := t.getConnection(ctx, node)
	if err == nil {
		err = t.send(ctx, oc, &proto.Message{
			MessageId:     req.Message.MessageId,
			CorrelationId: req.Message.CorrelationId,
			Destination:   req.Message.Destination,
			ReplyTo:       req.Message.ReplyTo,
			MessageType:   req.Message.MessageType,
			Payload:       req.Message.Payload,
		})
	}
	if err != nil {
		return nil, err
	}
	return &prototk.SendMessageResponse{}, nil
}
