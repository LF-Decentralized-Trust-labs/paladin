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

/*
Test Kata component with no mocking of any internal units.
Starts the GRPC server and drives the internal functions via GRPC messages
*/
package kata

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	transactionsPB "github.com/kaleido-io/paladin/kata/pkg/proto/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestRunTransactionSubmission(t *testing.T) {
	ctx := context.Background()

	socketAddress, done := runServiceForTesting(ctx, t)
	defer done()

	client, done := newClientForTesting(ctx, t, socketAddress)
	defer done()

	testDestination := "test-destination"
	listenerContext, stopListener := context.WithCancel(ctx)
	streams, err := client.Listen(listenerContext, &pb.ListenRequest{
		Destination: testDestination,
	})
	require.NoError(t, err, "failed to call Listen")

	submitTransaction := transactionsPB.SubmitTransactionRequest{
		From:            "fromID",
		ContractAddress: "contract",
		Payload: &transactionsPB.SubmitTransactionRequest_PayloadJSON{
			PayloadJSON: "{\"foo\":\"bar\"}",
		},
	}

	requestId := "requestID"
	body, err := anypb.New(&submitTransaction)
	require.NoError(t, err)
	submitTransactionRequest := &pb.Message{
		Destination: "kata-txn-engine",
		Id:          requestId,
		Body:        body,
		ReplyTo:     &testDestination,
	}

	sendMessageResponse, err := client.SendMessage(ctx, submitTransactionRequest)
	require.NoError(t, err)
	require.NotNil(t, sendMessageResponse)
	assert.Equal(t, pb.SEND_MESSAGE_RESULT_SEND_MESSAGE_OK, sendMessageResponse.GetResult())

	// attempt to receive the response message
	resp, err := streams.Recv()

	require.NotEqual(t, err, io.EOF)
	require.NoError(t, err)
	assert.Equal(t, requestId, resp.GetCorrelationId())
	assert.NotNil(t, resp.GetBody())

	stopListener()
	// Stop the server
	Stop(ctx, socketAddress)
}

func runServiceForTesting(ctx context.Context, t *testing.T) (string, func()) {
	// get a valid file name for a temp file by first creating a temp file and then removing it
	file, err := os.CreateTemp("", "paladin.sock")
	require.NoError(t, err)
	socketAddress := file.Name()
	os.Remove(file.Name())

	// Create and write a config file
	configFile, err := os.CreateTemp("", "test_*.yaml")
	require.NoError(t, err)

	// Write YAML content to the temporary file
	yamlContent := []byte(`
persistence:
  type: sqlite
  sqlite:
    uri:           ":memory:"
    autoMigrate:   true
    migrationsDir: ../../db/migrations/sqlite
    debugQueries:  true
commsBus:  
  grpc:
    socketAddress: ` + socketAddress + `
`)
	_, err = configFile.Write(yamlContent)
	require.NoError(t, err)

	configFile.Close()

	// Start the server
	go Run(ctx, configFile.Name())

	// todo do we really need to sleep here?
	time.Sleep(time.Second * 2)

	return socketAddress, func() {
		os.Remove(configFile.Name())
	}

}

