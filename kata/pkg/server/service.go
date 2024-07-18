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

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/transaction"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

func NewKataMessageService() *KataMessageService {
	return &KataMessageService{}
}

type KataMessageService struct {
	proto.UnimplementedKataMessageServiceServer
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
	log.L(ctx).Info("Listen")
	for {
		// Read the next message from the stream
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
		switch msg.GetType() {
		case proto.MESSAGE_TYPE_RESPONSE_MESSAGE:
			log.L(ctx).Info("Received MESSAGE_TYPE_RESPONSE_MESSAGE")
		case proto.MESSAGE_TYPE_REQUEST_MESSAGE:
			log.L(ctx).Info("Received MESSAGE_TYPE_REQUEST_MESSAGE")
			requestType := msg.GetRequest().GetType()
			requestId := msg.GetId()
			switch requestType {
			case "SUBMIT_TRANSACTION_REQUEST":
				log.L(ctx).Info("Received SUBMIT_TRANSACTION_REQUEST")
				submitTransactionRequest := msg.GetRequest().GetPayload()

				response, err := transaction.Submit(stream.Context(), submitTransactionRequest)
				if err != nil {
					// Handle the error
					return err
				}

				submitTransactionResponse := &proto.Message{
					Type: proto.MESSAGE_TYPE_RESPONSE_MESSAGE,
					Message: &proto.Message_Response{
						Response: &proto.Response{
							Type:      "SUBMIT_TRANSACTION_RESPONSE",
							RequestId: requestId,
							Payload:   response,
						},
					},
				}

				if err := stream.Send(submitTransactionResponse); err != nil {
					log.L(ctx).Error("Error sending submitTransactionResponse", err)
					return err
				}
				log.L(ctx).Info("Sent MESSAGE_TYPE_RESPONSE_MESSAGE")

			default:
				log.L(ctx).Info("Received unkonwn request type")
			}

		default:
			log.L(ctx).Info("Received unkonwn message type")

			// Handle unknown message types
		}

		// Optionally, you can also check if the client has requested to cancel the stream
		if stream.Context().Err() != nil {
			// Client has canceled the stream, exit the loop
			break
		}
	}

	return nil
}
