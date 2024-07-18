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

func TestRun(t *testing.T) {
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
	client := proto.NewPaladinTransactionServiceClient(conn)
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

	streams, err := client.Listen(ctx)
	require.NoError(t, err, "failed to call Listen")

	requestId := "requestID"
	submitTransactionRequest := &proto.TransactionMessage{
		Type: proto.MESSAGE_TYPE_REQUEST_MESSAGE,
		Id:   requestId,
		Message: &proto.TransactionMessage_Request{
			Request: &proto.TransactionRequest{
				Request: &proto.TransactionRequest_SubmitTransactionRequest{
					SubmitTransactionRequest: &proto.SubmitTransactionRequest{
						From:            "fromID",
						ContractAddress: "contract",
						Payload:         &proto.SubmitTransactionRequest_PayloadJSON{PayloadJSON: `{"foo":"bar"}`},
					},
				},
			},
		},
	}

	err = streams.Send(submitTransactionRequest)
	require.NoError(t, err)

	resp, err := streams.Recv()

	require.NotEqual(t, err, io.EOF)
	require.NoError(t, err)
	assert.Equal(t, requestId, resp.GetResponse().GetRequestId())
	assert.NotNil(t, resp.GetResponse().GetSubmitTransactionResponse().GetTransactionId())
	err = streams.CloseSend()
	require.NoError(t, err)
	resp, err = streams.Recv()
	require.Equal(t, err, io.EOF)
	assert.Nil(t, resp)

	// Stop the server
	Stop(ctx, socketAddress)
}
