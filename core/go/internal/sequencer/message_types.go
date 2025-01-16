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

package sequencer

import (
	"encoding/json"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

const (
	MessageType_DelegationRequest                = "DelegationRequest"
	MessageType_HandoverRequest                  = "HandoverRequest"
	MessageType_CoordinatorHeartbeatNotification = "CoordinatorHeartbeatNotification"
)

type CoordinatorHeartbeatNotification struct {
	From                   string                   `json:"from"`
	ContractAddress        *tktypes.EthAddress      `json:"contractAddress"`
	FlushPoints            []*FlushPoint            `json:"flushPoints"`
	DispatchedTransactions []*DispatchedTransaction `json:"dispatchedTransactions"`
	PooledTransactions     []*PooledTransaction     `json:"pooledTransactions"`
	ConfirmedTransactions  []*ConfirmedTransaction  `json:"confirmedTransactions"`
	CoordinatorState       CoordinatorState         `json:"coordinatorState"`
	BlockHeight            uint64                   `json:"blockHeight"`
}

func (chn *CoordinatorHeartbeatNotification) bytes() []byte {
	jsonBytes, _ := json.Marshal(chn)
	return jsonBytes
}

func ParseCoordinatorHeartbeatNotification(bytes []byte) (*CoordinatorHeartbeatNotification, error) {
	chn := &CoordinatorHeartbeatNotification{}
	err := json.Unmarshal(bytes, chn)
	return chn, err
}

type DelegationRequest struct {
	Sender          string                           `json:"sender"` //TODO this is duplicate of the ReplyTo field in the transport message.  Would it be more secure to assert that they are the same?
	ContractAddress *tktypes.EthAddress              `json:"contractAddress"`
	Transactions    []*components.PrivateTransaction `json:"transactions"`
}

type HandoverRequest struct {
	ContractAddress *tktypes.EthAddress `json:"contractAddress"`
}

func (dr *HandoverRequest) bytes() []byte {
	jsonBytes, _ := json.Marshal(dr)
	return jsonBytes
}

func (dr *DelegationRequest) bytes() []byte {
	jsonBytes, _ := json.Marshal(dr)
	return jsonBytes
}

func ParseDelegationRequest(bytes []byte) (*DelegationRequest, error) {
	dr := &DelegationRequest{}
	err := json.Unmarshal(bytes, dr)
	return dr, err
}
