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

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

func NewKataMessageService(ctx context.Context, messageBroker commsbus.Broker) *KataMessageService {
	return &KataMessageService{
		messageBroker: messageBroker,
	}
}

type KataMessageService struct {
	proto.UnimplementedKataMessageServiceServer
	messageBroker commsbus.Broker
}

func (s *KataMessageService) Status(ctx context.Context, req *proto.StatusRequest) (*proto.StatusResponse, error) {
	log.L(ctx).Info("Status")

	return &proto.StatusResponse{
		Ok: true,
	}, nil
}

func (s *KataMessageService) ListDestinations(ctx context.Context, req *proto.ListDestinationsRequest) (*proto.ListDestinationsResponse, error) {
	log.L(ctx).Info("ListDestinations")

	destinations, err := s.messageBroker.ListDestinations(ctx)
	if err != nil {
		log.L(ctx).Errorf("Failed to list destinations: %s", err)
		return nil, err
	}

	return &proto.ListDestinationsResponse{
		Destinations: destinations,
	}, nil
}

func (s *KataMessageService) SendMessage(ctx context.Context, msg *proto.Message) (*proto.SendMessageResponse, error) {

	log.L(ctx).Info("SendMessage")
	var replyDestinationPtr *string
	replyDestination := msg.GetReplyTo()
	if replyDestination != "" {
		//protobuf does not differentiate between an optional field being "" vs being missing
		replyDestinationPtr = &replyDestination
	}

	commsbusMessage := commsbus.Message{
		Destination:   msg.GetDestination(),
		Body:          []byte(msg.GetBody()),
		ID:            msg.GetId(),
		Type:          msg.GetType(),
		ReplyTo:       replyDestinationPtr,
		CorrelationID: msg.CorrelationId,
	}

	err := s.messageBroker.SendMessage(ctx, commsbusMessage)
	if err != nil {
		log.L(ctx).Error("Error sending message", err)
		// Handle the error
		return nil, err
	}
	return &proto.SendMessageResponse{
		Result: proto.SEND_MESSAGE_RESULT_SEND_MESSAGE_OK,
	}, nil
}

func (s *KataMessageService) PublishEvent(ctx context.Context, event *proto.Event) (*proto.PublishEventResponse, error) {
	log.L(ctx).Info("PublishEvent")
	commsbusEvent := commsbus.Event{
		Topic: event.GetTopic(),
		Body:  []byte(event.GetBody()),
		Type:  event.GetType(),
	}

	err := s.messageBroker.PublishEvent(ctx, commsbusEvent)
	if err != nil {
		log.L(ctx).Error("Error publishing event", err)
		// Handle the error
		return nil, err
	}
	return &proto.PublishEventResponse{
		Result: proto.PUBLISH_EVENT_RESULT_PUBLISH_EVENT_OK,
	}, nil
}

// Listen implements the Listen RPC method of KataService which is the main entry point for sending messages to plugins
func (s *KataMessageService) Listen(listenRequest *proto.ListenRequest, stream proto.KataMessageService_ListenServer) error {
	//TODO validate
	ctx := stream.Context()
	log.L(ctx).Info("Listen")
	destinationID := listenRequest.GetDestination()
	//TODO validate destinationID is unique and send an error message on the stream and exit if not
	log.L(ctx).Infof("Destination ID: %s", destinationID)

	messageHandler, err := s.messageBroker.Listen(ctx, destinationID)
	if err != nil {
		log.L(ctx).Errorf("Failed to listen for messages: %s", err)
		return err
	}
	// handle incoming messages from the internal comms bus and forward them to the client
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-messageHandler.Channel:
			log.L(ctx).Info("Received message from comms bus to forward to client")

			response := &proto.Message{
				Id:            msg.ID,
				Type:          msg.Type,
				Body:          string(msg.Body),
				CorrelationId: msg.CorrelationID,
			}
			err := stream.Send(response)
			if err != nil {
				log.L(ctx).Error("Error sending message", err)
			}
		}
	}
}
