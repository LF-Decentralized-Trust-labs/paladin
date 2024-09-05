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
	"crypto/x509"
	"encoding/json"
	"net"
	"regexp"
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
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
	bgCtx     context.Context
	callbacks plugintk.TransportCallbacks

	conf        Config
	connLock    sync.Mutex
	connections map[string]*grpcConnection
}

type grpcConnectionListener struct {
	net.Listener // this is the standard convention for overriding default listener
	proto.UnimplementedPaladinGRPCTransportServer
	t                      *grpcTransport
	grpcServer             *grpc.Server
	directCertVerification bool
	subjectMatchRegex      *regexp.Regexp
}

type grpcConnection struct {
	bgCtx            context.Context
	t                *grpcTransport
	gcl              *grpcConnectionListener
	transportDetails *PublishedTransportDetails
	direction        string
	netConn          net.Conn
	node             string
	cert             *x509.Certificate
}

func NewPlugin(ctx context.Context, conf *Config) plugintk.PluginBase {
	return plugintk.NewTransport(func(callbacks plugintk.TransportCallbacks) plugintk.TransportAPI {
		return &grpcTransport{bgCtx: ctx, callbacks: callbacks}
	})
}

func (t *grpcTransport) ConfigureTransport(ctx context.Context, req *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
	// Hold the connlock while setting our state (as we'll read it when creating new conns)
	t.connLock.Lock()
	defer t.connLock.Unlock()

	err := json.Unmarshal([]byte(req.ConfigJson), &t.conf)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidTransportConfig)
	}

	listenAddr := confutil.StringOrEmpty(t.conf.Address, "")
	if listenAddr == "" {
		return nil, i18n.NewError(ctx, msgs.MsgListenerAddressRequired)
	}

	// TLS is the only way we know who we're talking to, and we are a privacy system, so it cannot be disabled
	if !t.conf.TLS.Enabled || !t.conf.TLS.ClientAuth {
		return nil, i18n.NewError(ctx, msgs.MsgMTLSCannotBeDisabled)
	}

	directCertVerification := confutil.Bool(t.conf.DirectCertVerification, *ConfigDefaults.DirectCertVerification)
	if directCertVerification {
		// Check the tls default settings haven't been set with conflicting config
		if t.conf.TLS.CAFile != "" || t.conf.TLS.CA != "" || t.conf.TLS.InsecureSkipHostVerify || len(t.conf.TLS.RequiredDNAttributes) > 0 {
			return nil, i18n.NewError(ctx, msgs.MsgConfIncompatibleWithDirectCertVerify)
		}
	}

	var subjectMatchRegex *regexp.Regexp
	certSubjectMatcher := confutil.StringOrEmpty(t.conf.CertSubjectMatcher, "")
	if certSubjectMatcher != "" {
		if subjectMatchRegex, err = regexp.Compile(*t.conf.CertSubjectMatcher); err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidSubjectRegexp, *t.conf.CertSubjectMatcher)
		}
	}

	// We need to generate a connection for each accept, so we need to do our own
	// TLS wrapping
	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	gcl := &grpcConnectionListener{
		Listener:               l,
		t:                      t,
		grpcServer:             grpc.NewServer(),
		directCertVerification: directCertVerification,
		subjectMatchRegex:      subjectMatchRegex,
	}

	// Kick off the gRPC listener
	proto.RegisterPaladinGRPCTransportServer(gcl.grpcServer, gcl)

	return nil, nil
}

// Override the accept on the listener to wrap in our connection
func (gcl *grpcConnectionListener) Accept() (net.Conn, error) {
	c, err := gcl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	gc := &grpcConnection{
		bgCtx:     log.WithLogField(gcl.t.bgCtx, "remote", c.RemoteAddr().String()),
		t:         gcl.t,
		gcl:       gcl,
		netConn:   c,
		direction: "inbound",
	}
	return gc.server()
}

func (gcl *grpcConnectionListener) MessageStream(context.Context, *proto.Message) (*proto.Message, error) {
	panic("unimplemented")
}

