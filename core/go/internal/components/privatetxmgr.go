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

	"github.com/hyperledger/firefly-signer/pkg/abi"
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

// If we had lots of these we would probably want to centralize the assignment of the constants to avoid duplication
// but currently there is only 2 ( the other being IDENTITY_RESOLVER_DESTINATION )
const PRIVATE_TX_MANAGER_DESTINATION = "private-tx-manager"

type StateDistributionSet struct {
	LocalNode  string
	SenderNode string
	Remote     []*StateDistribution
	Local      []*StateDistribution
}

// A StateDistribution is an intent to send private data for a given state to a remote party
type StateDistribution struct {
	ID                    string
	StateID               string
	IdentityLocator       string
	Domain                string
	ContractAddress       string
	SchemaID              string
	StateDataJson         string
	NullifierAlgorithm    *string
	NullifierVerifierType *string
	NullifierPayloadType  *string
}

type PrivateTxManager interface {
	ManagerLifecycle
	TransportClient

	//Synchronous functions to submit a new private transaction
	HandleNewTx(ctx context.Context, tx *ValidatedTransaction) error
	GetTxStatus(ctx context.Context, domainAddress string, txID string) (status PrivateTxStatus, err error)

	// Synchronous function to call an existing deployed smart contract
	CallPrivateSmartContract(ctx context.Context, call *TransactionInputs) (*abi.ComponentValue, error)

	//TODO this is just a placeholder until we figure out the external interface for events
	// in the meantime, this is handy for some blackish box testing
	Subscribe(ctx context.Context, subscriber PrivateTxEventSubscriber)

	NotifyFailedPublicTx(ctx context.Context, dbTX *gorm.DB, confirms []*PublicTxMatch) error

	PrivateTransactionConfirmed(ctx context.Context, receipt *TxCompletion)

	BuildStateDistributions(ctx context.Context, tx *PrivateTransaction) (*StateDistributionSet, error)
}
