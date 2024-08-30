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

// Run in the test package to mimic actual usage (this also let us redefine a fake comms bus)
package grpctransport_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/yaml.v3"

	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"

	grpctransport "github.com/kaleido-io/paladin/kata/internal/plugins/grpctransport"
	interPaladinPB "github.com/kaleido-io/paladin/kata/pkg/proto/interpaladin"
)

type fakePluginController struct {
	prototk.UnimplementedPluginControllerServer

	recvMessages chan *prototk.TransportMessage
	sendMessages chan *prototk.TransportMessage
}

func (fpc *fakePluginController) ConnectTransport(stream grpc.BidiStreamingServer[prototk.TransportMessage, prototk.TransportMessage]) error {
	ctx := stream.Context()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-fpc.sendMessages:
				err := stream.Send(msg)
				if err != nil {
					return
				}
			}
		}
	}()

	for {
		inboundMessage, err := stream.Recv()
		if err != nil {
			return err
		}

		fpc.recvMessages <- inboundMessage
	}
}

func newFakePluginController(location string) (*fakePluginController, error) {
	listener, err := net.Listen("unix", location)
	if err != nil {
		return nil, err
	}

	fpc := &fakePluginController{
		recvMessages: make(chan *prototk.TransportMessage),
		sendMessages: make(chan *prototk.TransportMessage),
	}

	s := grpc.NewServer()
	prototk.RegisterPluginControllerServer(s, fpc)

	go func() {
		err := s.Serve(listener)
		if err != nil {
			return
		}
	}()

	return fpc, nil
}

func tempUDS(t *testing.T) string {
	// Not safe to use t.TempDir() as it generates too long paths including the test name
	f, err := os.CreateTemp("", "ut_*.sock")
	assert.NoError(t, err)
	_ = f.Close()
	allocatedUDSName := f.Name()
	err = os.Remove(allocatedUDSName)
	assert.NoError(t, err)
	t.Cleanup(func() {
		err := os.Remove(allocatedUDSName)
		assert.True(t, err == nil || os.IsNotExist(err))
	})
	return allocatedUDSName
}

type fakeExternalGRPCServer struct {
	interPaladinPB.UnimplementedInterPaladinTransportServer

	listener     net.Listener
	recvMessages chan *components.TransportMessage
}

func (fegs *fakeExternalGRPCServer) SendInterPaladinMessage(ctx context.Context, message *interPaladinPB.InterPaladinMessage) (*interPaladinPB.InterPaladinMessage, error) {
	transportedMessage := &components.TransportMessage{}
	err := yaml.Unmarshal(message.Body, transportedMessage)
	if err != nil {
		return nil, err
	}
	fegs.recvMessages <- transportedMessage
	return nil, nil
}

func setupPluginForTest(location string) (*grpctransport.GRPCTransport, error) {
	transport, err := grpctransport.NewGRPCTransport(uuid.NewString(), fmt.Sprintf("unix://%s", location))
	if err != nil {
		return nil, err
	}

	return transport, nil
}

