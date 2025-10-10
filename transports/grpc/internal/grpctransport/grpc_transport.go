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
	"encoding/pem"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/tlsconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/transports/grpc/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/transports/grpc/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/encoding/protojson"
)

type Server interface {
	Start() error
	Stop()
}

type grpcTransport struct {
	proto.UnimplementedPaladinGRPCTransportServer

	bgCtx     context.Context
	callbacks plugintk.TransportCallbacks

	name             string
	listener         net.Listener
	grpcServer       *grpc.Server
	serverDone       chan struct{}
	peerVerifier     *tlsVerifier
	externalHostname string
	localCertificate *tls.Certificate

	conf                Config
	connLock            sync.RWMutex
	outboundConnections map[string]*outboundConn
}

func NewPlugin(ctx context.Context) plugintk.PluginBase {
	return plugintk.NewTransport(NewGRPCTransport)
}

func NewGRPCTransport(callbacks plugintk.TransportCallbacks) plugintk.TransportAPI {
	return &grpcTransport{
		bgCtx:               context.Background(),
		callbacks:           callbacks,
		outboundConnections: make(map[string]*outboundConn),
	}
}

func (t *grpcTransport) ConfigureTransport(ctx context.Context, req *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
	// Hold the connlock while setting our state (as we'll read it when creating new conns)
	t.connLock.Lock()
	defer t.connLock.Unlock()

	t.name = req.Name

	err := json.Unmarshal([]byte(req.ConfigJson), &t.conf)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidTransportConfig)
	}

	listenAddrNoPort := confutil.StringOrEmpty(t.conf.Address, "")
	if t.conf.Port == nil || listenAddrNoPort == "" {
		return nil, i18n.NewError(ctx, msgs.MsgListenerPortAndAddressRequired)
	}
	listenAddr := fmt.Sprintf("%s:%d", listenAddrNoPort, *t.conf.Port)

	t.externalHostname = confutil.StringNotEmpty(t.conf.ExternalHostname, listenAddrNoPort)

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
	tlsDetail, err := tlsconf.BuildTLSConfigExt(ctx, &t.conf.TLS, tlsconf.ServerType)
	if err != nil {
		return nil, err
	}
	baseTLSConfig := tlsDetail.TLSConfig
	t.localCertificate = tlsDetail.Certificate

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

		log.L(ctx).Infof("GRPC received message id=%s cid=%v component=%d messageType=%s from peer %s",
			msg.MessageId, msg.CorrelationId, msg.Component, msg.MessageType, ai.verifiedNodeName)

		// Deliver it to Paladin
		_, err = t.callbacks.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
			FromNode: ai.verifiedNodeName,
			Message: &prototk.PaladinMsg{
				MessageId:     msg.MessageId,
				CorrelationId: msg.CorrelationId,
				Component:     prototk.PaladinMsg_Component(msg.Component),
				MessageType:   msg.MessageType,
				Payload:       msg.Payload,
			},
		})
		if err != nil {
			msgBytes, _ := protojson.Marshal(msg)
			log.L(ctx).Errorf("Receive failed (err=%s): %s", err, msgBytes)
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

func (t *grpcTransport) ActivatePeer(ctx context.Context, req *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
	t.connLock.Lock()
	defer t.connLock.Unlock()

	existing := t.outboundConnections[req.NodeName]
	if existing != nil {
		// Replace an existing connection - unexpected as Paladin shouldn't do this
		log.L(ctx).Warnf("replacing existing activation for node '%s'", req.NodeName)
		existing.close(ctx)
		delete(t.outboundConnections, req.NodeName)
	}
	oc, peerInfoJSON, err := t.newConnection(ctx, req.NodeName, req.TransportDetails)
	if err != nil {
		return nil, err
	}
	t.outboundConnections[req.NodeName] = oc
	return &prototk.ActivatePeerResponse{
		PeerInfoJson: string(peerInfoJSON),
	}, nil
}

func (t *grpcTransport) DeactivatePeer(ctx context.Context, req *prototk.DeactivatePeerRequest) (*prototk.DeactivatePeerResponse, error) {
	t.connLock.Lock()
	defer t.connLock.Unlock()

	existing := t.outboundConnections[req.NodeName]
	if existing != nil {
		// Replace an existing connection - unexpected as Paladin shouldn't do this
		log.L(ctx).Warnf("replacing existing activation for node '%s'", req.NodeName)
		existing.close(ctx)
		delete(t.outboundConnections, req.NodeName)
	}

	return &prototk.DeactivatePeerResponse{}, nil
}

func (t *grpcTransport) getConnection(nodeName string) *outboundConn {
	t.connLock.RLock()
	defer t.connLock.RUnlock()

	return t.outboundConnections[nodeName]
}

func (t *grpcTransport) SendMessage(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
	msg := req.Message
	oc := t.getConnection(req.Node)
	if oc == nil {
		// This is an error in the Paladin layer
		return nil, i18n.NewError(ctx, msgs.MsgNodeNotActive, req.Node)
	}
	log.L(ctx).Infof("GRPC sending message id=%s cid=%v component=%s messageType=%s to peer %s",
		msg.MessageId, msg.CorrelationId, msg.Component, msg.MessageType, req.Node)
	err := oc.send(&proto.Message{
		MessageId:     msg.MessageId,
		CorrelationId: msg.CorrelationId,
		Component:     int32(msg.Component),
		MessageType:   msg.MessageType,
		Payload:       msg.Payload,
	})
	if err != nil {
		return nil, err
	}
	return &prototk.SendMessageResponse{}, nil
}

func (t *grpcTransport) GetLocalDetails(ctx context.Context, req *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error) {

	certList := t.localCertificate.Certificate
	issuersText := new(strings.Builder)
	for _, cert := range certList {
		_ = pem.Encode(issuersText, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert,
		})
	}

	localDetails := &PublishedTransportDetails{
		Endpoint: fmt.Sprintf("dns:///%s:%d", t.externalHostname, *t.conf.Port),
		Issuers:  issuersText.String(),
	}
	jsonDetails, _ := json.Marshal(&localDetails)

	return &prototk.GetLocalDetailsResponse{
		TransportDetails: string(jsonDetails),
	}, nil

}
