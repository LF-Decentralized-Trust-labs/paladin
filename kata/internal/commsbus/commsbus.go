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

// Package commsbus provides a message broker that facilitates communication between different components.
// It allows sending messages to specific destinations or broadcasting messages to all destinations subscribed to a topic.
// [./doc/commsbus.png]
// This package implements the Broker interface, which defines the methods for sending messages, publishing events,
// listening for messages, unsubscribing from topics, and listing destinations.
//
// The Broker interface:
// - Listen: Listens for messages sent to a specific destination.
// - SendMessage: Sends a message to a specific destination.
// - SubscribeToTopic: Subscribes a destination to a specific topic.
// - PublishEvent: Publishes an event to all destinations subscribed to a topic.
// - Unlisten: Stops listening for messages sent to a specific destination.
// - UnsubscribeFromTopic: Unsubscribes a destination from a specific topic.
// - ListDestinations: Lists all destinations currently registered with the broker.
//
// The broker implementation in this package uses a map to store the registered destinations and their corresponding message handlers.
// It also maintains a map of topic subscriptions to efficiently route events to the subscribed destinations.
//
// The BrokerConfig struct is currently empty, but it can be extended in the future to provide configuration options for the broker.
//
// The Message struct represents a message that can be sent to a destination. It contains information such as the destination,
// message body, reply-to destination, ID, correlation ID, and message type.
//
// The Event struct represents an event that can be published to a topic. It contains information such as the topic, event body,
// event type, ID, and correlation ID.
//
// The EventMessage struct is a wrapper struct that combines an event with a destination, allowing it to be sent as a message
// to a named listener.
//
// The MessageHandler struct represents a message handler that can be used to receive messages sent to a specific destination.
// It contains a channel where the messages are received.
//
// The broker struct is the actual implementation of the Broker interface. It manages the registered destinations, topic subscriptions,
// and handles the sending and routing of messages and events.
//
// The newBroker function is a constructor function that creates a new instance of the broker.

package commsbus

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

type Config struct {
	Broker BrokerConfig `yaml:"broker"`
	GRPC   GRPCConfig   `yaml:"grpc"`
}

type CommsBus interface {
	Broker() Broker
	GRPCServer() GRPCServer
}

func NewCommsBus(ctx context.Context, conf *Config) (CommsBus, error) {

	if conf == nil {
		log.L(ctx).Error("Missing comms bus config")
		return nil, i18n.NewError(ctx, msgs.MsgConfigFileMissingMandatoryValue, "commsBus")
	}

	broker, err := newBroker()
	if err != nil {
		log.L(ctx).Error("Failed to create broker", err)
		return nil, err
	}

	grpcServer, err := newGRPCServer(ctx, broker, &conf.GRPC)
	if err != nil {
		log.L(ctx).Errorf("Failed to create grpc server: %s", err)
		return nil, err
	}

	//TODO is this the best time to start the server? As a side effect of creating it?
	go func() {
		grpcServerError := grpcServer.Run(ctx)
		if grpcServerError != nil {
			log.L(ctx).Error("Failed to create run server", err)
		}
	}()
	return &commsBus{
		broker,
		grpcServer,
	}, nil
}

type commsBus struct {
	broker     Broker
	grpcServer GRPCServer
}

func (c *commsBus) Broker() Broker {
	return c.broker
}
func (c *commsBus) GRPCServer() GRPCServer {
	return c.grpcServer
}
