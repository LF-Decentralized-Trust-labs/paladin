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

// Package commsbus provides functionality for managing communication between internal kata components and plugins.
// It includes a broker that acts as a central hub for routing messages and events between different components of the system.
// Messages have a body and a destination. The broker routes messages to the appropriate handler based on the destination.
// Events have a body and a topic abd are broadcast to all handlers that have subscribed to that topic.
// Messages may include a reply-to destination, which is used to route responses back to the original sender.

package commsbus

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroker_SendMessageOK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := NewBroker(ctx, &BrokerConfig{})
	require.NoError(t, err)

	// Create a channel to signal test completion
	completionChan := make(chan error)

	// and set it up to time out with errror after 5 seconds
	go func() {
		// Simulate a timeout by sending an error to the channel after 5 seconds
		time.Sleep(5 * time.Second)
		// Send an error to the response channel
		completionChan <- fmt.Errorf("Timed out")
	}()

	handler, err := testBroker.Listen(ctx, "test.destination")
	require.NoError(t, err)

	// spin up a thread to listen for messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-handler.Channel:
				assert.Equal(t, "test message body", string(message.Body))
				completionChan <- nil
				return
			}
		}
	}()

	// Create a test message
	message := Message{
		Destination: "test.destination",
		Body:        []byte("test message body"),
		ReplyTo:     nil,
	}

	// Call the SendMessage method
	err = testBroker.SendMessage(ctx, message)
	require.NoError(t, err)

	// Read the response from the channel
	response := <-completionChan
	assert.NoError(t, response)

}

func TestBroker_SendMessageHandlerTimeout(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := NewBroker(ctx, &BrokerConfig{})
	require.NoError(t, err)

	_, err = testBroker.Listen(ctx, "test.destination")
	require.NoError(t, err)

	//NOTE we do no spin up a thread to listen for messages so the message delivery will time out

	// Create a test message
	message := Message{
		Destination: "test.destination",
		Body:        []byte("test message body"),
		ReplyTo:     nil,
	}

	// Call the SendMessage method twice to fill up the buffer
	err = testBroker.SendMessage(ctx, message)
	assert.NoError(t, err)

	err = testBroker.SendMessage(ctx, message)
	require.Error(t, err)
	// TODO this behaviour gives us unreliable delivery.  We should consider retries and dead letter queues?
	assert.Contains(t, err.Error(), "PD010601")

}

func TestBroker_Unlisten(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := NewBroker(ctx, &BrokerConfig{})
	require.NoError(t, err)

	handler, err := testBroker.Listen(ctx, "test.destination")
	require.NoError(t, err)

	err = testBroker.Unlisten(ctx, "test.destination")
	require.NoError(t, err)

	// spin up a thread to listen for messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-handler.Channel:
				assert.Fail(t, "Message received after unlisten")
				return
			}
		}
	}()

	// Create a test message
	message := Message{
		Destination: "test.destination",
		Body:        []byte("test message body"),
		ReplyTo:     nil,
	}

	// Call the SendMessage method
	err = testBroker.SendMessage(ctx, message)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PD010600")

}

func TestBroker_ListDestinationsOK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := NewBroker(ctx, &BrokerConfig{})
	require.NoError(t, err)

	_, err = testBroker.Listen(ctx, "test.destination.1")
	require.NoError(t, err)

	_, err = testBroker.Listen(ctx, "test.destination.2")
	require.NoError(t, err)

	destinations, err := testBroker.ListDestinations(ctx)
	require.NoError(t, err)
	assert.Contains(t, destinations, "test.destination.1")
	assert.Contains(t, destinations, "test.destination.2")

}
