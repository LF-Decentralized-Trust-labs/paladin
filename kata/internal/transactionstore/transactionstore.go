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

package transactionstore

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"gorm.io/gorm"
)

type Config struct {
}

type TxStateGetters interface {
	HACKGetPrivateTx() *components.PrivateTransaction
	GetContractAddress(ctx context.Context) string
	GetTxID(ctx context.Context) string
	GetDomainID(ctx context.Context) string
	GetSchemaID(ctx context.Context) string

	GetDispatchTxPayload(ctx context.Context) string
	GetAttestationPlan(ctx context.Context) []*prototk.AttestationRequest
	GetAttestationResults(ctx context.Context) []*prototk.AttestationResult
	GetPayloadJSON(ctx context.Context) string

	IsAttestationCompleted(ctx context.Context) bool

	GetAssembledRound(ctx context.Context) int64

	GetPreReqTransactions(ctx context.Context) []string
	GetDispatchAddress(ctx context.Context) string
	GetDispatchNode(ctx context.Context) string
	GetDispatchTxID(ctx context.Context) string
	GetConfirmedTxHash(ctx context.Context) string
}

type TxStateSetters interface {
	ApplyTxUpdates(ctx context.Context, txUpdates *TransactionUpdate)
}

type TxStateManager interface {
	TxStateGetters
	TxStateSetters
}

type TransactionWrapper struct {
	Transaction
	*components.PrivateTransaction
}

type Transaction struct {
	gorm.Model
	ID                 uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4()"`
	From               string     `gorm:"type:text"`
	SequenceID         *uuid.UUID `gorm:"type:uuid"`
	DomainID           string     `gorm:"type:uuid"`
	SchemaID           string     `gorm:"type:uuid"`
	AssembledRound     int64      `gorm:"type:int"`
	AttestationPlan    string     `gorm:"type:text[]; serializer:json"`
	AttestationResults string     `gorm:"type:text[]; serializer:json"`
	Contract           string     `gorm:"type:uuid"`
	PayloadJSON        *string    `gorm:"type:text"`
	PayloadRLP         *string    `gorm:"type:text"`

	PreReqTxs         []string `gorm:"type:text[]; serializer:json"`
	DispatchNode      string   `gorm:"type:text"`
	DispatchAddress   string   `gorm:"type:text"`
	DispatchTxID      string   `gorm:"type:text"`
	DispatchTxPayload string   `gorm:"type:text"`
	ConfirmedTxHash   string   `gorm:"type:text"`
}

type TransactionUpdate struct { // TODO define updatable fields
	SequenceID         *uuid.UUID // this is just an example used for testing, sequence ID might not be updatable
	DispatchTxID       *string
	DispatchAddress    *string
	AssembledRound     int64
	PayloadJSON        string
	AttestationPlan    *string
	AttestationResults *string
	AssembleError      string
}

func NewTransactionStageManager(ctx context.Context, tx *components.PrivateTransaction) TxStateManager {

	// TODO: this function should use a cache and read from DB first
	return &TransactionWrapper{
		Transaction: Transaction{
			ID: tx.ID,
		},
		PrivateTransaction: tx,
	}
}
func NewTransactionStageManagerByTxID(ctx context.Context, txID string) TxStateManager {
	txUUID := uuid.MustParse(txID)
	// TODO: load tx state manager from DB record
	return &TransactionWrapper{
		Transaction: Transaction{
			ID: txUUID,
		},
		PrivateTransaction: &components.PrivateTransaction{
			ID: txUUID,
		},
	}
}

func (t *TransactionWrapper) HACKGetPrivateTx() *components.PrivateTransaction {
	return t.PrivateTransaction
}

func (t *TransactionWrapper) ApplyTxUpdates(ctx context.Context, txUpdates *TransactionUpdate) {
	if txUpdates.SequenceID != nil {
		t.SequenceID = txUpdates.SequenceID
	}

	if txUpdates.DispatchTxID != nil {
		t.DispatchTxID = *txUpdates.DispatchTxID
	}

	if txUpdates.AttestationPlan != nil {
		t.AttestationPlan = *txUpdates.AttestationPlan
		t.AttestationResults = "" // reset result when there is a new attestation plan
	}

	if txUpdates.AttestationResults != nil {
		t.AttestationResults = *txUpdates.AttestationResults
	}
	// TODO, plug in DB persistence
	// 1. persist to DB first
	// 2. update in memory object
}
func (t *TransactionWrapper) GetContractAddress(ctx context.Context) string {
	return t.Contract
}

