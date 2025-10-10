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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	privatekey, _ := rsa.GenerateKey(rand.Reader, 1024 /* smallish key to make the test faster */)
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
	transport := NewGRPCTransport(callbacks).(*grpcTransport)
	res, err := transport.ConfigureTransport(transport.bgCtx, &prototk.ConfigureTransportRequest{
		Name:       "grpc",
		ConfigJson: string(jsonConf),
	})
	assert.NoError(t, err)
	assert.NotNil(t, res)

	// Build the transport details for this plugin
	transportDetails := &PublishedTransportDetails{
		Endpoint: "dns:///" + transport.listener.Addr().String(),
		Issuers:  nodeCert, // self-signed
	}

	// Wait until the socket is up
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		conn, err := net.Dial("tcp", transport.listener.Addr().String())
		if err != nil {
			return
		}
		_ = conn.Close()
		assert.True(c, true) // explicitly pass
	}, 2*time.Second, 10*time.Millisecond, "server took too long to start")

	return transport, transportDetails, callbacks, func() {
		panicked := recover()
		if panicked != nil {
			panic(panicked)
		}
		transport.grpcServer.Stop()
		<-transport.serverDone
	}
}

func mockRegistry(cb *testCallbacks, ptds map[string]*PublishedTransportDetails) {
	reg := make(map[string]string)
	for node, ptd := range ptds {
		reg[node] = pldtypes.JSONString(ptd).String()
	}
	cb.getTransportDetails = func(ctx context.Context, gtdr *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
		res := reg[gtdr.Node]
		if res == "" {
			return nil, fmt.Errorf("not found")
		}
		return &prototk.GetTransportDetailsResponse{
			TransportDetails: res,
		}, nil
	}
}

func newSuccessfulVerifiedConnection(t *testing.T, setup ...func(callbacks1, callbacks2 *testCallbacks)) (plugin1, plugin2 *grpcTransport, done func()) {
	// the default config is direct cert verification
	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	plugin2, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})

	// Register nodes
	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	for _, fn := range setup {
		fn(callbacks1, callbacks2)
	}

	deactivate := testActivatePeer(t, plugin1, "node2", transportDetails2)

	return plugin1, plugin2, func() {
		deactivate()
		done1()
		done2()
	}
}

func testActivatePeer(t *testing.T, sender *grpcTransport, remoteNodeName string, transportDetails *PublishedTransportDetails) func() {

	ctx := context.Background()

	res, err := sender.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         remoteNodeName,
		TransportDetails: pldtypes.JSONString(transportDetails).Pretty(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, res)

	return func() {
		res, err := sender.DeactivatePeer(ctx, &prototk.DeactivatePeerRequest{
			NodeName: remoteNodeName,
		})
		assert.NoError(t, err)
		assert.NotNil(t, res)
	}

}

func TestGRPCTransport_DirectCertVerification_OK(t *testing.T) {
	ctx := context.Background()

	received := make(chan *prototk.PaladinMsg)
	plugin1, _, done := newSuccessfulVerifiedConnection(t, func(_, callbacks2 *testCallbacks) {
		callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
			received <- rmr.Message
			return &prototk.ReceiveMessageResponse{}, nil
		}
	})
	defer done()

	// Connect and send from plugin1 to plugin2
	sendRes, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Node: "node2",
		Message: &prototk.PaladinMsg{
			Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, sendRes)

	if err == nil {
		<-received
	}

	details, err := plugin1.GetLocalDetails(ctx, &prototk.GetLocalDetailsRequest{})
	require.NoError(t, err)
	var pubDetails PublishedTransportDetails
	err = json.Unmarshal([]byte(details.TransportDetails), &pubDetails)
	require.NoError(t, err)
	require.Contains(t, pubDetails.Issuers, "CERTIFICATE")

}