func TestGRPCTransportEndToEnd(t *testing.T) {
	// ctx := context.Background()

	pluginControllerSocketLocation := tempUDS(t)

	// ---------------------------------------------------------------------------- mTLS Certificate Configuration

	// Certs for Real Paladin
	realPaladinCACert, err := os.ReadFile("../../../test/ca1/ca.crt")
	assert.NoError(t, err)

	realPaladinServerCertBytes, err := os.ReadFile("../../../test/ca1/clients/client1.crt")
	assert.NoError(t, err)
	realPaladinServerCert := string(realPaladinServerCertBytes)
	realPaladinServerKeyBytes, err := os.ReadFile("../../../test/ca1/clients/client1.key")
	assert.NoError(t, err)
	realPaladinServerKey := string(realPaladinServerKeyBytes)

	realPaladinClientCertBytes, err := os.ReadFile("../../../test/ca1/clients/client1.crt")
	assert.NoError(t, err)
	realPaladinClientCert := string(realPaladinClientCertBytes)
	realPaladinClientKeyBytes, err := os.ReadFile("../../../test/ca1/clients/client1.key")
	assert.NoError(t, err)
	realPaladinClientKey := string(realPaladinClientKeyBytes)


	// realPaladinServerCert, err := tls.LoadX509KeyPair(, "../../../test/ca1/clients/client1.key")
	// assert.NoError(t, err)
	// realPaladinClientCert, err := tls.LoadX509KeyPair("../../../test/ca1/clients/client2.crt", "../../../test/ca1/clients/client2.key")
	// assert.NoError(t, err)

	// // Certs for the faked Paladin
	fakePaladinCACert, err := os.ReadFile("../../../test/ca2/ca.crt")
	assert.NoError(t, err)
	fakePaladinServerCert, err := tls.LoadX509KeyPair("../../../test/ca2/clients/client1.crt", "../../../test/ca2/clients/client1.key")
	assert.NoError(t, err)
	// fakePaladinClientCert, err := tls.LoadX509KeyPair("../../../test/ca2/clients/client2.crt", "../../../test/ca2/clients/client2.key")
	// assert.NoError(t, err)

	// // Certs for the client that does not have its CA trusted
	// untrustedCACert, err := tls.LoadX509KeyPair("../../../test/ca3/clients/client1.crt", "../../../test/ca3/clients/client1.key")
	// assert.NoError(t, err)

	// ---------------------------------------------------------------------------- Stand up a fake Plugin Controller

	pluginController, err := newFakePluginController(pluginControllerSocketLocation)
	assert.NoError(t, err)

	// ---------------------------------------------------------------------------- Stand up a fake Transport

	transport, err := setupPluginForTest(pluginControllerSocketLocation)
	assert.NoError(t, err)

	err = transport.Init()
	assert.NoError(t, err)

	pluginControllerPort := 8080

	// Configure the transport
	config := grpctransport.UnprocessedGRPCConfig{
		ServerCertificate: &realPaladinServerCert,
		ServerKey:         &realPaladinServerKey,
		ClientCertificate: &realPaladinClientCert,
		ClientKey:         &realPaladinClientKey,
		ExternalPort:      pluginControllerPort,
	}
	marshalledConfig, err := yaml.Marshal(config)
	assert.NoError(t, err)
	pluginController.sendMessages <- &prototk.TransportMessage{
		RequestToTransport: &prototk.TransportMessage_ConfigureTransport{
			ConfigureTransport: &prototk.ConfigureTransportRequest{
				ConfigYaml: string(marshalledConfig),
			},
		},
	}
	<-pluginController.recvMessages

	// Initialize the transport
	pluginController.sendMessages <- &prototk.TransportMessage{
		RequestToTransport: &prototk.TransportMessage_InitTransport{
			InitTransport: &prototk.InitTransportRequest{},
		},
	}
	<-pluginController.recvMessages

	// ---------------------------------------------------------------------------- Stand up a fake Paladin node

	// Need to make another Paladin node that this one can talk to, we fake up its implementation, but comms to it are
	// going to be done over TCP on localhost (so the certs need to have localhost in their SANs)
	fakeAddress := fmt.Sprintf(":%d", pluginControllerPort+1)
	fakePaladinListener, err := net.Listen("tcp", fakeAddress)
	assert.NoError(t, err)
	fakeServer := &fakeExternalGRPCServer{
		listener:     fakePaladinListener,
		recvMessages: make(chan *components.TransportMessage, 1),
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM([]byte(realPaladinCACert))
	assert.Equal(t, true, ok)

	fakePaladinServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{fakePaladinServerCert},
		RootCAs:      certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	})))
	interPaladinPB.RegisterInterPaladinTransportServer(fakePaladinServer, fakeServer)

	// Start the fake server
	var fakeServerWg sync.WaitGroup
	fakeServerWg.Add(1)
	go func() {
		_ = fakePaladinServer.Serve(fakePaladinListener)
		fakeServerWg.Done()
	}()

	// ---------------------------------------------------------------------------- Initialize the fake transport

	pluginController.sendMessages <- &prototk.TransportMessage{
		RequestToTransport: &prototk.TransportMessage_InitTransport{
			InitTransport: &prototk.InitTransportRequest{},
		},
	}

	<-pluginController.recvMessages

	// ---------------------------------------------------------------------------- Run the test

	transportDetails := &grpctransport.TransportDetails{
		Address:       fmt.Sprintf(":%d", pluginControllerPort+1),
		CaCertificate: string(fakePaladinCACert),
	}
	td, err := yaml.Marshal(transportDetails)
	assert.NoError(t, err)

	transportMessage := &components.TransportMessage{
		MessageType: "something",
		Payload:     []byte("somethingelse"),
	}
	tm, err := yaml.Marshal(transportMessage)
	assert.NoError(t, err)

	pluginController.sendMessages <- &prototk.TransportMessage{
		RequestToTransport: &prototk.TransportMessage_SendMessage{
			SendMessage: &prototk.SendMessageRequest{
				Body:             string(tm),
				TransportDetails: string(td),
			},
		},
	}

	<-pluginController.recvMessages
	<-fakeServer.recvMessages
}
