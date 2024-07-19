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
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

type BrokerConfig struct {
}

type Message struct {
	Destination   string
	Body          []byte
	ReplyTo       *string
	ID            string
	CorrelationID *string
	Type          string
}

type Event struct {
	Topic string
	Body  []byte
}

type EventMessage struct {
	Destination string
	Event
}

type MessageHandler struct {
	Channel chan Message
}

// Broker represents a message broker that facilitates communication between different components.
// We assume that each component has a unique destination that it can be identified by.
// For point to point messages the sender componnets know the destinations strings for the components they need to send to.
// For broadcast messages, the sender component knows the topic string and the broker routes the message to all destinations that have subscribed to that topic.

type Broker interface {
	SendMessage(ctx context.Context, message Message) error
	SendEvent(ctx context.Context, event Event) error
	Listen(ctx context.Context, destination string) (MessageHandler, error)
	Unlisten(ctx context.Context, destination string) error
	SubscribeEvent(ctx context.Context, topic string, destination string) (string, error)
	UnsubscribeEvent(ctx context.Context, topic string, destination string) error
}

type broker struct {
	destinations     map[string]MessageHandler
	destinationsLock sync.Mutex
}

func NewBroker(ctx context.Context, conf *BrokerConfig) (Broker, error) {
	return &broker{
		destinations: make(map[string]MessageHandler),
	}, nil
}

// Listen implements Broker.
func (b *broker) Listen(ctx context.Context, destination string) (MessageHandler, error) {
	handler := MessageHandler{
		Channel: make(chan Message, 1),
	}
	b.destinationsLock.Lock()
	b.destinations[destination] = handler
	b.destinationsLock.Unlock()
	return handler, nil
}

// Unlisten implements Broker.
func (b *broker) Unlisten(ctx context.Context, destination string) error {
	b.destinationsLock.Lock()
	_, ok := b.destinations[destination]
	defer b.destinationsLock.Unlock()
	if !ok {
		return i18n.NewError(ctx, msgs.MsgDestinationNotFound, destination)
	}
	delete(b.destinations, destination)
	return nil
}

// SendMessage implements Broker.
func (b *broker) SendMessage(ctx context.Context, message Message) error {
	log.L(ctx).Infof("SendMessage: %s", message.Destination)

	b.destinationsLock.Lock()
	handler, ok := b.destinations[message.Destination]
	b.destinationsLock.Unlock()
	if !ok {
		return i18n.NewError(ctx, msgs.MsgDestinationNotFound, message.Destination)
	}
	select {
	case handler.Channel <- message:
	case <-time.After(time.Second):
		log.L(ctx).Error("Timed out")
		return i18n.NewError(ctx, msgs.MsgHandlerError)
	}

	return nil
}

// SubscribeEvent implements Broker.
func (b *broker) SubscribeEvent(ctx context.Context, topic string, destination string) (string, error) {
	panic("unimplemented")
}

// SendEvent implements Broker.
func (b *broker) SendEvent(ctx context.Context, event Event) error {
	panic("unimplemented")
}

// UnsubscribeEvent implements Broker.
func (b *broker) UnsubscribeEvent(ctx context.Context, topic string, destination string) error {
	panic("unimplemented")
}
