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
	// "io"
	"net"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/anypb"

	grpctransport "github.com/kaleido-io/paladin/kata/internal/plugins/grpctransport"
	grpctransportpb "github.com/kaleido-io/paladin/kata/pkg/proto/grpctransport"
	interPaladinPB "github.com/kaleido-io/paladin/kata/pkg/proto/interpaladin"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
	transaction "github.com/kaleido-io/paladin/kata/pkg/proto/transaction"
)

type fakeCommsBusServer struct {
	proto.UnimplementedKataMessageServiceServer

	initializedOk             bool
	listenMessages            chan string
	recvMessages              map[string]chan *proto.Message
	sendMessages              map[string]chan *proto.Message
	enrichInterPaladinMessage func(*proto.Message) *proto.Message
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

	// Register that we have a new destination
	fcb.listenMessages <- lr.Destination
	// go func() {

	// 	for {
	// 		var msg *proto.Message
	// 		err := ls.RecvMsg(msg)
	// 		if err == io.EOF {
	// 			return
	// 		}
	// 		if err != nil {
	// 			return
	// 		}

	// 		fcb.recvMessages[msg.Destination] <- msg
	// 	}
	// }()

	// Messages being sent to the plugin
	messageChan := fcb.sendMessages[lr.Destination]
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-messageChan:
			enrichedMessage := fcb.enrichInterPaladinMessage(msg)
			err := ls.Send(enrichedMessage)
			if err != nil {
				return err
			}
		}
	}
}
func (fcb *fakeCommsBusServer) SubscribeToTopic(context.Context, *proto.SubscribeToTopicRequest) (*proto.SubscribeToTopicResponse, error) {
	return nil, nil
}
func (fcb *fakeCommsBusServer) SendMessage(ctx context.Context, msg *proto.Message) (*proto.SendMessageResponse, error) {
	// This gets called when we have a message come into the Paladin from a node who's CA we trust
	if fcb.recvMessages[msg.Destination] == nil {
		fcb.recvMessages[msg.Destination] = make(chan *proto.Message, 1)
	}

	fcb.recvMessages[msg.Destination] <- msg
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

	listener     net.Listener
	recvMessages chan *proto.Message
}

func (fegs *fakeExternalGRPCServer) SendInterPaladinMessage(ctx context.Context, message *interPaladinPB.InterPaladinMessage) (*interPaladinPB.InterPaladinMessage, error) {
	recvMessage := &proto.Message{}
	err := message.GetBody().UnmarshalTo(recvMessage)

	if err != nil {
		return nil, err
	}

	fegs.recvMessages <- recvMessage
	return nil, nil
}

