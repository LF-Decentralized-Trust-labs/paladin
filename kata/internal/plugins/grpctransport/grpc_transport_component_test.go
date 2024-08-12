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
	"io"
	"net"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/grpc/credentials"

	grpctransport "github.com/kaleido-io/paladin/kata/internal/plugins/grpctransport"
	interPaladinPB "github.com/kaleido-io/paladin/kata/pkg/proto/interpaladin"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
)

type fakeCommsBusServer struct {
	proto.UnimplementedKataMessageServiceServer

	initializedOk  bool
	listenMessages chan string
	recvMessages   map[string]chan *proto.Message
	sendMessages   map[string]chan *proto.Message
}

func (fcb *fakeCommsBusServer) Listen(lr *proto.ListenRequest, ls proto.KataMessageService_ListenServer) error {
	// On a new connection to the fake server, store the registered destination and the messages it sends
	ctx := ls.Context()

	if fcb.recvMessages[lr.Destination] == nil {
		fcb.recvMessages[lr.Destination] = make(chan *proto.Message)
	}

	if fcb.sendMessages[lr.Destination] == nil {
		fcb.sendMessages[lr.Destination] = make(chan *proto.Message)
	}

	go func() {
		fcb.listenMessages <- lr.Destination

		for {
			var msg *proto.Message
			err := ls.RecvMsg(msg)
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}

			fcb.recvMessages[msg.Destination] <- msg
		}
	}()

	messageChan := fcb.sendMessages[lr.Destination]

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-messageChan:
			err := ls.Send(msg)
			if err != nil {
				return err
			}
		}
	}
}
func (fcb *fakeCommsBusServer) SubscribeToTopic(context.Context, *proto.SubscribeToTopicRequest) (*proto.SubscribeToTopicResponse, error) {
	return nil, nil
}
func (fcb *fakeCommsBusServer) SendMessage(context.Context, *proto.Message) (*proto.SendMessageResponse, error) {
	return nil, nil
}
func (fcb *fakeCommsBusServer) PublishEvent(context.Context, *proto.Event) (*proto.PublishEventResponse, error) {
	return nil, nil
}
func (fcb *fakeCommsBusServer) Status(context.Context, *proto.StatusRequest) (*proto.StatusResponse, error) {
	if fcb.initializedOk {
		return &proto.StatusResponse{
			Ok: true,
		}, nil
	}

	return nil, fmt.Errorf("bangbangbang")
}
func (fcb *fakeCommsBusServer) ListDestinations(context.Context, *proto.ListDestinationsRequest) (*proto.ListDestinationsResponse, error) {
	return nil, nil
}

type fakeExternalGRPCServer struct {
	interPaladinPB.UnimplementedInterPaladinTransportServer

	listener net.Listener
}

func (fegs *fakeExternalGRPCServer) SendInterPaladinMessage(ctx context.Context, message *interPaladinPB.InterPaladinMessage) (*interPaladinPB.InterPaladinMessage, error) {
	fegs.listener.Close()
	return nil, nil
}