func (gc *grpcConnection) server() (net.Conn, error) {
	gc.t.connLock.Lock()
	defer gc.t.connLock.Unlock()

	serverTlsConf, err := tlsconf.BuildTLSConfig(gc.bgCtx, &gc.t.conf.TLS, tlsconf.ServerType)
	if err != nil {
		return gc.netConn, nil
	}

	serverTlsConf.VerifyPeerCertificate = gc.peerValidator
	if gc.gcl.directCertVerification {
		// Do not verify the remote certificate
		serverTlsConf.ClientAuth = tls.RequireAnyClientCert
	}

	// Note we will only add ourselves to the list of connections if we can establish the identity of the other side
	// as a result of a successful TLS handshake
	return tls.Server(gc.netConn, serverTlsConf), nil
}

func (t *grpcTransport) getOrCreateConn(ctx context.Context, node string) (*grpcConnection, error) {
}

func (t *grpcTransport) SendMessage(context.Context, *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {

}

func (gc *grpcConnection) peerValidator(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) (err error) {

	if len(rawCerts) != 1 {
		// We currently require exactly one certificate to be provided by the peer
		return i18n.NewError(gc.bgCtx, msgs.MsgVerifierRequiresOneCert, len(rawCerts))
	}

	// Parse the certificate we are provided
	gc.cert, err = x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return err
	}

	log.L(gc.bgCtx).Infof("Received certificate %s (serial=%s)", gc.cert.Subject.String(), gc.cert.SerialNumber.Text(16))

	if gc.gcl.subjectMatchRegex != nil {
		match := gc.gcl.subjectMatchRegex.FindStringSubmatch(gc.cert.Subject.String())
		if len(match) != 2 /* we require one capture group */ {
			return i18n.NewError(gc.bgCtx, msgs.MsgSubjectRegexpMismatch, len(match))
		}
		gc.node = match[1]
	} else {
		gc.node = gc.cert.Subject.CommonName
	}

	// Ask the Paladin server/registry for details of the node we are peering with
	gtdr, err := gc.t.callbacks.GetTransportDetails(gc.bgCtx, &prototk.GetTransportDetailsRequest{
		Destination: "certcheck@" + gc.node,
	})
	if err != nil {
		log.L(gc.bgCtx).Errorf("lookup failed for node %s: %s", gc.node, err)
		return err
	}

	// Parse the details
	if err = json.Unmarshal([]byte(gtdr.TransportDetails), &gc.transportDetails); err != nil {
		return i18n.WrapError(gc.bgCtx, err, msgs.MsgPeerTransportDetailsInvalid, gc.node)
	}

	// If we need to check the issuer, do that now
	if gc.gcl.directCertVerification {
		issuerCert, err := x509.ParseCertificate([]byte(gc.transportDetails.Issuer))
		if err != nil {
			return i18n.WrapError(gc.bgCtx, err, msgs.MsgPeerTransportDetailsInvalid, gc.node)
		}
		rootPool := x509.NewCertPool()
		rootPool.AddCert(issuerCert)
		if _, err = gc.cert.Verify(x509.VerifyOptions{
			// Only need to verify up to that issuer
			Roots: rootPool,
			// We do not verify key usages
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		}); err != nil {
			return i18n.WrapError(gc.bgCtx, err, msgs.MsgPeerCertificateInvalid,
				gc.node, issuerCert.Subject.String(), gc.cert.Issuer.String(),
			)
		}
	}

	// Grab the conn lock at this point
	gc.t.connLock.Lock()
	defer gc.t.connLock.Unlock()

	// Ok - we know who we are talking to, and we are happy!
	// Add this to the connection list - we close any existing connection
	existing := gc.t.connections[gc.node]
	if existing != nil {
		log.L(gc.bgCtx).Infof("Connection %s (%s) is replaced by connection %s (%s)",
			existing.netConn.RemoteAddr(), existing.direction, gc.netConn.RemoteAddr(), gc.direction)
		// Just close the connection
		_ = existing.netConn.Close()
	}
	gc.t.connections[gc.node] = gc
	return nil
}
