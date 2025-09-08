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

package pldapi

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// These are user-supplied directly on the external interface (vs. calculated)
// If set these affect the submission of the public transaction.
// All are optional
type PublicTxOptions struct {
	Gas                *pldtypes.HexUint64  `docstruct:"PublicTxOptions" json:"gas,omitempty"`
	Value              *pldtypes.HexUint256 `docstruct:"PublicTxOptions" json:"value,omitempty"`
	PublicTxGasPricing                      // fixed when any of these are supplied - disabling the gas pricing engine for this TX
}

type PublicCallOptions struct {
	Block pldtypes.HexUint64OrString `docstruct:"PublicCallOptions" json:"block,omitempty"` // a number, or special strings like "latest"
}

type PublicTxGasPricing struct {
	MaxPriorityFeePerGas *pldtypes.HexUint256 `docstruct:"PublicTxGasPricing" json:"maxPriorityFeePerGas,omitempty"`
	MaxFeePerGas         *pldtypes.HexUint256 `docstruct:"PublicTxGasPricing" json:"maxFeePerGas,omitempty"`
	GasPrice             *pldtypes.HexUint256 `docstruct:"PublicTxGasPricing" json:"gasPrice,omitempty"`
}

type PublicTxInput struct {
	From *pldtypes.EthAddress `docstruct:"PublicTxInput" json:"from"`           // resolved signing account
	To   *pldtypes.EthAddress `docstruct:"PublicTxInput" json:"to,omitempty"`   // target contract address, or nil for deploy
	Data pldtypes.HexBytes    `docstruct:"PublicTxInput" json:"data,omitempty"` // the pre-encoded calldata
	PublicTxOptions
}

type PublicTxSubmission struct {
	From  pldtypes.EthAddress `docstruct:"PublicTxSubmission" json:"from"`
	Nonce pldtypes.HexUint64  `docstruct:"PublicTxSubmission" json:"nonce"`
	PublicTxSubmissionData
}

type PublicTxSubmissionData struct {
	Time            pldtypes.Timestamp `docstruct:"PublicTxSubmissionData" json:"time"`
	TransactionHash pldtypes.Bytes32   `docstruct:"PublicTxSubmissionData" json:"transactionHash"`
	PublicTxGasPricing
}

type PublicTx struct {
	LocalID         *uint64                     `docstruct:"PublicTx" json:"localId,omitempty"` // only a local DB identifier for the public transaction. Not directly related to nonce order
	To              *pldtypes.EthAddress        `docstruct:"PublicTx" json:"to,omitempty"`
	Data            pldtypes.HexBytes           `docstruct:"PublicTx" json:"data,omitempty"`
	From            pldtypes.EthAddress         `docstruct:"PublicTx" json:"from"`
	Nonce           *pldtypes.HexUint64         `docstruct:"PublicTx" json:"nonce"`
	Created         pldtypes.Timestamp          `docstruct:"PublicTx" json:"created"`
	CompletedAt     *pldtypes.Timestamp         `docstruct:"PublicTx" json:"completedAt,omitempty"` // only once confirmed
	TransactionHash *pldtypes.Bytes32           `docstruct:"PublicTx" json:"transactionHash"`       // only once confirmed
	Success         *bool                       `docstruct:"PublicTx" json:"success,omitempty"`     // only once confirmed
	RevertData      pldtypes.HexBytes           `docstruct:"PublicTx" json:"revertData,omitempty"`  // only once confirmed, if available
	Submissions     []*PublicTxSubmissionData   `docstruct:"PublicTx" json:"submissions,omitempty"`
	Activity        []TransactionActivityRecord `docstruct:"PublicTx" json:"activity,omitempty"`
	PublicTxOptions
}

type PublicTxBinding struct {
	Transaction     uuid.UUID                      `docstruct:"PublicTxBinding" json:"transaction"`
	TransactionType pldtypes.Enum[TransactionType] `docstruct:"PublicTxBinding" json:"transactionType"`
}
type PublicTxWithBinding struct {
	*PublicTx
	PublicTxBinding
}
