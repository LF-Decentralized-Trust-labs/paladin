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
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type TransactionEventListener struct {
	Name    string                           `docstruct:"TransactionEventListener" json:"name"`
	Created tktypes.Timestamp                `docstruct:"TransactionEventListener" json:"created"`
	Started *bool                            `docstruct:"TransactionEventListener" json:"started"`
	Sources []TransactionEventListenerSource `docstruct:"TransactionEventListener" json:"sources"`
	Options TransactionEventListenerOptions  `docstruct:"TransactionEventListener" json:"options"`
}

type TransactionEventListenerOptions struct {
	BatchSize    *int    `docstruct:"TransactionEventListenerOptions" json:"batchSize,omitempty"`
	BatchTimeout *string `docstruct:"TransactionEventListenerOptions" json:"batchTimeout,omitempty"`
}

type TransactionEventListenerSource struct {
	ABI     abi.ABI             `docstruct:"TransactionEventListenerSource" json:"abi"`
	Address *tktypes.EthAddress `docstruct:"TransactionEventListenerSource" json:"address,omitempty"`
}
