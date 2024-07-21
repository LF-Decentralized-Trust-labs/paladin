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
	"encoding/json"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/persistence"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

var ts transactionstore.TransactionStore
var handler commsbus.MessageHandler
var broker commsbus.Broker

const SUBMIT_TRANSACTION_REQUEST = "SUBMIT_TRANSACTION_REQUEST"
const SUBMIT_TRANSACTION_RESPONSE = "SUBMIT_TRANSACTION_RESPONSE"
const SUBMIT_TRANSACTION_ERROR = "SUBMIT_TRANSACTION_ERROR"
const MESSAGE_DESTINATION = "kata-txn-engine"

func Init(ctx context.Context, persistence persistence.Persistence, messageBroker commsbus.Broker) error {
	log.L(ctx).Info("Initializing transaction store")
	ts = transactionstore.NewTransactionStore(ctx, &transactionstore.Config{}, persistence)
	var err error
	handler, err = messageBroker.Listen(ctx, MESSAGE_DESTINATION)
	if err != nil {
		log.L(ctx).Errorf("Failed to listen for messages: %s", err)
		return err
	}
	broker = messageBroker
	go messageHandler(ctx)
	return nil
}

func handleMessage(ctx context.Context, msg commsbus.Message) error {
	log.L(ctx).Infof("Received message: %s", msg.Body)
	switch msg.Type {
	case SUBMIT_TRANSACTION_REQUEST:
		log.L(ctx).Info("Received SUBMIT_TRANSACTION_REQUEST")
		requestId := msg.ID
		submitTransactionRequest := msg.Body

		response, err := Submit(ctx, string(submitTransactionRequest))
		if err != nil {
			// Handle the error
			log.L(ctx).Errorf("Error submitting transaction: %s", err)
			return err
		}

		if msg.ReplyTo != nil {
			submitTransactionResponse := commsbus.Message{
				Type:          SUBMIT_TRANSACTION_RESPONSE,
				Body:          []byte(response),
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
		log.L(ctx).Info("Received unkonwn message type")
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

func Submit(ctx context.Context, requestJSON string) (string, error) {
	req := struct {
		ContractAddress string `json:"contractAddress"`
		From            string `json:"from"`
		IdempotencyKey  string `json:"idempotencyKey"`
		PayloadJSON     string `json:"payloadJSON"`
		PayloadRLP      string `json:"payloadRLP"`
	}{}
	err := json.Unmarshal([]byte(requestJSON), &req)
	if err != nil {
		return "", i18n.WrapError(ctx, err, msgs.MsgTransactionParseError)
	}

	payload := req.PayloadJSON
	if payload == "" {
		payload = req.PayloadRLP
	}

	log.L(ctx).Infof("Received SubmitTransactionRequest: contractAddress=%s, from=%s, idempotencyKey=%s, payload=%s", req.ContractAddress, req.From, req.IdempotencyKey, payload)

	if payload == "" || req.From == "" || req.ContractAddress == "" {
		missingFields := make([]string, 4)
		if payload == "" {
			missingFields = append(missingFields, "PayloadJSON", "PayloadRLP")
		}
		if req.From == "" {
			missingFields = append(missingFields, "From")
		}
		if req.ContractAddress == "" {
			missingFields = append(missingFields, "payload")
		}
		return "", i18n.NewError(ctx, msgs.MsgTransactionMissingField, missingFields)
	}

	payloadJSON := req.PayloadJSON
	payloadRLP := req.PayloadRLP

	createdTransaction, err := ts.InsertTransaction(ctx, transactionstore.Transaction{
		Contract:    req.ContractAddress,
		From:        req.From,
		PayloadJSON: &payloadJSON,
		PayloadRLP:  &payloadRLP,
	})

	if err != nil {
		log.L(ctx).Errorf("Failed to create transaction: %s", err)
		return "", err
	}

	response := struct {
		TransactionID string `json:"transactionID"`
	}{
		TransactionID: createdTransaction.ID.String(),
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", i18n.WrapError(ctx, err, msgs.MsgTransactionSerializeError)
	}
	return string(responseJSON), nil
}
