// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugins

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"testing"

	interPaladinProto "github.com/kaleido-io/talaria/pkg/plugins/proto"
	pluginInterfaceProto "github.com/kaleido-io/talaria/pkg/talaria/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var externalTestingPluginPort = 10001

var (
	certificateRoot = "../../test/certs/"

	// Certs for the plugin under test
	pluginCAPath         = certificateRoot + "ca1/ca.crt"
	pluginClientCertPath = certificateRoot + "ca1/clients/client1.crt"
	pluginClientKeyPath  = certificateRoot + "ca1/clients/client1.key"

	// Good client is all of the files in the ca2 directory (good == trusted)
	goodClientCaPath   = certificateRoot + "ca2/ca.crt"
	goodClientCertPath = certificateRoot + "ca2/clients/client1.crt"
	goodClientKeyPath  = certificateRoot + "ca2/clients/client1.key"

	// Bad client is all of the files in the ca3 directory (bad == untrusted)
	// badClientCaPath = certificateRoot + "ca3/ca.crt"
	badClientCertPath = certificateRoot + "ca3/clients/client1.crt"
	badClientKeyPath  = certificateRoot + "ca3/clients/client1.key"
)

func getTestGRPCPlugin(ctx context.Context) (*GRPCTransportPlugin, error) {
	clientCertificate, err := tls.LoadX509KeyPair(pluginClientCertPath, pluginClientKeyPath)
	if err != nil {
		return nil, err
	}

	plugin := NewGRPCTransportPlugin(ctx, externalTestingPluginPort, clientCertificate)
	plugin.Start(ctx)
	return plugin, nil
}

