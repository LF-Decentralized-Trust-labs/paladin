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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/transports/grpc/internal/msgs"
	"google.golang.org/grpc/credentials"
)

// TransportCredentials implementation that performs peer verification against the paladin registry
type tlsVerifier struct {
	tlsVerifierStatic
	baseTLSConfig *tls.Config
	expectedNode  string
}

type tlsVerifierStatic struct {
	t                      *grpcTransport
	directCertVerification bool
	subjectMatchRegex      *regexp.Regexp
}

type tlsVerifierAuthInfo struct {
	credentials.CommonAuthInfo
	authType         string
	cert             *x509.Certificate
	transportDetails *PublishedTransportDetails
	remoteAddr       string
	verifiedNodeName string
}

type hasCommonAuthInfo interface {
	GetCommonAuthInfo() credentials.CommonAuthInfo
}

func (tv *tlsVerifier) returnAuthInfo(dir string, aip *atomic.Pointer[tlsVerifierAuthInfo], c net.Conn, tlsAuthInfo credentials.AuthInfo, err error) (net.Conn, credentials.AuthInfo, error) {
	ai := aip.Load()
	if err == nil && (ai == nil || ai.verifiedNodeName == "") {
		err = i18n.NewError(tv.t.bgCtx, msgs.MsgTLSNegotiationFailed)
	}
	if err != nil {
		log.L(tv.t.bgCtx).Errorf("%s TLS handshake failed: %s", dir, err)
		return nil, nil, err
	}
	ai.remoteAddr = c.RemoteAddr().String()
	ai.authType = tlsAuthInfo.AuthType()
	if ciai, ok := tlsAuthInfo.(hasCommonAuthInfo); ok {
		// unclear the value of this experimental API, but we honor the recommendation to propagate it
		ai.CommonAuthInfo = ciai.GetCommonAuthInfo()
	}
	log.L(tv.t.bgCtx).Infof("%s TLS handshake completed remote=%s authInfo=%s", dir, c.RemoteAddr(), tlsAuthInfo.AuthType())
	return c, ai, nil
}

func (tv *tlsVerifier) ClientHandshake(ctx context.Context, s string, c net.Conn) (net.Conn, credentials.AuthInfo, error) {
	log.L(tv.t.bgCtx).Debugf("Client TLS handshake initiated remote=%s", c.RemoteAddr())
	authInfo, validator := tv.peerValidator()
	c, tlsAuthInfo, err := validator.ClientHandshake(ctx, s, c)
	return tv.returnAuthInfo("Client", authInfo, c, tlsAuthInfo, err)
}

func (tv *tlsVerifier) ServerHandshake(c net.Conn) (net.Conn, credentials.AuthInfo, error) {
	log.L(tv.t.bgCtx).Debugf("Server TLS handshake initiated remote=%s", c.RemoteAddr())
	authInfo, validator := tv.peerValidator()
	c, tlsAuthInfo, err := validator.ServerHandshake(c)
	return tv.returnAuthInfo("Server", authInfo, c, tlsAuthInfo, err)
}

func (ai *tlsVerifierAuthInfo) AuthType() string {
	return ai.authType
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

func getCertListFromPEM(ctx context.Context, pemBytes []byte) (certs []*x509.Certificate, err error) {
	for {
		block, remaining := pem.Decode(pemBytes)
		if block == nil {
			break
		}
		pemBytes = remaining
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgPEMCertificateInvalid)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgPEMCertificateInvalid)
	}
	return certs, err
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
				log.L(ctx).Errorf("subject regexp '%s' mismatch on '%s' len=%d (0:fail,1:no-groups,2+:too-many-groups)",
					tv.subjectMatchRegex, ai.cert.Subject, len(match))
				return err
			}
			node = match[1]
		} else {
			node = ai.cert.Subject.CommonName
		}

		// On the client side we connect expecting to find a particular node on the other side
		if tv.expectedNode != "" && node != tv.expectedNode {
			return i18n.NewError(ctx, msgs.MsgConnectionToWrongNode, node, tv.expectedNode)
		}

		// Ask the Paladin server/registry for details of the node we are peering with
		ai.transportDetails, err = tv.t.getTransportDetails(ctx, node)
		if err != nil {
			log.L(ctx).Error(err.Error())
			return err
		}

		// If we need to check the issuer, do that now
		if tv.directCertVerification {
			issuerCerts, err := getCertListFromPEM(ctx, []byte(ai.transportDetails.Issuers))
			if err != nil {
				return err
			}
			rootPool := x509.NewCertPool()
			issuerSubjects := []string{}
			for _, issuerCert := range issuerCerts {
				rootPool.AddCert(issuerCert)
				issuerSubjects = append(issuerSubjects, issuerCert.Subject.String())
			}
			if _, err = ai.cert.Verify(x509.VerifyOptions{
				// Only need to verify up to that issuer
				Roots: rootPool,
				// We do not verify key usages
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			}); err != nil {
				return i18n.WrapError(ctx, err, msgs.MsgPeerCertificateIssuerInvalid,
					node, ai.cert.Issuer.String(), issuerSubjects,
				)
			}
		}

		// OK - we've verified.
		// We're not completely certain the handshake happens on a single go-routine, so we play safe
		// and use an atomic pointer to pass it back to the waiting TransportCredentials
		// ClientHandshake/ServerHandshake function.
		ai.verifiedNodeName = node
		authInfo.Store(ai)

		return nil
	}
	return authInfo, credentials.NewTLS(tlsConfig)
}