func TestGRPCTransport_DirectCertVerificationWithKeyRotation_OK(t *testing.T) {
	ctx := context.Background()

	received := make(chan *prototk.PaladinMsg)

	// the default config is direct cert verification
	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()

	// Add an old cert to the PEM ahead of the good one
	node1CertOld, _ := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	transportDetails1.Issuers = fmt.Sprintf("%s\n%s", node1CertOld, node1Cert)

	// Register nodes
	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
		received <- rmr.Message
		return &prototk.ReceiveMessageResponse{}, nil
	}

	// Connect and send from plugin1 to plugin2
	deactivate := testActivatePeer(t, plugin1, "node2", transportDetails2)
	defer deactivate()
	sendRes, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Node: "node2",
		Message: &prototk.PaladinMsg{
			Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
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
	cas, err := getCertListFromPEM(ctx, []byte(caCert))
	assert.NoError(t, err)
	caKey := getRSAKeyFromPEM(t, caKeyPEM)

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, cas[0], caKey)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{
		TLS:                    pldconf.TLSConfig{CA: caCert},
		DirectCertVerification: confutil.P(false),
		CertSubjectMatcher:     confutil.P(`^.*CN=([0-9A-Za-z._-]+).*$`),
	})
	defer done1()
	transportDetails1.Issuers = "" // to ensure we're not falling back to cert verification

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, cas[0], caKey)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{
		TLS:                    pldconf.TLSConfig{CA: caCert},
		DirectCertVerification: confutil.P(false),
		CertSubjectMatcher:     confutil.P(`^.*CN=([0-9A-Za-z._-]+).*$`),
	})
	defer done2()
	transportDetails1.Issuers = "" // to ensure we're not falling back to cert verification

	received := make(chan *prototk.PaladinMsg)
	callbacks2.receiveMessage = func(ctx context.Context, rmr *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
		received <- rmr.Message
		return &prototk.ReceiveMessageResponse{}, nil
	}

	// Register nodes
	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	// Connect and send from plugin1 to plugin2
	deactivate := testActivatePeer(t, plugin1, "node2", transportDetails2)
	defer deactivate()
	sendRes, err := plugin1.SendMessage(ctx, &prototk.SendMessageRequest{
		Node: "node2",
		Message: &prototk.PaladinMsg{
			Component: prototk.PaladinMsg_TRANSACTION_ENGINE,
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, sendRes)

	if err == nil {
		<-received
	}

}

func TestGRPCTransport_CAServerWrongCA(t *testing.T) {

	ctx := context.Background()

	caCert, caKeyPEM := buildTestCertificate(t, pkix.Name{CommonName: "ca"}, nil, nil)
	cas, err := getCertListFromPEM(ctx, []byte(caCert))
	assert.NoError(t, err)
	caKey := getRSAKeyFromPEM(t, caKeyPEM)

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, cas[0], caKey)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{
		TLS:                    pldconf.TLSConfig{CA: caCert},
		DirectCertVerification: confutil.P(false),
	})
	defer done1()
	transportDetails1.Issuers = "" // to ensure we're not falling back to cert verification

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{
		TLS:                    pldconf.TLSConfig{CA: caCert},
		DirectCertVerification: confutil.P(false),
	})
	defer done2()
	transportDetails1.Issuers = "" // to ensure we're not falling back to cert verification

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err = plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Error(t, err)

}

func TestGRPCTransport_CAClientWrongCA(t *testing.T) {

	ctx := context.Background()

	caCert, caKeyPEM := buildTestCertificate(t, pkix.Name{CommonName: "ca"}, nil, nil)
	cas, err := getCertListFromPEM(ctx, []byte(caCert))
	assert.NoError(t, err)
	caKey := getRSAKeyFromPEM(t, caKeyPEM)

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{
		TLS:                    pldconf.TLSConfig{CA: caCert},
		DirectCertVerification: confutil.P(false),
	})
	defer done1()
	transportDetails1.Issuers = "" // to ensure we're not falling back to cert verification

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, cas[0], caKey)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{
		TLS:                    pldconf.TLSConfig{CA: caCert},
		DirectCertVerification: confutil.P(false),
	})
	defer done2()
	transportDetails1.Issuers = "" // to ensure we're not falling back to cert verification

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err = plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Error(t, err)

}

