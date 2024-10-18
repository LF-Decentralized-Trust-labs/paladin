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

package components

import (
	"context"

	"gorm.io/gorm"
)

type PrivateTxEventSubscriber func(event PrivateTxEvent)

type PrivateTxEvent interface {
}

type TransactionDispatchedEvent struct {
	TransactionID   string `json:"transactionId"`
	ContractAddress string `json:"contractAddress"`
	Nonce           uint64 `json:"nonce"`
	SigningAddress  string `json:"signingAddress"`
}

type PrivateTxStatus struct {
	TxID        string `json:"transactionId"`
	Status      string `json:"status"`
	LatestEvent string `json:"latestEvent"`
	LatestError string `json:"latestError"`
}

type PrivateTxManager interface {
	ManagerLifecycle
	TransportClient

	//Synchronous functions to submit a new private transaction
	HandleNewTx(ctx context.Context, tx *PrivateTransaction) error
	HandleDeployTx(ctx context.Context, tx *PrivateContractDeploy) error
	GetTxStatus(ctx context.Context, domainAddress string, txID string) (status PrivateTxStatus, err error)

	//TODO this is just a placeholder until we figure out the external interface for events
	// in the meantime, this is handy for some blackish box testing
	Subscribe(ctx context.Context, subscriber PrivateTxEventSubscriber)

	NotifyFailedPublicTx(ctx context.Context, dbTX *gorm.DB, confirms []*PublicTxMatch) error
}
