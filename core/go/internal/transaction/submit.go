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
package transaction

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/commsbus"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/transactionstore"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	transactionsPB "github.com/kaleido-io/paladin/core/pkg/proto/transaction"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

var ts transactionstore.TransactionStore
var handler commsbus.MessageHandler
var broker commsbus.Broker

const SUBMIT_TRANSACTION_REQUEST = "github.com.kaleido_io.paladin.kata.transaction.SubmitTransactionRequest"
const SUBMIT_TRANSACTION_RESPONSE = "github.com.kaleido_io.paladin.kata.transaction.SubmitTransactionResponse"
const SUBMIT_TRANSACTION_ERROR = "github.com.kaleido_io.paladin.kata.transaction.SubmitTransactionError"
const MESSAGE_DESTINATION = "kata-txn-engine"

func Init(ctx context.Context, persistence persistence.Persistence, commsBus commsbus.CommsBus) error {
	log.L(ctx).Info("Initializing transaction store")
	ts = transactionstore.NewTransactionStore(ctx, &transactionstore.Config{}, persistence)
	var err error
	broker = commsBus.Broker()

	handler, err = broker.Listen(ctx, MESSAGE_DESTINATION)
	if err != nil {
		log.L(ctx).Errorf("Failed to listen for messages: %s", err)
		return err
	}

	go messageHandler(ctx)
	return nil
}

func handleMessage(ctx context.Context, msg commsbus.Message) error {
	log.L(ctx).Infof("Received message: %s", msg.Body)
	msgType := string(msg.Body.ProtoReflect().Descriptor().FullName())
	switch msgType {
	case SUBMIT_TRANSACTION_REQUEST:
		log.L(ctx).Info("Received SUBMIT_TRANSACTION_REQUEST")
		requestId := msg.ID
		submitTransactionRequest := msg.Body.(*transactionsPB.SubmitTransactionRequest)

		response, err := Submit(ctx, submitTransactionRequest)
		if err != nil {
			// Handle the error
			log.L(ctx).Errorf("Error submitting transaction: %s", err)
			return err
		}

		if msg.ReplyTo != nil {
			submitTransactionResponse := commsbus.Message{
				Body:          response,
				CorrelationID: &requestId,
				Destination:   *msg.ReplyTo,
			}

			log.L(ctx).Infof("Sending reply to %s", submitTransactionResponse.Destination)

			err = broker.SendMessage(ctx, submitTransactionResponse)
			if err != nil {
				log.L(ctx).Errorf("Error sending response: %s", err)
				return err
			}
			log.L(ctx).Info("Sent MESSAGE_TYPE_RESPONSE_MESSAGE")
		} else {
			log.L(ctx).Info("No reply requested")

		}

	default:
		log.L(ctx).Infof("Received unknown message type %s", msgType)
		// TODO Handle unknown message types
	}
	return nil
}

func messageHandler(ctx context.Context) {

	for msg := range handler.Channel {
		log.L(ctx).Infof("Received message: %s", msg.Body)
		err := handleMessage(ctx, msg)
		if err != nil {
			log.L(ctx).Errorf("Error handling message: %s", err)
			//TODO error handling
		}
	}
}

func Submit(ctx context.Context, request *transactionsPB.SubmitTransactionRequest) (*transactionsPB.SubmitTransactionResponse, error) {

	payload := request.GetPayloadJSON()
	if payload == "" {
		payload = request.GetPayloadRLP()
	}

	log.L(ctx).Infof("Received SubmitTransactionRequest: contractAddress=%s, from=%s, idempotencyKey=%s, payload=%s", request.GetContractAddress(), request.GetFrom(), request.GetIdempotencyKey(), payload)

	if payload == "" || request.GetFrom() == "" || request.GetContractAddress() == "" {
		missingFields := make([]string, 4)
		if payload == "" {
			missingFields = append(missingFields, "PayloadJSON", "PayloadRLP")
		}
		if request.GetFrom() == "" {
			missingFields = append(missingFields, "From")
		}
		if request.GetContractAddress() == "" {
			missingFields = append(missingFields, "contractAddress")
		}
		return nil, i18n.NewError(ctx, msgs.MsgTransactionMissingField, missingFields)
	}

	payloadJSON := request.GetPayloadJSON()
	payloadRLP := request.GetPayloadRLP()

	createdTransaction, err := ts.InsertTransaction(ctx, transactionstore.Transaction{
		Contract:    request.GetContractAddress(),
		From:        request.GetFrom(),
		PayloadJSON: &payloadJSON,
		PayloadRLP:  &payloadRLP,
	})

	if err != nil {
		log.L(ctx).Errorf("Failed to create transaction: %s", err)
		return nil, err
	}

	response := transactionsPB.SubmitTransactionResponse{
		Id: createdTransaction.ID.String(),
	}

	return &response, nil
}
