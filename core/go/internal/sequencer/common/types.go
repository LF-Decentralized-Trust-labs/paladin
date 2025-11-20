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
package common

import (
	"fmt"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type FlushPoint struct {
	From          pldtypes.EthAddress
	Nonce         uint64
	TransactionID uuid.UUID
	Hash          pldtypes.Bytes32
	Confirmed     bool
}

func (f *FlushPoint) GetSignerNonce() string {
	return fmt.Sprintf("%s:%d", f.From.String(), f.Nonce)
}

type CoordinatorSnapshot struct {
	FlushPoints            []*FlushPoint            `json:"flushPoints"`
	DispatchedTransactions []*DispatchedTransaction `json:"dispatchedTransactions"`
	PooledTransactions     []*Transaction           `json:"pooledTransactions"`
	ConfirmedTransactions  []*ConfirmedTransaction  `json:"confirmedTransactions"`
	CoordinatorState       CoordinatorState         `json:"coordinatorState"`
	BlockHeight            uint64                   `json:"blockHeight"`
}

type CoordinatorState string

const (
	CoordinatorState_Idle      CoordinatorState = "Idle"
	CoordinatorState_Observing CoordinatorState = "Observing"
	CoordinatorState_Standby   CoordinatorState = "Standby"
	CoordinatorState_Elect     CoordinatorState = "Elect"
	CoordinatorState_Prepared  CoordinatorState = "Prepared"
	CoordinatorState_Active    CoordinatorState = "Active"
	CoordinatorState_Flush     CoordinatorState = "Flush"
	CoordinatorState_Closing   CoordinatorState = "Closing"
)

type Transaction struct {
	//components.PrivateTransaction
	ID         uuid.UUID
	Originator string
}

func (t *Transaction) GetID() string {
	return t.ID.String()
}

type DispatchedTransaction struct {
	Transaction
	SignerLocator        string
	Signer               pldtypes.EthAddress
	LatestSubmissionHash *pldtypes.Bytes32
	Nonce                *uint64
}

type ConfirmedTransaction struct {
	DispatchedTransaction
	Hash         pldtypes.Bytes32
	RevertReason pldtypes.HexBytes
}