func newClientForTesting(ctx context.Context, t *testing.T, socketAddress string) (pb.KataMessageServiceClient, func()) {
	// Create a gRPC client connection
	conn, err := grpc.NewClient("unix:"+socketAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	// Create a new instance of the gRPC client
	client := pb.NewKataMessageServiceClient(conn)
	status, err := client.Status(ctx, &pb.StatusRequest{})

	delay := 0
	for !status.GetOk() {
		time.Sleep(time.Second)
		delay++
		status, err = client.Status(ctx, &pb.StatusRequest{})
		require.Less(t, delay, 2, "Server did not start after 2 seconds")
	}
	require.NoError(t, err)
	assert.True(t, status.GetOk())
	return client, func() {
		conn.Close()
	}
}

func TestRunPointToPoint(t *testing.T) {
	// Test that 2 clients can send messages to each other if they know each other's destination string
	ctx := context.Background()

	socketAddress, done := runServiceForTesting(ctx, t)
	defer done()

	client2Destination := "client2-destination"

	client1, done := newClientForTesting(ctx, t, socketAddress)
	defer done()

	client2, done := newClientForTesting(ctx, t, socketAddress)
	defer done()

	// client 2 listens, client 1 sends
	listenerContext, stopListener := context.WithCancel(ctx)
	streams2, err := client2.Listen(listenerContext, &pb.ListenRequest{
		Destination: client2Destination,
	})
	require.NoError(t, err, "failed to call Listen")

	isClient2Listenting := func() bool {
		listDestinationsResponse, err := client1.ListDestinations(ctx, &pb.ListDestinationsRequest{})
		require.NoError(t, err)
		for _, connectedClient := range listDestinationsResponse.Destinations {
			if connectedClient == client2Destination {
				return true
			}
		}
		return false
	}

	delay := 0
	for !isClient2Listenting() {
		delay++
		time.Sleep(time.Second)
		require.Less(t, delay, 2, "Clients did not connect after 2 seconds")
	}

	body1 := wrapperspb.String("hello from client 1")

	body1Any, err := anypb.New(body1)
	require.NoError(t, err)

	requestId := "request001"
	helloMessage1 := &pb.Message{
		Destination: client2Destination,
		Id:          requestId,
		Body:        body1Any,
	}

	sendMessageResponse, err := client1.SendMessage(ctx, helloMessage1)
	require.NoError(t, err)
	assert.Equal(t, pb.SEND_MESSAGE_RESULT_SEND_MESSAGE_OK, sendMessageResponse.GetResult())

	resp, err := streams2.Recv()
	require.NotEqual(t, err, io.EOF)
	require.NoError(t, err)
	assert.Equal(t, requestId, resp.GetId())
	assert.NotNil(t, resp.GetBody())
	receivedBody1, err := resp.GetBody().UnmarshalNew()
	require.NoError(t, err)
	require.Equal(t, "google.protobuf.StringValue", string(receivedBody1.ProtoReflect().Descriptor().FullName()))
	assert.Equal(t, body1.Value, receivedBody1.(*wrapperspb.StringValue).Value)

	stopListener()
	// Stop the server
	Stop(ctx, socketAddress)
}

func TestPubSub(t *testing.T) {
	// Test that events published by one client can be subscribed by other clients
	// and that they are delivered to only those clients and not other clients who are not subscribed
	ctx := context.Background()

	socketAddress, done := runServiceForTesting(ctx, t)
	defer done()

	client2Destination := "client2-destination"
	client3Destination := "client3-destination"
	client4Destination := "client4-destination"

	client1, done := newClientForTesting(ctx, t, socketAddress)
	defer done()

	client2, done := newClientForTesting(ctx, t, socketAddress)
	defer done()

	client3, done := newClientForTesting(ctx, t, socketAddress)
	defer done()

	client4, done := newClientForTesting(ctx, t, socketAddress)
	defer done()

	testTopic := "test-topic"
	// client 1 published, clients 2 and 3 subscribe 4 does not
	listenerContext, stopListeners := context.WithCancel(ctx)

	streams2, err := client2.Listen(listenerContext, &pb.ListenRequest{
		Destination: client2Destination,
	})
	require.NoError(t, err, "failed to call Listen")

	streams3, err := client3.Listen(listenerContext, &pb.ListenRequest{
		Destination: client3Destination,
	})
	require.NoError(t, err, "failed to call Listen")

	streams4, err := client4.Listen(listenerContext, &pb.ListenRequest{
		Destination: client4Destination,
	})
	require.NoError(t, err, "failed to call Listen")

	areAllClientsListenting := func() bool {
		listDestinationsResponse, err := client1.ListDestinations(ctx, &pb.ListDestinationsRequest{})
		require.NoError(t, err)
		isClient2Listenting := false
		isClient3Listenting := false
		isClient4Listenting := false
		for _, connectedClient := range listDestinationsResponse.Destinations {
			if connectedClient == client2Destination {
				isClient2Listenting = true
			}
			if connectedClient == client3Destination {
				isClient3Listenting = true
			}
			if connectedClient == client4Destination {
				isClient4Listenting = true
			}
		}
		return isClient2Listenting && isClient3Listenting && isClient4Listenting
	}

	delay := 0
	for !areAllClientsListenting() {
		delay++
		time.Sleep(time.Second)
		require.Less(t, delay, 2, "Clients did not connect after 2 seconds")
	}

	subscribeResponse, err := client2.SubscribeToTopic(ctx, &pb.SubscribeToTopicRequest{
		Destination: client2Destination,
		Topic:       testTopic,
	})
	require.NoError(t, err, "failed to subscribe")
	require.Equal(t, pb.SUBSCRIBE_TO_TOPIC_RESULT_SUBSCRIBE_TO_TOPIC_OK, subscribeResponse.GetResult())

	subscribeResponse, err = client3.SubscribeToTopic(ctx, &pb.SubscribeToTopicRequest{
		Destination: client3Destination,
		Topic:       testTopic,
	})
	require.NoError(t, err, "failed to subscribe")
	require.Equal(t, pb.SUBSCRIBE_TO_TOPIC_RESULT_SUBSCRIBE_TO_TOPIC_OK, subscribeResponse.GetResult())

	body1 := wrapperspb.String("hello from client 1")
	body1Any, err := anypb.New(body1)
	require.NoError(t, err)

	eventId := "event001"

	helloEvent1 := &pb.Event{
		Id:    eventId,
		Topic: testTopic,
		Body:  body1Any,
	}

	publishEventResponse, err := client1.PublishEvent(ctx, helloEvent1)
	require.NoError(t, err)
	assert.Equal(t, pb.PUBLISH_EVENT_RESULT_PUBLISH_EVENT_OK, publishEventResponse.GetResult())

	resp, err := streams2.Recv()
	require.NotEqual(t, err, io.EOF)
	require.NoError(t, err)
	assert.Equal(t, eventId, resp.GetEventId())
	assert.NotNil(t, resp.GetBody())

	receivedBody1, err := resp.GetBody().UnmarshalNew()
	require.NoError(t, err)
	require.Equal(t, "google.protobuf.StringValue", string(receivedBody1.ProtoReflect().Descriptor().FullName()))
	assert.Equal(t, body1.Value, receivedBody1.(*wrapperspb.StringValue).Value)

	resp, err = streams3.Recv()
	require.NotEqual(t, err, io.EOF)
	require.NoError(t, err)
	assert.Equal(t, eventId, resp.GetEventId())
	assert.NotNil(t, resp.GetBody())
	receivedBody1, err = resp.GetBody().UnmarshalNew()
	require.NoError(t, err)
	require.Equal(t, "google.protobuf.StringValue", string(receivedBody1.ProtoReflect().Descriptor().FullName()))
	assert.Equal(t, body1.Value, receivedBody1.(*wrapperspb.StringValue).Value)

	go func() {
		_, err = streams4.Recv()
		assert.Contains(t, err.Error(), "context canceled")
	}()

	//wait for a second to ensure that client 4 does not receive the message
	time.Sleep(time.Second)

	stopListeners()
	// Stop the server
	Stop(ctx, socketAddress)
}