func (t *TransactionWrapper) IsAttestationCompleted(ctx context.Context) bool {
	if t.AttestationPlan == "" {
		return true
	} else {
		// TODO: should used in memory objects directly
		attPlan := t.GetAttestationPlan(ctx) // not sure whether attestation completeness needs domain specific knowledge, preference is no.
		if attPlan != nil {
			attResults := t.GetAttestationResults(ctx)
			if attResults == nil || len(attResults) < len(attPlan) {
				return true
			}
		}
	}
	return false
}

func (t *TransactionWrapper) GetAttestationPlan(ctx context.Context) []*prototk.AttestationRequest {
	var attPlans []*prototk.AttestationRequest
	_ = json.Unmarshal([]byte(t.AttestationPlan), &attPlans)
	return attPlans
}
func (t *TransactionWrapper) GetAttestationResults(ctx context.Context) []*prototk.AttestationResult {
	var attResults []*prototk.AttestationResult
	_ = json.Unmarshal([]byte(t.AttestationResults), &attResults)
	return attResults
}
func (t *TransactionWrapper) GetDomainID(ctx context.Context) string {
	return t.DomainID
}

func (t *TransactionWrapper) GetSchemaID(ctx context.Context) string {
	return t.SchemaID
}

func (t *TransactionWrapper) GetTxID(ctx context.Context) string {
	return t.Transaction.ID.String()
}

func (t *TransactionWrapper) GetAssembledRound(ctx context.Context) int64 {
	return t.AssembledRound
}

func (t *TransactionWrapper) GetDispatchAddress(ctx context.Context) string {
	return t.DispatchAddress
}

func (t *TransactionWrapper) GetDispatchNode(ctx context.Context) string {
	return t.DispatchNode
}

func (t *TransactionWrapper) GetDispatchTxID(ctx context.Context) string {
	return t.DispatchTxID
}

func (t *TransactionWrapper) GetDispatchTxPayload(ctx context.Context) string {
	return t.DispatchTxPayload
}

func (t *TransactionWrapper) GetPayloadJSON(ctx context.Context) string {
	return *t.PayloadJSON
}

func (t *TransactionWrapper) GetConfirmedTxHash(ctx context.Context) string {
	return t.ConfirmedTxHash
}

func (t *TransactionWrapper) GetPreReqTransactions(ctx context.Context) []string {
	return t.PreReqTxs
}

type TransactionStore interface {
	InsertTransaction(context.Context, Transaction) (*Transaction, error)
	GetAllTransactions(context.Context) ([]Transaction, error)
	GetTransactionByID(context.Context, uuid.UUID) (*Transaction, error)
	UpdateTransaction(ctx context.Context, t Transaction) (*Transaction, error)
	DeleteTransaction(ctx context.Context, t Transaction) error
}

type transactionStore struct {
	p persistence.Persistence
}

func NewTransactionStore(ctx context.Context, conf *Config, p persistence.Persistence) TransactionStore {
	return &transactionStore{
		p: p,
	}
}

func (ts *transactionStore) InsertTransaction(ctx context.Context, t Transaction) (*Transaction, error) {
	t.ID = uuid.New()
	err := ts.p.DB().
		Table("transactions").
		Create(&t).
		Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (ts *transactionStore) GetTransactionByID(ctx context.Context, id uuid.UUID) (*Transaction, error) {
	log.L(ctx).Infof("GetTransactionByID: %s", id.String())
	var transaction Transaction
	err := ts.p.DB().
		Table("transactions").
		Where("id = ?", id).
		First(&transaction).
		Error
	if err != nil {
		return nil, err
	}
	return &transaction, nil
}

func (ts *transactionStore) UpdateTransaction(ctx context.Context, t Transaction) (*Transaction, error) {
	err := ts.p.DB().
		Table("transactions").
		Save(&t).
		Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (ts *transactionStore) GetAllTransactions(context.Context) ([]Transaction, error) {
	var transactions []Transaction

	err := ts.p.DB().
		Table("transactions").
		Find(&transactions).
		Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

func (ts *transactionStore) DeleteTransaction(ctx context.Context, t Transaction) error {
	err := ts.p.DB().
		Table("transactions").
		Delete(&t).
		Error
	if err != nil {
		return err
	}
	return nil
}
