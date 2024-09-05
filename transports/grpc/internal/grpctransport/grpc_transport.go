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
	connecting bool
	connError  error
	stream     grpc.ClientStreamingClient[proto.Message, proto.Empty]
}

func NewPlugin(ctx context.Context) plugintk.PluginBase {
	return plugintk.NewTransport(func(callbacks plugintk.TransportCallbacks) plugintk.TransportAPI {
		return newGRPCTransport(ctx, callbacks)
	})
}

func newGRPCTransport(ctx context.Context, callbacks plugintk.TransportCallbacks) *grpcTransport {
	return &grpcTransport{
		bgCtx:               ctx,
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

	t.conf.TLS.Enabled = true
	baseTLSConfig, err := tlsconf.BuildTLSConfig(ctx, &t.conf.TLS, tlsconf.ServerType)
	if err != nil {
		return nil, err
	}

	directCertVerification := confutil.Bool(t.conf.DirectCertVerification, *ConfigDefaults.DirectCertVerification)
	baseTLSConfig.InsecureSkipVerify = false
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

// // Override the accept on the listener to wrap in our connection
// func (gcl *grpcConnectionListener) Accept() (net.Conn, error) {
// 	gc := &grpcConnection{
// 		t:         gcl.t,
// 		gcl:       gcl,
// 		direction: "inbound",
// 	}
// 	c, err := gcl.Listener.Accept()
// 	if err == nil {
// 		c, err = gc.tlsInit(c)
// 	}
// 	return c, err
// }

func (t *grpcTransport) serve() {
	defer close(t.serverDone)

	log.L(t.bgCtx).Infof("gRPC server for plugin %s starting on %s", t.name, t.listener.Addr())
	err := t.grpcServer.Serve(t.listener)
	log.L(t.bgCtx).Infof("gRPC server for plugin %s stopped (err=%v)", t.name, err)
}

func (t *grpcTransport) ConnectSendStream(stream grpc.ClientStreamingServer[proto.Message, proto.Empty]) error {
	// ctx := stream.Context()
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		panic(msg)
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

func (t *grpcTransport) getConnection(ctx context.Context, nodeName string) (grpc.ClientStreamingClient[proto.Message, proto.Empty], error) {
	t.connLock.L.Lock()
	existing := t.outboundConnections[nodeName]
	if existing != nil {
		// Multiple routines might try to connect concurrently, so we have a condition
		defer t.connLock.L.Unlock() // unlock on return on this path
		for existing.connecting {
			t.connLock.Wait()
		}
		return existing.stream, existing.connError
	}
	// We need to create the connection - put the placeholder in the map
	newConn := &outboundConn{connecting: true}
	t.outboundConnections[nodeName] = newConn
	t.connLock.L.Unlock()

	// We must ensure we complete the newConn (for good or bad)
	// and notify everyone waiting to check status before we return
	var err error
	defer func() {
		t.connLock.L.Lock()
		newConn.connecting = false
		if err != nil {
			// copy our error to anyone queuing - everybody fails
			newConn.connError = err
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
	conn, err := grpc.NewClient(transportDetails.Endpoint,
		grpc.WithTransportCredentials(t.peerVerifier.Clone()))
	if err != nil {
		return nil, err
	}
	client := proto.NewPaladinGRPCTransportClient(conn)
	newConn.stream, err = client.ConnectSendStream(ctx)
	return newConn.stream, err
}

func (t *grpcTransport) SendMessage(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
	node, err := tktypes.PrivateIdentityLocator(req.Message.Destination).Node(ctx, false)
	if err != nil {
		return nil, err
	}
	stream, err := t.getConnection(ctx, node)
	if err == nil {
		err = stream.Send(&proto.Message{
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
