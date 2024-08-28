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

	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBroker_ListenOK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create 2 listeners.  The first listener subscribes for new listener events
	// and then they will see that the 2nd listener has been created

	testBroker, err := newBroker()
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

	listener1Destination := "test.destination.1"
	listener1, err := testBroker.Listen(ctx, listener1Destination)
	require.NoError(t, err)

	err = testBroker.SubscribeToTopic(ctx, TOPIC_NEW_LISTENER, listener1Destination)
	require.NoError(t, err)

	listener2Destination := "test.destination.2"
	_, err = testBroker.Listen(ctx, listener2Destination)
	require.NoError(t, err)

	var getMessageForListener1 func() *pb.NewListenerEvent
	getMessageForListener1 = func() *pb.NewListenerEvent {
		select {
		case receivedEventMessage := <-listener1.Channel:
			require.NotNil(t, receivedEventMessage.Topic)
			assert.Equal(t, "paladin.kata.commsbus.listener.new", *receivedEventMessage.Topic)
			newListenerEvent, ok := receivedEventMessage.Body.(*pb.NewListenerEvent)
			require.True(t, ok)
			//there is a chance, given timing of goroutines that listener 1 actually get the event for itself starting up
			if newListenerEvent.Destination == listener1Destination {
				return getMessageForListener1()
			}
			return newListenerEvent
		case <-completionChan:
			require.Fail(t, "Timed out waiting for event")
			return nil // unreachable because above line will panic
		}
	}

	receivedEvent := getMessageForListener1()
	assert.Equal(t, listener2Destination, receivedEvent.Destination)

}
func TestBroker_SendMessageOK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := newBroker()
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

	testMessage := "test message body"
	strWrapper := wrapperspb.String(testMessage)

	// spin up a thread to listen for messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-handler.Channel:
				require.Equal(t, "google.protobuf.StringValue", string(message.Body.ProtoReflect().Descriptor().FullName()))
				assert.Equal(t, testMessage, message.Body.(*wrapperspb.StringValue).Value)

				completionChan <- nil
				return
			}
		}
	}()

	// Create a test message
	message := Message{
		Destination: "test.destination",
		Body:        strWrapper,
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

	testBroker, err := newBroker()
	require.NoError(t, err)

	_, err = testBroker.Listen(ctx, "test.destination")
	require.NoError(t, err)

	//NOTE we do no spin up a thread to listen for messages so the message delivery will time out

	testMessage := "test message body"
	strWrapper := wrapperspb.String(testMessage)
	// Create a test message
	message := Message{
		Destination: "test.destination",
		Body:        strWrapper,
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

	testBroker, err := newBroker()
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

	testMessage := "test message body"
	strWrapper := wrapperspb.String(testMessage)
	// Create a test message
	message := Message{
		Destination: "test.destination",
		Body:        strWrapper,
		ReplyTo:     nil,
	}

	// Call the SendMessage method
	err = testBroker.SendMessage(ctx, message)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PD010600")
	//wait for a second to make sure the message is not delivered
	time.Sleep(1 * time.Second)

}

func TestBroker_ListDestinationsOK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := newBroker()
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

func TestBroker_SubscribeToTopicsOK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := newBroker()
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

	handler, err := testBroker.Listen(ctx, "test.destination.1")
	require.NoError(t, err)

	testMessage := "test subscribe message body"

	// spin up a thread to listen for messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-handler.Channel:
				require.Equal(t, "google.protobuf.StringValue", string(message.Body.ProtoReflect().Descriptor().FullName()))
				assert.Equal(t, testMessage, message.Body.(*wrapperspb.StringValue).Value)

				completionChan <- nil
				return
			}
		}
	}()

	err = testBroker.SubscribeToTopic(ctx, "test.topic", "test.destination.1")
	require.NoError(t, err)

	event := Event{
		Topic: "test.topic",
		Body:  wrapperspb.String(testMessage),
	}
	// Call the PublishEvent method
	err = testBroker.PublishEvent(ctx, event)
	require.NoError(t, err)

	// Read the response from the channel
	response := <-completionChan
	assert.NoError(t, response)
}

func TestBroker_PublishEventsNoSubscribers(t *testing.T) {
	//TODO
}

func TestBroker_UnSubscribeToTopicsOK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := newBroker()
	require.NoError(t, err)

	handler, err := testBroker.Listen(ctx, "test.destination.1")
	require.NoError(t, err)

	// spin up a thread to listen for messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-handler.Channel:
				assert.Fail(t, "Message received after unsubscribe")
				return
			}
		}
	}()
	err = testBroker.SubscribeToTopic(ctx, "test.topic", "test.destination.1")
	require.NoError(t, err)

	err = testBroker.UnsubscribeFromTopic(ctx, "test.topic", "test.destination.1")
	require.NoError(t, err)

	// Create a test event
	eventBody := "test event body"
	event := Event{
		Topic: "test.topic",
		Body:  wrapperspb.String(eventBody),
	}

	// Call the PublishEvent method
	err = testBroker.PublishEvent(ctx, event)
	require.NoError(t, err)

	//wait for a second to make sure the message is not delivered
	time.Sleep(1 * time.Second)
}

func TestBroker_DoubleSubscribeFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := newBroker()
	require.NoError(t, err)

	_, err = testBroker.Listen(ctx, "test.destination.1")
	require.NoError(t, err)

	err = testBroker.SubscribeToTopic(ctx, "test.topic", "test.destination.1")
	require.NoError(t, err)
	err = testBroker.SubscribeToTopic(ctx, "test.topic", "test.destination.1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD010602")
	assert.Contains(t, err.Error(), "test.topic")
	assert.Contains(t, err.Error(), "test.destination.1")
}

func TestBroker_SubscribeUnknownDestinationFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testBroker, err := newBroker()
	require.NoError(t, err)

	_, err = testBroker.Listen(ctx, "test.destination.1")
	require.NoError(t, err)

	err = testBroker.SubscribeToTopic(ctx, "test.topic", "test.destination.1")
	require.NoError(t, err)
	err = testBroker.SubscribeToTopic(ctx, "test.topic", "test.destination.2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD010600")
	assert.Contains(t, err.Error(), "test.destination.2")
}