func TestGRPCTransport_DirectCertVerification_WrongIssuerServer(t *testing.T) {

	ctx := context.Background()

	// In this test we try a certificate with the right subject, but not the same CA key
	anotherCert, _ := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()
	transportDetails2.Issuers = anotherCert

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Regexp(t, "PD030007", err)

}

func TestGRPCTransport_DirectCertVerification_WrongIssuerClient(t *testing.T) {

	ctx := context.Background()

	// In this test we try a certificate with the the wrong subject too
	anotherCert, _ := buildTestCertificate(t, pkix.Name{CommonName: "another"}, nil, nil)

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()
	transportDetails1.Issuers = anotherCert

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Error(t, err)

}

func TestGRPCTransport_DirectCertVerification_BadIssuersServer(t *testing.T) {

	ctx := context.Background()

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()
	transportDetails2.Issuers = "Not a PEM"

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Regexp(t, "PD030012", err)

}

func TestGRPCTransport_SubjectRegexpMismatch(t *testing.T) {

	ctx := context.Background()

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{
		CertSubjectMatcher: confutil.P("^O=([0-9a-zA-Z]*)$"),
	})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Regexp(t, "PD030008", err)

}

func TestGRPCTransport_ClientWrongNode(t *testing.T) {

	ctx := context.Background()

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node3": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node3",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Regexp(t, "PD030011", err)

}

func TestGRPCTransport_BadTransportDetails(t *testing.T) {

	ctx := context.Background()

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()

	callbacks1.getTransportDetails = func(ctx context.Context, gtdr *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
		return &prototk.GetTransportDetailsResponse{
			TransportDetails: `{!!! not JSON`,
		}, nil
	}
	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Regexp(t, "PD030006", err)

}

func TestGRPCTransport_BadTransportIssuerPEM(t *testing.T) {

	ctx := context.Background()

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()
	// Put the the private key rather than a PEM certificate
	transportDetails2.Issuers = node2Key

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Regexp(t, "PD030012", err)

}

func TestGRPCTransport_NodeUnknownToServer(t *testing.T) {

	ctx := context.Background()

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, map[string]*PublishedTransportDetails{})

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Error(t, err)

}

func TestGRPCTransport_NodeUnknownToClient(t *testing.T) {

	ctx := context.Background()

	node1Cert, node1Key := buildTestCertificate(t, pkix.Name{CommonName: "node1"}, nil, nil)
	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, node1Cert, node1Key, &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	_, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Regexp(t, "not found", err)

}

func TestGRPCTransport_ServerRejectNoCerts(t *testing.T) {

	ctx := context.Background()

	plugin1, transportDetails1, callbacks1, done1 := newTestGRPCTransport(t, "", "", &Config{})
	defer done1()

	node2Cert, node2Key := buildTestCertificate(t, pkix.Name{CommonName: "node2"}, nil, nil)
	plugin2, transportDetails2, callbacks2, done2 := newTestGRPCTransport(t, node2Cert, node2Key, &Config{})
	defer done2()
	// For test we ask for one, but don't have one to give
	plugin2.peerVerifier.baseTLSConfig.ClientAuth = tls.RequestClientCert

	ptds := map[string]*PublishedTransportDetails{"node1": transportDetails1, "node2": transportDetails2}
	mockRegistry(callbacks1, ptds)
	mockRegistry(callbacks2, ptds)

	_, err := plugin1.ActivatePeer(ctx, &prototk.ActivatePeerRequest{
		NodeName:         "node2",
		TransportDetails: pldtypes.JSONString(transportDetails2).Pretty(),
	})
	assert.Error(t, err)

}

func TestTLSVerifierRejectsOverrideServerName(t *testing.T) {
	err := (&tlsVerifier{}).OverrideServerName("whatever")
	assert.Error(t, err)
}
