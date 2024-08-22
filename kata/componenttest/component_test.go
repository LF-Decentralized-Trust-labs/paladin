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
	_ "embed"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/kata"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	transactionsPB "github.com/kaleido-io/paladin/kata/pkg/proto/transaction"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

//go:embed abis/SimpleStorage.json
var simpleStorageBuildJSON []byte // From "gradle copyTestSolidityBuild"

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

	// TODO: figure out race condition here with listener startup
	time.Sleep(2 * time.Second)

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
	kata.Stop(ctx, socketAddress)
}

func TestRunSimpleStorageEthTransaction(t *testing.T) {
	ctx := context.Background()
	logrus.SetLevel(logrus.DebugLevel)

	// This is a placeholder for when we have the TX engine in place with full
	// JSON/RPC commands on the main Paladin engine to invoke this over HTTP
	type testConfigType struct {
		Persistence persistence.Config  `yaml:"persistence"`
		Eth         ethclient.Config    `yaml:"eth"`
		Indexer     blockindexer.Config `yaml:"indexer"`
		Keys        api.Config          `yaml:"keys"`
	}
	var testConfig testConfigType

	err := yaml.Unmarshal([]byte(`
persistence:
  type: sqlite
  sqlite:
    uri:           ":memory:"
    autoMigrate:   true
    migrationsDir: ../db/migrations/sqlite
    debugQueries:  true
eth:  
    ws:
        url: ws://localhost:8546
        initialConnectAttempts: 25
keys:
    keyDerivation:
      type: bip32
    keyStore:
      type: static
      static:
        keys:
          seed:
            encoding: none
            inline: polar mechanic crouch jungle field room dry sure machine brisk seed bulk student total ethics
`), &testConfig)
	assert.NoError(t, err)

	p, err := persistence.NewPersistence(ctx, &testConfig.Persistence)
	assert.NoError(t, err)
	defer p.Close()

	indexer, err := blockindexer.NewBlockIndexer(ctx, &blockindexer.Config{
		FromBlock: types.RawJSON(`"latest"`), // don't want earlier events
	}, &testConfig.Eth.WS, p)
	assert.NoError(t, err)

	type solBuild struct {
		ABI      abi.ABI                   `json:"abi"`
		Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
	}
	var simpleStorageBuild solBuild
	err = json.Unmarshal(simpleStorageBuildJSON, &simpleStorageBuild)
	assert.NoError(t, err)

	eventStreamEvents := make(chan *blockindexer.EventWithData, 2 /* all the events we exepct */)
	err = indexer.Start(&blockindexer.InternalEventStream{
		Handler: func(ctx context.Context, tx *gorm.DB, batch *blockindexer.EventDeliveryBatch) error {
			// With SQLite we cannot hang in here with a DB TX - as there's only one per process.
			for _, e := range batch.Events {
				select {
				case eventStreamEvents <- e:
				default:
					assert.Fail(t, "more than expected number of events received")
				}
			}
			return nil
		},
		Definition: &blockindexer.EventStream{
			Name: "unittest",
			ABI:  abi.ABI{simpleStorageBuild.ABI.Events()["Changed"]},
		},
	})
	assert.NoError(t, err)
	defer indexer.Stop()

	keyMgr, err := ethclient.NewSimpleTestKeyManager(ctx, &testConfig.Keys)
	assert.NoError(t, err)

	ethClient, err := ethclient.NewEthClient(ctx, keyMgr, &testConfig.Eth)
	assert.NoError(t, err)
	defer ethClient.Close()

	simpleStorage, err := ethClient.ABI(ctx, simpleStorageBuild.ABI)
	assert.NoError(t, err)

	txHash1, err := simpleStorage.MustConstructor(simpleStorageBuild.Bytecode).R(ctx).
		Signer("key1").Input(`{"x":11223344}`).SignAndSend()
	assert.NoError(t, err)
	deployTX, err := indexer.WaitForTransaction(ctx, txHash1.String())
	assert.NoError(t, err)
	contractAddr := deployTX.ContractAddress.Address0xHex()

	getX1, err := simpleStorage.MustFunction("get").R(ctx).To(contractAddr).CallJSON()
	assert.NoError(t, err)
	assert.JSONEq(t, `{"x":"11223344"}`, string(getX1))

	txHash2, err := simpleStorage.MustFunction("set").R(ctx).
		Signer("key1").To(contractAddr).Input(`{"_x":99887766}`).SignAndSend()
	assert.NoError(t, err)
	_, err = indexer.WaitForTransaction(ctx, txHash2.String())
	assert.NoError(t, err)

	getX2, err := simpleStorage.MustFunction("get").R(ctx).To(contractAddr).CallJSON()
	assert.NoError(t, err)
	assert.JSONEq(t, `{"x":"99887766"}`, string(getX2))

	// Expect our event listener to be queued up with two Changed events
	event1 := <-eventStreamEvents
	assert.JSONEq(t, `{"x":"11223344"}`, string(event1.Data))
	event2 := <-eventStreamEvents
	assert.JSONEq(t, `{"x":"99887766"}`, string(event2.Data))

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
    migrationsDir: ../db/migrations/sqlite
    debugQueries:  true
commsBus:  
  grpc:
    socketAddress: ` + socketAddress + `
`)
	_, err = configFile.Write(yamlContent)
	require.NoError(t, err)

	configFile.Close()

	// Start the server
	go kata.Run(ctx, configFile.Name())

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
	kata.Stop(ctx, socketAddress)
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
	kata.Stop(ctx, socketAddress)
}
