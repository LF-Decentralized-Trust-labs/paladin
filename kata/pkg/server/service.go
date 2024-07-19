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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"google.golang.org/grpc/metadata"
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

// OpenStreams implements the OpenStreams RPC method of KataService which is the main entry point for bidirectional communication
// between plugins and kata. It receives a stream of messages and sends a stream of messages.
// The body and type of the messages control routing to specific functions within the kata and its plugins.
func (s *KataMessageService) OpenStreams(stream proto.KataMessageService_OpenStreamsServer) error {
	ctx := stream.Context()
	log.L(ctx).Info("OpenStreams")
	//defaulting to an ephemeral session which means a new destination ID is generated for each stream
	destinationID := uuid.New().String()

	// allow the client to specify a destination ID in metadata so that they can resume a session if needed
	if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
		log.L(ctx).Debug("got metadata")

		if val, ok := md["destination"]; ok {
			log.L(ctx).Debug("got destination from metadata", val[0])

			destinationID = val[0]
		} else {
			log.L(ctx).Debug("metadata does not contain destination")

		}
	} else {
		log.L(ctx).Debug("no metadata")

	}
	log.L(ctx).Infof("Destination ID: %s", destinationID)

	messageHandler, err := s.messageBroker.Listen(ctx, destinationID)
	if err != nil {
		log.L(ctx).Errorf("Failed to listen for messages: %s", err)
		return err
	}
	// Start a goroutine to handle incoming messages from the internal comms bus and forward them to the client
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
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
	}()
	for {
		// Read the next message from the stream and forward it to the internal comms bus
		msg, err := stream.Recv()
		if err == io.EOF {
			// End of stream, exit the loop
			log.L(ctx).Info("EOF")
			break
		}
		if err != nil {
			log.L(ctx).Error("Error from stream", err)
			// Handle the error
			return err
		}
		log.L(ctx).Info("Received message")
		commsbusMessage := commsbus.Message{
			Destination:   msg.GetDestination(),
			Body:          []byte(msg.GetBody()),
			ID:            msg.GetId(),
			Type:          msg.GetType(),
			ReplyTo:       &destinationID, // We always set replyto just incase the client wants to send a response back
			CorrelationID: msg.CorrelationId,
		}

		err = s.messageBroker.SendMessage(ctx, commsbusMessage)
		if err != nil {
			log.L(ctx).Error("Error sending message", err)
			// Handle the error
			return err
		}

		// Optionally, you can also check if the client has requested to cancel the stream
		if stream.Context().Err() != nil {
			// Client has canceled the stream, exit the loop
			break
		}
	}

	return nil
}
