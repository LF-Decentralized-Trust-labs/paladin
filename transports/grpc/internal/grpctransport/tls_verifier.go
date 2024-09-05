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
	"encoding/pem"
	"errors"
	"net"
	"regexp"
	"sync/atomic"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/transports/grpc/internal/msgs"
	"google.golang.org/grpc/credentials"
)

// TransportCredentials implementation that performs peer verification against the paladin registry
type tlsVerifier struct {
	tlsVerifierStatic
	baseTLSConfig *tls.Config
}

type tlsVerifierStatic struct {
	t                      *grpcTransport
	directCertVerification bool
	subjectMatchRegex      *regexp.Regexp
}

type tlsVerifierAuthInfo struct {
	credentials.CommonAuthInfo
	cert             *x509.Certificate
	transportDetails *PublishedTransportDetails
	verifiedNodeName string
}

func (tv *tlsVerifier) ClientHandshake(ctx context.Context, s string, c net.Conn) (net.Conn, credentials.AuthInfo, error) {
	authInfo, validator := tv.peerValidator()
	c, tlsAuthInfo, err := validator.ClientHandshake(ctx, s, c)
	ai := authInfo.Load()
	if err == nil && (ai == nil || ai.verifiedNodeName == "") {
		err = i18n.NewError(tv.t.bgCtx, msgs.MsgTLSNegotiationFailed)
	}
	if err != nil {
		return nil, nil, err
	}
	log.L(tv.t.bgCtx).Infof("TLS client handshake completed. TLS authInfo=%s", tlsAuthInfo.AuthType())
	return c, authInfo.Load(), nil
}

func (tv *tlsVerifier) ServerHandshake(c net.Conn) (net.Conn, credentials.AuthInfo, error) {
	authInfo, validator := tv.peerValidator()
	c, tlsAuthInfo, err := validator.ServerHandshake(c)
	ai := authInfo.Load()
	if err == nil && (ai == nil || ai.verifiedNodeName == "") {
		err = i18n.NewError(tv.t.bgCtx, msgs.MsgTLSNegotiationFailed)
	}
	if err != nil {
		return nil, nil, err
	}
	log.L(tv.t.bgCtx).Infof("TLS server handshake completed. TLS authInfo=%s", tlsAuthInfo.AuthType())
	return c, authInfo.Load(), nil
}

func (ai *tlsVerifierAuthInfo) AuthType() string {
	return ai.verifiedNodeName
}

func (tv *tlsVerifier) Info() credentials.ProtocolInfo {
	_, validator := tv.peerValidator()
	return validator.Info()
}

func (tv *tlsVerifier) Clone() credentials.TransportCredentials {
	tv2 := &tlsVerifier{
		tlsVerifierStatic: tv.tlsVerifierStatic,
		baseTLSConfig:     tv.baseTLSConfig.Clone(),
	}
	return tv2
}

// Deprecated function that we do not use or support
func (tv *tlsVerifier) OverrideServerName(s string) error {
	return errors.ErrUnsupported
}

func (tv *tlsVerifier) peerValidator() (*atomic.Pointer[tlsVerifierAuthInfo], credentials.TransportCredentials) {
	authInfo := new(atomic.Pointer[tlsVerifierAuthInfo])
	tlsConfig := tv.baseTLSConfig.Clone()
	tlsConfig.VerifyConnection = func(cs tls.ConnectionState) (err error) {
		ctx := tv.t.bgCtx

		if len(cs.PeerCertificates) != 1 {
			// We currently require exactly one certificate to be provided by the peer
			return i18n.NewError(ctx, msgs.MsgVerifierRequiresOneCert, len(cs.PeerCertificates))
		}

		ai := &tlsVerifierAuthInfo{cert: cs.PeerCertificates[0]}
		log.L(ctx).Infof("Received certificate %s (serial=%s)", ai.cert.Subject.String(), ai.cert.SerialNumber.Text(16))

		var node string
		if tv.subjectMatchRegex != nil {
			match := tv.subjectMatchRegex.FindStringSubmatch(ai.cert.Subject.String())
			if len(match) != 2 /* we require one capture group */ {
				return i18n.NewError(ctx, msgs.MsgSubjectRegexpMismatch, len(match))
			}
			node = match[1]
		} else {
			node = ai.cert.Subject.CommonName
		}

		// Ask the Paladin server/registry for details of the node we are peering with
		ai.transportDetails, err = tv.t.getTransportDetails(ctx, node)

		// If we need to check the issuer, do that now
		if tv.directCertVerification {
			var issuerCert *x509.Certificate
			block, _ := pem.Decode([]byte(ai.transportDetails.Issuer))
			if block != nil {
				issuerCert, err = x509.ParseCertificate(block.Bytes)
			}
			if block == nil || err != nil {
				return i18n.WrapError(ctx, err, msgs.MsgPeerTransportDetailsInvalid, node)
			}
			rootPool := x509.NewCertPool()
			rootPool.AddCert(issuerCert)
			if _, err = ai.cert.Verify(x509.VerifyOptions{
				// Only need to verify up to that issuer
				Roots: rootPool,
				// We do not verify key usages
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			}); err != nil {
				return i18n.WrapError(ctx, err, msgs.MsgPeerCertificateInvalid,
					node, issuerCert.Subject.String(), ai.cert.Issuer.String(),
				)
			}
		}

		// OK - we've verified
		ai.verifiedNodeName = node
		authInfo.Store(ai)

		// Grab the conn lock at this point
		tv.t.connLock.L.Lock()
		defer tv.t.connLock.L.Unlock()

		// Ok - we know who we are talking to, and we are happy!
		// Add this to the connection list - we close any existing connection
		// existing := gc.t.connections[ai.verifiedNodeName]
		// if existing != nil {
		// 	log.L(ctx).Infof("Connection %s (%s) is replaced by connection %s (%s)",
		// 		existing.netConn.RemoteAddr(), existing.direction, gc.netConn.RemoteAddr(), gc.direction)
		// 	// Just close the connection
		// 	_ = existing.netConn.Close()
		// }
		// gc.t.connections[gc.node] = gc
		return nil
	}
	return authInfo, credentials.NewTLS(tlsConfig)
}
