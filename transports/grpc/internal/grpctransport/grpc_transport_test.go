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

func buildSelfSignedTLSKeyPair(t *testing.T, subject pkix.Name) (string, string) {
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
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	require.NoError(t, err)
	derBytes, err := x509.CreateCertificate(rand.Reader, x509Template, x509Template, publickey, privatekey)
	require.NoError(t, err)
	publicKeyPEM := &strings.Builder{}
	err = pem.Encode(publicKeyPEM, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	require.NoError(t, err)
	return publicKeyPEM.String(), privateKeyPEM.String()
}

func newTestPlugin(t *testing.T, certSubject pkix.Name, conf *Config) (*grpcTransport, *PublishedTransportDetails, *testCallbacks, func()) {
	// Grab a localhost port to use and put that in config
	portGrabber, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	port := portGrabber.Addr().(*net.TCPAddr).Port
	err = portGrabber.Close()
	assert.NoError(t, err)
	conf.Port = &port
	conf.Address = confutil.P("127.0.0.1")

	// Build the certs for the config
	nodeCert, nodeKey := buildSelfSignedTLSKeyPair(t, certSubject)
	conf.TLS.Cert = nodeCert
	conf.TLS.Key = nodeKey

	// Serialize the config
	jsonConf, err := json.Marshal(conf)
	assert.NoError(t, err)

	//  construct the plugin
	ctx := context.Background()
	callbacks := &testCallbacks{}
	transport := newGRPCTransport(ctx, callbacks)
	res, err := transport.ConfigureTransport(ctx, &prototk.ConfigureTransportRequest{
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

func TestGRPCTransportPingPong(t *testing.T) {

	ctx := context.Background()

	plugin1, transportDetails1, callbacks1, done1 := newTestPlugin(t, pkix.Name{
		CommonName: "node1",
	}, &Config{})
	defer done1()

	_, transportDetails2, callbacks2, done2 := newTestPlugin(t, pkix.Name{
		CommonName: "node2",
	}, &Config{})
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
			Destination: "anything@node2",
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, sendRes)

	<-received

}
