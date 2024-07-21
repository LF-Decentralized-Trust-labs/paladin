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
package server

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRunTransactionSubmission(t *testing.T) {
	ctx := context.Background()

	// get a valid file name for a temp file by first creating a temp file and then removing it
	file, err := os.CreateTemp("", "paladin.sock")
	require.NoError(t, err)
	socketAddress := file.Name()
	os.Remove(file.Name())

	//Create and write a config file
	configFile, err := os.CreateTemp("", "test_*.yaml")
	require.NoError(t, err)

	defer os.Remove(configFile.Name())

	// Write YAML content to the temporary file
	yamlContent := []byte(`
persistence:
  type: sqlite
  sqlite:
    uri:           ":memory:"
    autoMigrate:   true
    migrationsDir: ../../db/migrations/sqlite
    debugQueries:  true
grpc:
  socketAddress: ` + socketAddress + `
`)
	_, err = configFile.Write(yamlContent)
	require.NoError(t, err)

	configFile.Close()

	// Start the server
	go Run(ctx, configFile.Name())
	time.Sleep(time.Second * 2)

	// Create a gRPC client connection
	conn, err := grpc.NewClient("unix:"+socketAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	// Create a new instance of the gRPC client
	client := proto.NewKataMessageServiceClient(conn)
	status, err := client.Status(ctx, &proto.StatusRequest{})

	delay := 0
	for !status.GetOk() {
		time.Sleep(time.Second)
		delay++
		status, err = client.Status(ctx, &proto.StatusRequest{})
		require.Less(t, delay, 2, "Server did not start after 2 seconds")
	}
	require.NoError(t, err)
	assert.True(t, status.GetOk())

	testDestination := "test-destination"
	listenerContext, stopListener := context.WithCancel(ctx)
	streams, err := client.Listen(listenerContext, &proto.ListenRequest{
		Destination: testDestination,
	})
	require.NoError(t, err, "failed to call Listen")

	submitTransactionJSON := `
	{
		"from":            "fromID",
		"contractAddress": "contract",
		"payloadJSON":     "{\"foo\":\"bar\"}"
	}
	`
	requestId := "requestID"
	submitTransactionRequest := &proto.Message{
		Destination: "kata-txn-engine",
		Id:          requestId,
		Type:        "SUBMIT_TRANSACTION_REQUEST",
		Body:        submitTransactionJSON,
		ReplyTo:     &testDestination,
	}

	sendMessageResponse, err := client.SendMessage(ctx, submitTransactionRequest)
	require.NoError(t, err)
	require.NotNil(t, sendMessageResponse)
	assert.Equal(t, proto.SEND_MESSAGE_RESULT_SEND_MESSAGE_OK, sendMessageResponse.GetResult())

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

func newClientForTesting(ctx context.Context, t *testing.T, socketAddress string) (proto.KataMessageServiceClient, func()) {
	// Create a gRPC client connection
	conn, err := grpc.NewClient("unix:"+socketAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	// Create a new instance of the gRPC client
	client := proto.NewKataMessageServiceClient(conn)
	status, err := client.Status(ctx, &proto.StatusRequest{})

	delay := 0
	for !status.GetOk() {
		time.Sleep(time.Second)
		delay++
		status, err = client.Status(ctx, &proto.StatusRequest{})
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
	streams2, err := client2.Listen(listenerContext, &proto.ListenRequest{
		Destination: client2Destination,
	})
	require.NoError(t, err, "failed to call Listen")

	isClient2Listenting := func() bool {
		listDestinationsResponse, err := client1.ListDestinations(ctx, &proto.ListDestinationsRequest{})
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

	body1 := "hello from client 1"

	requestId := "request001"
	helloMessage1 := &proto.Message{
		Destination: client2Destination,
		Id:          requestId,
		Type:        "HELLO",
		Body:        body1,
	}

	sendMessageResponse, err := client1.SendMessage(ctx, helloMessage1)
	require.NoError(t, err)
	assert.Equal(t, proto.SEND_MESSAGE_RESULT_SEND_MESSAGE_OK, sendMessageResponse.GetResult())

	resp, err := streams2.Recv()
	require.NotEqual(t, err, io.EOF)
	require.NoError(t, err)
	assert.Equal(t, requestId, resp.GetId())
	assert.NotNil(t, resp.GetBody())
	assert.Equal(t, body1, resp.GetBody())

	stopListener()
	// Stop the server
	Stop(ctx, socketAddress)
}
