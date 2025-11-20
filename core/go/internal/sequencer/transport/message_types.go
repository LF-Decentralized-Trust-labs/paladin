/*
 * Copyright © 2025 Kaleido, Inc.
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

package transport

import (
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

const (
	MessageType_AssembleRequest                  = "AssembleRequest"
	MessageType_AssembleResponse                 = "AssembleResponse"
	MessageType_AssembleError                    = "AssembleError"
	MessageType_CoordinatorHeartbeatNotification = "CoordinatorHeartbeatNotification"
	MessageType_DelegationRequest                = "DelegationRequest"
	MessageType_DelegationRequestAcknowledgment  = "DelegationRequestAcknowledgment"
	MessageType_Dispatched                       = "Dispatched"
	MessageType_EndorsementRequest               = "EndorsementRequest"
	MessageType_EndorsementResponse              = "EndorsementResponse"
	MessageType_HandoverRequest                  = "HandoverRequest"
	MessageType_NonceAssigned                    = "NonceAssigned"
	MessageType_PreDispatchRequest               = "PreDispatchRequest"
	MessageType_PreDispatchResponse              = "PreDispatchResponse"
	MessageType_TransactionRequest               = "TransactionRequest"
	MessageType_TransactionSubmitted             = "TransactionSubmitted"
	MessageType_TransactionConfirmed             = "TransactionConfirmed"
)

type CoordinatorHeartbeatNotification struct {
	From            string               `json:"from"`
	ContractAddress *pldtypes.EthAddress `json:"contractAddress"`
	common.CoordinatorSnapshot
}

type EndorsementResponse struct {
	TransactionID          uuid.UUID                  `json:"transactionId"`
	IdempotencyKey         string                     `json:"idempotencyKey"`
	ContractAddress        string                     `json:"contractAddress"`
	Endorsement            *prototk.AttestationResult `json:"endorsement"`
	RevertReason           *string                    `json:"revertReason"`
	Party                  string                     `json:"party"`
	AttestationRequestName string                     `json:"attestationRequestName"`
}

func ParseCoordinatorHeartbeatNotification(bytes []byte) (*CoordinatorHeartbeatNotification, error) {
	chn := &CoordinatorHeartbeatNotification{}
	err := json.Unmarshal(bytes, chn)
	return chn, err
}

type TransactionRequest struct {
	Sender          string                           `json:"sender"` //TODO this is duplicate of the ReplyTo field in the transport message.  Would it be more secure to assert that they are the same?
	ContractAddress *pldtypes.EthAddress             `json:"contractAddress"`
	Transactions    []*components.PrivateTransaction `json:"transactions"`
}

type HandoverRequest struct {
	ContractAddress *pldtypes.EthAddress `json:"contractAddress"`
}

type DispatchConfirmationRequest struct {
	ContractAddress *pldtypes.EthAddress `json:"contractAddress"`
	Coordinator     string               `json:"coordinator"`
	TransactionID   uuid.UUID            `json:"transactionID"`
	TransactionHash []byte               `json:"transactionHash"`
}

type DispatchConfirmationResponse struct {
	ContractAddress *pldtypes.EthAddress `json:"contractAddress"`
	TransactionID   uuid.UUID            `json:"transactionID"`
}

func ParseTransactionRequest(bytes []byte) (*TransactionRequest, error) {
	dr := &TransactionRequest{}
	err := json.Unmarshal(bytes, dr)
	return dr, err
}

func ParseDispatchConfirmationRequest(bytes []byte) (*DispatchConfirmationRequest, error) {
	dcr := &DispatchConfirmationRequest{}
	err := json.Unmarshal(bytes, dcr)
	return dcr, err
}

func ParseDispatchConfirmationResponse(bytes []byte) (*DispatchConfirmationResponse, error) {
	dcr := &DispatchConfirmationResponse{}
	err := json.Unmarshal(bytes, dcr)
	return dcr, err
}
