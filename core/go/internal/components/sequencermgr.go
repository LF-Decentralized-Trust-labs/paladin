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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type PrivateTxEndorsementStatus struct {
	Party               string `json:"party"`
	RequestTime         string `json:"requestTime,omitempty"`
	EndorsementReceived bool   `json:"endorsementReceived"`
}

type PrivateTxStatus struct {
	TxID           string                       `json:"transactionId"`
	Status         string                       `json:"status"`
	LatestEvent    string                       `json:"latestEvent"`
	LatestError    string                       `json:"latestError"`
	Endorsements   []PrivateTxEndorsementStatus `json:"endorsements"`
	Transaction    *PrivateTransaction          `json:"transaction,omitempty"`
	FailureMessage string                       `json:"failureMessage,omitempty"`
}

type StateDistribution struct {
	StateID               string  `json:"stateId"`
	IdentityLocator       string  `json:"identityLocator"`
	Domain                string  `json:"domain"`
	ContractAddress       string  `json:"contractAddress"`
	SchemaID              string  `json:"schemaId"`
	NullifierAlgorithm    *string `json:"nullifierAlgorithm,omitempty"`
	NullifierVerifierType *string `json:"nullifierVerifierType,omitempty"`
	NullifierPayloadType  *string `json:"nullifierPayloadType,omitempty"`
}

type StateDistributionSet struct {
	LocalNode  string
	SenderNode string
	Remote     []*StateDistributionWithData
	Local      []*StateDistributionWithData
}

// A StateDistributionWithData is an intent to send private data for a given state to a remote party
type StateDistributionWithData struct {
	StateDistribution
	StateData pldtypes.RawJSON `json:"stateData"`
}

type SequencerManager interface {
	ManagerLifecycle
	TransportClient

	//Synchronous functions to submit a new private transaction
	HandleNewTx(ctx context.Context, dbTX persistence.DBTX, tx *ValidatedTransaction) error
	HandleNewEvent(ctx context.Context, event string) error
	HandleTransactionCollected(ctx context.Context, signerAddress string, contractAddress string, txID uuid.UUID) error
	HandleNonceAssigned(ctx context.Context, from string, nonce uint64, contractAddress string, txID uuid.UUID) error
	HandlePublicTXSubmission(ctx context.Context, signerAddress string, txHash *pldtypes.Bytes32, contractAddress string, txnID uuid.UUID) error
	HandleTransactionConfirmed(ctx context.Context, transactionSender string, receipt *TxCompletion, nonce uint64) error
	// GetTxStatus(ctx context.Context, domainAddress string, txID uuid.UUID) (status PrivateTxStatus, err error)

	// Synchronous function to call an existing deployed smart contract
	CallPrivateSmartContract(ctx context.Context, call *ResolvedTransaction) (*abi.ComponentValue, error)

	// NotifyFailedPublicTx(ctx context.Context, dbTX persistence.DBTX, confirms []*PublicTxMatch) error

	// BuildStateDistributions(ctx context.Context, tx *PrivateTransaction) (*StateDistributionSet, error)
	BuildNullifier(ctx context.Context, kr KeyResolver, s *StateDistributionWithData) (*NullifierUpsert, error)
	BuildNullifiers(ctx context.Context, distributions []*StateDistributionWithData) (nullifiers []*NullifierUpsert, err error)
}
