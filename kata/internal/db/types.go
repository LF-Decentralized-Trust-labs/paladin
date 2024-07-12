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

package db

import "github.com/hyperledger/firefly-common/pkg/fftypes"

type TransactionStatus int

const (
	TransactionPending TransactionStatus = iota
	TransactionAssembling
	TransactionTracking
)

type Transaction struct {
	ID             *fftypes.UUID      `ffstruct:"Transaction" json:"id"`      // kid of the key
	Created        *fftypes.FFTime    `ffstruct:"Transaction" json:"created"` // time when the key is created
	Updated        *fftypes.FFTime    `ffstruct:"Transaction" json:"updated"` // time when the key is updated
	IdempotencyKey *string            `ffstruct:"Transaction" json:"idempotencyKey"`
	Status         *TransactionStatus `ffstruct:"Transaction" json:"status"`
	StatusMessage  *string            `ffstruct:"Transaction" json:"statusMessage,omitempty"`

	PreReqTxs       *string `ffstruct:"Transaction" json:"preReqTxs,omitempty"`
	From            *string `ffstruct:"Transaction" json:"from" gorm:"column:tx_from"`
	ContractAddress *string `ffstruct:"Transaction" json:"contractAddress" gorm:"column:tx_contract_address"`
	Payload         *string `ffstruct:"Transaction" json:"payload" gorm:"column:tx_payload"`

	AssembledPreReqTxs     *string `ffstruct:"Transaction" json:"assembledPreReqTxs,omitempty" gorm:"-"`
	AssembledPayload       *string `ffstruct:"Transaction" json:"assembledPayload,omitempty" gorm:"-"`
	AssembledInputStates   *string `ffstruct:"Transaction" json:"assembledInputStates,omitempty" gorm:"-"`
	AssembledOutputStates  *string `ffstruct:"Transaction" json:"assembledOutputStates,omitempty" gorm:"-"`
	ConfirmationTrackingId *string `ffstruct:"Transaction" json:"confirmationTrackingId,omitempty" gorm:"-"`
}

func (t *Transaction) GetID() string {
	return t.ID.String()
}

func (t *Transaction) SetCreated(tm *fftypes.FFTime) {
	t.Created = tm
}

func (t *Transaction) SetUpdated(tm *fftypes.FFTime) {
	t.Updated = tm
}
