// Copyright © 2025 Kaleido, Inc.
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

package pldapi

import (
	"encoding/json"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
)

type BlockchainEventListener struct {
	Name    string                          `docstruct:"BlockchainEventListener" json:"name"`
	Created pldtypes.Timestamp              `docstruct:"BlockchainEventListener" json:"created"`
	Started *bool                           `docstruct:"BlockchainEventListener" json:"started"`
	Sources []BlockchainEventListenerSource `docstruct:"BlockchainEventListener" json:"sources"`
	Options BlockchainEventListenerOptions  `docstruct:"BlockchainEventListener" json:"options"`
}

type BlockchainEventListenerOptions struct {
	BatchSize    *int            `docstruct:"BlockchainEventListenerOptions" json:"batchSize,omitempty"`
	BatchTimeout *string         `docstruct:"BlockchainEventListenerOptions" json:"batchTimeout,omitempty"`
	FromBlock    json.RawMessage `docstruct:"BlockchainEventListenerOptions" json:"fromBlock,omitempty"`
}

type BlockchainEventListenerSource struct {
	ABI     abi.ABI              `docstruct:"BlockchainEventListenerSource" json:"abi"`
	Address *pldtypes.EthAddress `docstruct:"BlockchainEventListenerSource" json:"address,omitempty"`
}

type BlockchainEventListenerStatus struct {
	Catchup    bool                              `docstruct:"BlockchainEventListenerStatus" json:"catchup"`
	Checkpoint BlockchainEventListenerCheckpoint `docstruct:"BlockchainEventListenerStatus" json:"checkpoint"`
}

type BlockchainEventListenerCheckpoint struct {
	BlockNumber int64 `docstruct:"BlockchainEventListenerCheckpoint" json:"blockNumber"`
}