func TestMessageSendRecieveFlowFromSingleSender(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gp, err := getTestGRPCPlugin(ctx)
	assert.NoError(t, err)

	conn, err := grpc.NewClient(fmt.Sprintf("unix://%s", gp.SocketName), grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.Nil(t, err)
	defer conn.Close()

	client := pluginInterfaceProto.NewPluginInterfaceClient(conn)
	stream, err := client.PluginMessageFlow(context.Background())
	
	assert.Nil(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	messages := make(chan []byte, 10)
	go func() {
		for {
			returnedMessage, err := stream.Recv()
			if err == io.EOF {
				return
			} else {
				assert.Nil(t, err)
			}

			messages <- []byte(returnedMessage.Payload)
			wg.Done()
			// stream.CloseSend()
		}
	}()

	// Provide the CA that the client is configured with and check that the comms work
	caCert, err := os.ReadFile(pluginCAPath)
	assert.NoError(t, err)
	ok := gp.AddNewKnownPeer(caCert)
	assert.Equal(t, true, ok)

	// Send a message that does will not verify over mTLS and check that the comms do not work
	routeInfo := &GRPCRoutingInformation{
		Address: fmt.Sprintf("localhost:%d", externalTestingPluginPort),
		CACertificate: "", // Not going to be able to verify the plugins Cert
	}
	sri, err := json.Marshal(routeInfo)
	assert.NoError(t, err)
	stream.Send(&pluginInterfaceProto.PaladinMessage{
		Payload: []byte("FAIL"),
		RoutingInformation: sri,
	})

	routeInfo = &GRPCRoutingInformation{
		Address: fmt.Sprintf("localhost:%d", externalTestingPluginPort),
		CACertificate: string(caCert),
	}
	sri, err = json.Marshal(routeInfo)
	assert.NoError(t, err)
	stream.Send(&pluginInterfaceProto.PaladinMessage{
		Payload: []byte("PASS"),
		RoutingInformation: sri,
	})

	wg.Wait()
	assert.NoError(t, err)
}

func TestSingleMessageRecieveFlowFromMultipleSenders(t *testing.T) {
	// The plugin needs to be able to handle talking to other plugins that have different CAs
	// this test is making sure that we only are speaking to connections that we trust.
	//
	// This test is not mimicing the flow from Talaria down to the plugin, it is strictly
	// using the flow from the plugin up,
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gp, err := getTestGRPCPlugin(ctx)
	assert.NoError(t, err)

	// Need to make a connection to the plugin to read back messages that have been recieved
	conn, err := grpc.NewClient(fmt.Sprintf("unix://%s", gp.SocketName), grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.Nil(t, err)
	defer conn.Close()

	// create stream
	client := pluginInterfaceProto.NewPluginInterfaceClient(conn)
	stream, err := client.PluginMessageFlow(context.Background())
	assert.Nil(t, err)

	// Read back messages from the plugin
	messages := make(chan []byte, 10)
	go func() {
		for {
			returnedMessage, err := stream.Recv()
			if err == io.EOF {
				return
			} else {
				assert.Nil(t, err)
			}

			messages <- []byte(returnedMessage.Payload)
			stream.CloseSend()
		}
	}()

	pluginClientCA, err := os.ReadFile(pluginCAPath)
	assert.NoError(t, err)

	// Now we need to spin up 2 clients to speak to the server, one is going have its CA trusted (so the connection
	// is deemed as valid) and the other is not, although it will provide a client cert.
	goodClientCertificate, err := tls.LoadX509KeyPair(goodClientCertPath, goodClientKeyPath)
	assert.NoError(t, err)
	goodClientCA, err := os.ReadFile(goodClientCaPath)
	assert.NoError(t, err)
	goodClientCertPool := x509.NewCertPool()
	goodClientCertPool.AppendCertsFromPEM(pluginClientCA)
	goodClientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{goodClientCertificate},
		RootCAs:      goodClientCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
	}
	gconn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", externalTestingPluginPort), grpc.WithTransportCredentials(credentials.NewTLS(goodClientTLSConfig)))
	assert.Nil(t, err)
	defer gconn.Close()
	goodClient := interPaladinProto.NewInterPaladinTransportClient(gconn)

	// Now create the bad client
	badClientCertificate, err := tls.LoadX509KeyPair(badClientCertPath, badClientKeyPath)
	assert.NoError(t, err)
	badClientCAPool := x509.NewCertPool()
	badClientCAPool.AppendCertsFromPEM(pluginClientCA)
	badClientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{badClientCertificate},
		RootCAs: badClientCAPool,
	}
	bconn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", externalTestingPluginPort), grpc.WithTransportCredentials(credentials.NewTLS(badClientTLSConfig)))
	assert.Nil(t, err)
	defer bconn.Close()
	badClient := interPaladinProto.NewInterPaladinTransportClient(bconn)

	// ---------------------------------------------------------------------------------------------------------------------------------

	// We expect this to be rejected since the CA is not currently in the store of the plugin
	_, err = goodClient.SendInterPaladinMessage(ctx, &interPaladinProto.InterPaladinMessage{
		Payload: []byte("FAIL"),
	})
	assert.Error(t, err)
	
	// Need to close the connection otherwise it will be re-used since we're on localhost
	gconn.Close()

	// Add the new known peer and the recreate the client
	ok := gp.AddNewKnownPeer(goodClientCA)
	assert.Equal(t, true, ok)
	gconn, err = grpc.NewClient(fmt.Sprintf("localhost:%d", externalTestingPluginPort), grpc.WithTransportCredentials(credentials.NewTLS(goodClientTLSConfig)))
	assert.Nil(t, err)
	goodClient = interPaladinProto.NewInterPaladinTransportClient(gconn)

	// // We now expect the exact same request to work
	_, err = goodClient.SendInterPaladinMessage(ctx, &interPaladinProto.InterPaladinMessage{
		Payload: []byte("PASS"),
	})
	assert.NoError(t, err)

	// And we will never expect this to work since the CA isn't in the store
	_, err = badClient.SendInterPaladinMessage(ctx, &interPaladinProto.InterPaladinMessage{
		Payload: []byte("FAIL"),
	})
	assert.Error(t, err)

	// During this run we should have only ever had a single valid message
	assert.Equal(t, 1, len(messages))

	// And if we look at the content of the message, it should be the pass message
	msg := <- messages
	assert.Equal(t, []byte("PASS"), msg)
}

func TestMessageFlowSingleMessage(t *testing.T) {
	// To show messaging working from one end to another, we're going ot make the plugin make a connection
	// to itself on localhost and then vertify that we're able to see the message coming through to the channel
	//
	// Essentially this test is pretending that it's Talaria
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gp, err := getTestGRPCPlugin(ctx)
	assert.NoError(t, err)

	conn, err := grpc.NewClient(fmt.Sprintf("unix://%s", gp.SocketName), grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.Nil(t, err)
	defer conn.Close()

	// create stream
	client := pluginInterfaceProto.NewPluginInterfaceClient(conn)
	stream, err := client.PluginMessageFlow(context.Background())
	assert.Nil(t, err)

	messages := make(chan []byte, 10)

	// Start a routine for listening to messages from the plugin, store the outputs in a buffered channel
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			returnedMessage, err := stream.Recv()
			if err == io.EOF {
				return
			} else {
				assert.Nil(t, err)
			}

			messages <- []byte(returnedMessage.Payload)

			// When we get 10 messages in the channel, let's exit
			if len(messages) == 10 {
				wg.Done()
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		for i := 0; i < 10; i++ {
			req := &pluginInterfaceProto.PaladinMessage{
				Payload:            []byte("Hello, World!"),
				RoutingInformation: []byte("{\"address\":\"localhost:10001\"}"),
			}
			if err := stream.Send(req); err != nil {
				log.Fatalf("can not send %v", err)
			}
		}
		wg.Done()
	}()

	wg.Wait()

	// Verify after the flow is complete that we have 10 messages in our buffer
	assert.Equal(t, 10, len(messages))
}

func TestGetRegistration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gp, err := getTestGRPCPlugin(ctx)
	assert.NoError(t, err)

	reg := gp.GetRegistration()
	assert.NotNil(t, reg.Name)
	assert.NotNil(t, reg.SocketLocation)
}
