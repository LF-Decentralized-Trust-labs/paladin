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

type CoordinatorHeartbeatNotification struct {
	From                   string                   `json:"from"`
	ContractAddress        *tktypes.EthAddress      `json:"contractAddress"`
	DispatchedTransactions []*DispatchedTransaction `json:"dispatchedTransactions"`
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
	From            string                           `json:"from"`
	ContractAddress *tktypes.EthAddress              `json:"contractAddress"`
	Transactions    []*components.PrivateTransaction `json:"transactions"`
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
