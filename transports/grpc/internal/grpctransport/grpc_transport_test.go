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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/tlsconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCallbacks struct {
	getTransportDetails func(context.Context, *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error)
	receiveMessage      func(context.Context, *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error)
}

func (tc *testCallbacks) GetTransportDetails(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
	return tc.getTransportDetails(ctx, req)
}

func (tc *testCallbacks) ReceiveMessage(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	return tc.receiveMessage(ctx, req)
}

func getRSAKeyFromPEM(t *testing.T, pemBytes string) *rsa.PrivateKey {
	block, _ := pem.Decode([]byte(pemBytes))
	assert.NotNil(t, block)
	assert.Equal(t, "RSA PRIVATE KEY", block.Type)
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	assert.NoError(t, err)
	return privateKey
}

func buildTestCertificate(t *testing.T, subject pkix.Name, ca *x509.Certificate, caKey *rsa.PrivateKey) (string, string) {
	// Create an X509 certificate pair
	privatekey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publickey := &privatekey.PublicKey
	var privateKeyBytes []byte = x509.MarshalPKCS1PrivateKey(privatekey)
	privateKeyBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}
	privateKeyPEM := &strings.Builder{}
	err := pem.Encode(privateKeyPEM, privateKeyBlock)
	require.NoError(t, err)
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	x509Template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(100 * time.Second),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:              []string{"127.0.0.1", "localhost"},
	}
	require.NoError(t, err)
	if ca == nil {
		ca = x509Template
		caKey = privatekey
		x509Template.IsCA = true
		x509Template.KeyUsage |= x509.KeyUsageCertSign
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, x509Template, ca, publickey, caKey)
	require.NoError(t, err)
	publicKeyPEM := &strings.Builder{}
	err = pem.Encode(publicKeyPEM, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	require.NoError(t, err)
	return publicKeyPEM.String(), privateKeyPEM.String()
}

func newTestGRPCTransport(t *testing.T, nodeCert, nodeKey string, conf *Config) (*grpcTransport, *PublishedTransportDetails, *testCallbacks, func()) {
	// Grab a localhost port to use and put that in config
	portGrabber, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	port := portGrabber.Addr().(*net.TCPAddr).Port
	err = portGrabber.Close()
	assert.NoError(t, err)
	conf.Port = &port
	conf.Address = confutil.P("127.0.0.1")

	// Put the certs in the config
	conf.TLS.Cert = nodeCert
	conf.TLS.Key = nodeKey

	// Serialize the config
	jsonConf, err := json.Marshal(conf)
	assert.NoError(t, err)

	//  construct the plugin
	callbacks := &testCallbacks{}
	transport := grpcTransportFactory(callbacks).(*grpcTransport)
	res, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: string(jsonConf),
	})
	assert.NoError(t, err)
	assert.NotNil(t, res)

	// Build the transport details for this plugin
	transportDetails := &PublishedTransportDetails{
		Endpoint: "dns:///" + transport.listener.Addr().String(),
		Issuer:   nodeCert, // self-signed
	}

	return transport, transportDetails, callbacks, func() {
		panicked := recover()
		if panicked != nil {
			panic(panicked)
		}
		transport.grpcServer.Stop()
		<-transport.serverDone
	}
}

func TestPluginLifecycle(t *testing.T) {
	pb := NewPlugin(context.Background())
	assert.NotNil(t, pb)
}

func TestGRPCTransport_DirectCertVerification_OK(t *testing.T) {

	ctx := context.Background()

	// the default config is direct cert verification
	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()

	received := make(chan *prototk.Message)
	callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
		received <- rmr.Message
		return &prototk.ReceiveMessageResponse{}, nil
	}

	// Register nodes
	fakeRegistry := func(ctx context.Context, gtdr *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
		reg := map[string]string{
			"node1": tktypes.JSONString(transportDetails1).String(),
			"node2": tktypes.JSONString(transportDetails2).String(),
		}
		assert.Contains(t, reg, gtdr.Node)
		return &prototk.GetTransportDetailsResponse{
			TransportDetails: reg[gtdr.Node],
		}, nil
	}
	callbacks1.getTransportDetails = fakeRegistry
	callbacks2.getTransportDetails = fakeRegistry

	// Connect plugin1 to plugin2
	sendRes, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Message: &prototk.Message{
			ReplyTo:     "to.me@node1",
			Destination: "to.you@node2",
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, sendRes)

	if err == nil {
		<-received
	}

}

func TestGRPCTransport_CACertVerificationWithSubjectRegex_OK(t *testing.T) {

	ctx := context.Background()

	caCert, caKeyPEM := buildTestCertificate(t, pkix.Name{CommonName: "ca"}, nil, nil)
	ca, err := getCertFromPEM(ctx, []byte(caCert))
	assert.NoError(t, err)
	caKey := getRSAKeyFromPEM(t, caKeyPEM)

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, ca, caKey)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{
		TLS:                    tlsconf.Config{CA: caCert},
		DirectCertVerification: confutil.P(false),
	})
	defer done1()
	transportDetails1.Issuer = "" // to ensure we're not falling back to cert verification

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, ca, caKey)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{
		TLS:                    tlsconf.Config{CA: caCert},
		DirectCertVerification: confutil.P(false),
	})
	defer done2()
	transportDetails1.Issuer = "" // to ensure we're not falling back to cert verification

	received := make(chan *prototk.Message)
	callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
		received <- rmr.Message
		return &prototk.ReceiveMessageResponse{}, nil
	}

	// Register nodes
	fakeRegistry := func(ctx context.Context, gtdr *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
		reg := map[string]string{
			"node1": tktypes.JSONString(transportDetails1).String(),
			"node2": tktypes.JSONString(transportDetails2).String(),
		}
		assert.Contains(t, reg, gtdr.Node)
		return &prototk.GetTransportDetailsResponse{
			TransportDetails: reg[gtdr.Node],
		}, nil
	}
	callbacks1.getTransportDetails = fakeRegistry
	callbacks2.getTransportDetails = fakeRegistry

	// Connect plugin1 to plugin2
	sendRes, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Message: &prototk.Message{
			ReplyTo:     "to.me@node1",
			Destination: "to.you@node2",
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, sendRes)

	if err == nil {
		<-received
	}

}
