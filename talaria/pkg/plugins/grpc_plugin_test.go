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
	"log"
	"io"
	"testing"
	"sync"
	"fmt"

	pluginInterfaceProto "github.com/kaleido-io/talaria/pkg/talaria/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"github.com/stretchr/testify/assert"
)

func getTestGRPCPlugin(ctx context.Context) *GRPCTransportPlugin {
	plugin := NewGRPCTransportPlugin(10001)
	plugin.Start(ctx)
	return plugin
}

func TestMessageFlowSingleMessage(t *testing.T) {
	// To show messaging working from one end to another, we're going ot make the plugin make a connection
	// to itself on localhost and then vertify that we're able to see the message coming through to the channel
	//
	// Essentially this test is pretending that it's Talaria
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gp := getTestGRPCPlugin(ctx)

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
	go func(){
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
	go func(){
		for i := 0; i < 10; i++ {
			req := &pluginInterfaceProto.PaladinMessage{
				Payload: []byte("Hello, World!"),
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

func TestGetRegistration(t *testing.T){
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gp := getTestGRPCPlugin(ctx)

	reg := gp.GetRegistration()
	assert.NotNil(t, reg.Name)
	assert.NotNil(t, reg.SocketLocation)
}