func TestGRPCTransportEndToEnd(t *testing.T) {
	/*

		This test:
			- Stands up a faked comms bus and plugin registry
			- Stands up a gRPC Transport Server configured for mTLS
			- Stands up a fake gRPC Server for the plugin to talk to
			- Generates messages through the comms bus and checks that they're sent properly through to the other Paladin

		This test is notably diffrent from some of the UTs in the grpc transport plugin file as this test actually configures
		a real external server (it's just that the real external server and the fake one are operating on localhost)

	*/

	// ---------------------------------------------------------------------------- Fake Comms Bus
	testDir := os.TempDir()
	fakeCommsBusSocketLocation := path.Join(testDir, fmt.Sprintf("%s.sock", uuid.New().String()))
	defer os.Remove(fakeCommsBusSocketLocation)

	commsbusListener, err := net.Listen("unix", fakeCommsBusSocketLocation)
	assert.NoError(t, err)
	commsBusServer := grpc.NewServer()
	fakeCommsBus := &fakeCommsBusServer{
		initializedOk: true,

		// There will only be a single listener
		listenMessages: make(chan string, 1),
		sendMessages:   make(map[string]chan *proto.Message),
		recvMessages:   make(map[string]chan *proto.Message),
	}
	proto.RegisterKataMessageServiceServer(commsBusServer, fakeCommsBus)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := commsBusServer.Serve(commsbusListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	// ---------------------------------------------------------------------------- mTLS Certificate Configuration

	// Certs for Real Paladin
	realPaladinCACert, err := os.ReadFile("../../../test/ca1/ca.crt")
	assert.NoError(t, err)
	realPaladinServerCert, err := tls.LoadX509KeyPair("../../../test/ca1/clients/client1.crt", "../../../test/ca1/clients/client1.key")
	assert.NoError(t, err)
	realPaladinClientCert, err := tls.LoadX509KeyPair("../../../test/ca1/clients/client2.crt", "../../../test/ca1/clients/client2.key")
	assert.NoError(t, err)

	// Certs for the faked Paladin
	// fakePaladinCACert, err := os.ReadFile("../../../test/ca2/ca.crt")
	// assert.NoError(t, err)
	// fakePaladinServerCert, err := tls.LoadX509KeyPair("../../../test/ca2/clients/client1.crt", "../../../test/ca2/clients/client1.key")
	// assert.NoError(t, err)
	fakePaladinClientCert, err := tls.LoadX509KeyPair("../../../test/ca2/clients/client2.crt", "../../../test/ca2/clients/client2.key")
	assert.NoError(t, err)

	// ---------------------------------------------------------------------------- GRPC Transport Provider

	providerDestinationName := uuid.New().String()
	providerPort := 8080

	// Need this to be an mTLS enabled so need to do the config now
	config := &grpctransport.GRPCTransportConfig{
		ExternalListenPort: providerPort,
		ClientCertificate:  &realPaladinClientCert,
		ServerCertificate:  &realPaladinServerCert,
	}

	err = grpctransport.InitializeTransportProvider(fakeCommsBusSocketLocation, providerDestinationName, config)
	assert.NoError(t, err)

	// Verify that we have a message in the comms registering the provider
	newListener := <-fakeCommsBus.listenMessages
	assert.Equal(t, providerDestinationName, newListener)

	// ---------------------------------------------------------------------------- GRPC Transport Instance

	newTransportInstanceName := uuid.New().String()

	// Send a message to the registered provider to make a new instance
	createInstanceMessage := &pb.CreateInstance{
		MessageDestination: providerDestinationName,
		Name:               newTransportInstanceName,
	}
	body, err := anypb.New(createInstanceMessage)
	assert.NoError(t, err)
	createInstancePBMessage := &proto.Message{
		Id:          uuid.New().String(),
		Destination: providerDestinationName,
		Body:        body,
	}

	// Send the message containing the create instance request to the transport provider
	fakeCommsBus.sendMessages[providerDestinationName] <- createInstancePBMessage

	// Verify that we have a message in the comms registering the instance
	newInstanceName := <-fakeCommsBus.listenMessages
	assert.Equal(t, newTransportInstanceName, newInstanceName)

	// ---------------------------------------------------------------------------- Fake other GRPC Plugin

	// Need to make another Paladin node that this one can talk to, we fake up its implementation, but comms to it are
	// going to be done over TCP on localhost (so the certs need to have localhost in their SANs)

	fakePaladinListener, err := net.Listen("tcp", fmt.Sprintf(":%d", providerPort + 1))
	assert.NoError(t, err)
	fakeServer := &fakeExternalGRPCServer{
		listener: fakePaladinListener,
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM([]byte(realPaladinCACert))
	assert.Equal(t, true, ok)

	fakePaladinServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{fakePaladinClientCert},
		RootCAs:      certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	})))
	interPaladinPB.RegisterInterPaladinTransportServer(fakePaladinServer, fakeServer)

	// Start the fake server
	var fakeServerWg sync.WaitGroup
	fakeServerWg.Add(1)
	go func(){
		_ = fakePaladinServer.Serve(fakePaladinListener)
		fakeServerWg.Done()
	}()

	// ---------------------------------------------------------------------------- Test Flow

	/*

		At this point, we're initialised with a gRPC transport provider, an instance, and a fake other paladin node
		that are all prepped to do mTLS. We now run through a few messaging flows:

			- Sending a message from the comms bus a Paladin with valid configuration
			- Sending a message form the comms bus a Paladin where the certs don't work

	*/

	// ---------------------------------------------------------------------------- Send a messsage through the Comms bus

	// TODO: Setup is done, start sending messages
}
