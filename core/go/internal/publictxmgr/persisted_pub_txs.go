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

package publictxmgr

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// public_transactions
type DBPublicTxn struct {
	PublicTxnID     uint64                 `gorm:"column:pub_txn_id;primaryKey"`
	From            pldtypes.EthAddress    `gorm:"column:from"`
	Nonce           *uint64                `gorm:"column:nonce"`
	Created         pldtypes.Timestamp     `gorm:"column:created;autoCreateTime:nano"`
	To              *pldtypes.EthAddress   `gorm:"column:to"`
	Gas             uint64                 `gorm:"column:gas"`
	FixedGasPricing pldtypes.RawJSON       `gorm:"column:fixed_gas_pricing"`
	Value           *pldtypes.HexUint256   `gorm:"column:value"`
	Data            pldtypes.HexBytes      `gorm:"column:data"`
	Suspended       bool                   `gorm:"column:suspended"`                            // excluded from processing because it's suspended by user
	Completed       *DBPublicTxnCompletion `gorm:"foreignKey:pub_txn_id;references:pub_txn_id"` // excluded from processing because it's done
	Submissions     []*DBPubTxnSubmission  `gorm:"-"`                                           // we do the aggregation, not GORM
	// Binding is used only on queries by transaction (GORM doesn't seem to allow us to define a separate struct for this)
	Binding *DBPublicTxnBinding `gorm:"foreignKey:pub_txn_id;references:pub_txn_id;"`
}

func (DBPublicTxn) TableName() string {
	return "public_txns"
}

type DBPublicTxnBinding struct {
	PublicTxnID     uint64                                `gorm:"column:pub_txn_id;primaryKey"`
	Transaction     uuid.UUID                             `gorm:"column:transaction"`
	TransactionType pldtypes.Enum[pldapi.TransactionType] `gorm:"column:tx_type"`
	Sender          string                                `gorm:"column:sender"`
	ContractAddress string                                `gorm:"column:contract_address"`
}

func (DBPublicTxnBinding) TableName() string {
	return "public_txn_bindings"
}

type DBPubTxnSubmission struct {
	from            string             `gorm:"-"` // just used to ensure we dispatch to same writer as the associated pubic TX
	PublicTxnID     uint64             `gorm:"column:pub_txn_id"`
	Created         pldtypes.Timestamp `gorm:"column:created;autoCreateTime:false"` // we set this as we track the record in memory too
	TransactionHash pldtypes.Bytes32   `gorm:"column:tx_hash;primaryKey"`
	GasPricing      pldtypes.RawJSON   `gorm:"column:gas_pricing"` // no filtering allowed on this field as it's complex JSON gasPrice/maxFeePerGas/maxPriorityFeePerGas calculation
}

func (DBPubTxnSubmission) TableName() string {
	return "public_submissions"
}

type DBPublicTxnCompletion struct {
	PublicTxnID     uint64             `gorm:"column:pub_txn_id;primaryKey"`
	Created         pldtypes.Timestamp `gorm:"column:created;autoCreateTime:nano"`
	TransactionHash pldtypes.Bytes32   `gorm:"column:tx_hash"`
	Success         bool               `gorm:"column:success"`
	RevertData      pldtypes.HexBytes  `gorm:"column:revert_data"` // block indexer does not keep this for all TXs
}

func (DBPublicTxnCompletion) TableName() string {
	return "public_completions"
}

func (s *DBPubTxnSubmission) WriteKey() string {
	// Just use the from address as the write key, so all submissions on the same signing address get batched together
	return s.from
}

type bindingsMatchingSubmission struct {
	DBPublicTxnBinding `gorm:"embedded"`
	Submission         *DBPubTxnSubmission `gorm:"foreignKey:pub_txn_id;references:pub_txn_id;"`
}

type txFromOnly struct {
	From pldtypes.EthAddress
}