func TestGRPCTransportEndToEnd(t *testing.T) {
	ctx := context.Background()

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

	// For now we don't want to do an enriching
	fakeCommsBus.enrichInterPaladinMessage = func(m *proto.Message) *proto.Message { return m }

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
	fakePaladinCACert, err := os.ReadFile("../../../test/ca2/ca.crt")
	assert.NoError(t, err)
	fakePaladinServerCert, err := tls.LoadX509KeyPair("../../../test/ca2/clients/client1.crt", "../../../test/ca2/clients/client1.key")
	assert.NoError(t, err)
	fakePaladinClientCert, err := tls.LoadX509KeyPair("../../../test/ca2/clients/client2.crt", "../../../test/ca2/clients/client2.key")
	assert.NoError(t, err)

	// ---------------------------------------------------------------------------- GRPC Transport Provider

	providerDestinationName := uuid.New().String()
	providerPort := 8080
	realAddress := fmt.Sprintf("localhost:%d", providerPort)

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

	fakeAddress := fmt.Sprintf(":%d", providerPort+1)
	fakePaladinListener, err := net.Listen("tcp", fakeAddress)
	assert.NoError(t, err)
	fakeServer := &fakeExternalGRPCServer{
		listener:     fakePaladinListener,
		recvMessages: make(chan *proto.Message, 1),
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

	// ---------------------------------------------------------------------------- Test Flow

	/*

		At this point, we're initialised with a gRPC transport provider, an instance, and a fake other paladin node
		that are all prepped to do mTLS. We now run through a few messaging flows:

			- Sending a message from the comms bus a Paladin with valid configuration
			- Recieving a message from the internet feeds it all the way up and out of the comms bus

	*/

	// ---------------------------------------------------------------------------- Send a messsage through the Comms bus with valid configuration

	fakePayload := &transaction.SubmitTransactionError{
		Id:     uuid.NewString(),
		Reason: "somedummypayload",
	}
	pbBody, err := anypb.New(fakePayload)
	assert.NoError(t, err)
	messageToCommsBus := &proto.Message{
		Id:          uuid.NewString(),
		Body:        pbBody,
		Destination: newTransportInstanceName,
	}

	fakeCommsBus.enrichInterPaladinMessage = func(m *proto.Message) *proto.Message {
		// This implementation of this is part of the comms bus which handles speaking to the registry to find out more
		// information about where we're sending a payload, but for now we're going to fake it up.

		mAny, err := anypb.New(m)
		assert.NoError(t, err)

		// Interpaladin Transport Information
		ipWrapper := &grpctransportpb.GRPCTransportInformation{
			Address:       fakeAddress,
			CaCertificate: string(fakePaladinCACert),
		}
		ipWrapperAny, err := anypb.New(ipWrapper)
		assert.NoError(t, err)

		externMessage := &proto.ExternalMessage{
			TransportInformation: ipWrapperAny,
			Body:                 mAny,
		}
		externMessageAny, err := anypb.New(externMessage)
		assert.NoError(t, err)

		return &proto.Message{
			Body:        externMessageAny,
			Id:          m.Id,
			Destination: newTransportInstanceName, // Now send to the instance
		}
	}

	// Send the message to the comms bus
	fakeCommsBus.sendMessages[newTransportInstanceName] <- messageToCommsBus

	// Wait for the message to come back and then verify it is the same as the original payload
	recvMessage := <-fakeServer.recvMessages

	submitTransactionErrObj := &transaction.SubmitTransactionError{}
	err = recvMessage.Body.UnmarshalTo(submitTransactionErrObj)
	assert.NoError(t, err)
	assert.Equal(t, "somedummypayload", submitTransactionErrObj.Reason)

	// ---------------------------------------------------------------------------- Send a message into the Paladin from some node that we know

	fakeInboundPayload := &transaction.SubmitTransactionError{
		Id:     uuid.NewString(),
		Reason: "someinboundpayload",
	}
	fakeInboundPayloadPb, err := anypb.New(fakeInboundPayload)
	assert.NoError(t, err)
	inboundMessage := &proto.Message{
		Id:          uuid.NewString(),
		Body:        fakeInboundPayloadPb,
		Destination: newTransportInstanceName,
	}
	inboundMessagePb, err := anypb.New(inboundMessage)
	assert.NoError(t, err)
	interPaladinMessage := &interPaladinPB.InterPaladinMessage{
		Body: inboundMessagePb,
	}

	// After the first test, we know that the Paladin should trust the CA of our client cert, so make a request direct
	// to the gRPC external server presenting the client cert from that CA
	realPaladinCertPool := x509.NewCertPool()
	ok = certPool.AppendCertsFromPEM(realPaladinCACert)
	assert.Equal(t, true, ok)

	fakeClientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{fakePaladinClientCert},
		RootCAs:      realPaladinCertPool,
	}

	conn, err := grpc.NewClient(realAddress, grpc.WithTransportCredentials(credentials.NewTLS(fakeClientTLSConfig)))
	assert.NoError(t, err)
	defer conn.Close()

	client := interPaladinPB.NewInterPaladinTransportClient(conn)

	// SEND IT!
	_, err = client.SendInterPaladinMessage(ctx, interPaladinMessage)
	assert.NoError(t, err)

	// Check that we got the messsage back from the plugin in the comms bus
	<-fakeCommsBus.recvMessages[newTransportInstanceName]
}
