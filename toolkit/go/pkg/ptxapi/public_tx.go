// Copyright © 2024 Kaleido, Inc.
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

package ptxapi

import (
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// These are user-supplied directly on the external interface (vs. calculated)
// If set these affect the submission of the public transaction.
// All are optional
type PublicTxOptions struct {
	Gas                *tktypes.HexUint64  `json:"gas,omitempty"`
	Value              *tktypes.HexUint256 `json:"value,omitempty"`
	PublicTxGasPricing                     // fixed when any of these are supplied - disabling the gas pricing engine for this TX
}

type PublicTxGasPricing struct {
	MaxPriorityFeePerGas *tktypes.HexUint256 `json:"maxPriorityFeePerGas,omitempty"`
	MaxFeePerGas         *tktypes.HexUint256 `json:"maxFeePerGas,omitempty"`
	GasPrice             *tktypes.HexUint256 `json:"gasPrice,omitempty"`
}

type PublicTxInput struct {
	From string              `json:"from"`           // unresolved signing account locator (public TX manager will resolve)
	To   *tktypes.EthAddress `json:"to,omitempty"`   // target contract address, or nil for deploy
	Data tktypes.HexBytes    `json:"data,omitempty"` // the pre-encoded calldata
	PublicTxOptions
}

type PublicTxSubmission struct {
	From  tktypes.EthAddress `json:"from"`
	Nonce tktypes.HexUint64  `json:"nonce"`
	PublicTxSubmissionData
}

type PublicTxSubmissionData struct {
	Time            tktypes.Timestamp `json:"time"`
	TransactionHash tktypes.Bytes32   `json:"transactionHash"`
	PublicTxGasPricing
}

type PublicTx struct {
	To              *tktypes.EthAddress         `json:"to,omitempty"`
	Data            tktypes.HexBytes            `json:"data,omitempty"`
	From            tktypes.EthAddress          `json:"from"`
	Nonce           tktypes.HexUint64           `json:"nonce"`
	Created         tktypes.Timestamp           `json:"created"`
	CompletedAt     *tktypes.Timestamp          `json:"completedAt,omitempty"` // only once confirmed
	TransactionHash *tktypes.Bytes32            `json:"transactionHash"`       // only once confirmed
	Success         *bool                       `json:"success,omitempty"`     // only once confirmed
	RevertData      tktypes.HexBytes            `json:"revertData,omitempty"`  // only once confirmed, if available
	Submissions     []*PublicTxSubmissionData   `json:"submissions,omitempty"`
	Activity        []TransactionActivityRecord `json:"activity,omitempty"`
	PublicTxOptions
}
