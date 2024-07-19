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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"gorm.io/gorm"

	"github.com/kaleido-io/paladin/kata/internal/persistence"
)

type Config struct {
}

type TxStateGetters interface {
	GetContract(ctx context.Context) string
	GetTxID(ctx context.Context) string

	GetDispatchTxPayload(ctx context.Context) string

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

type Transaction struct {
	gorm.Model
	ID          uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4()"`
	From        string     `gorm:"type:text"`
	SequenceID  *uuid.UUID `gorm:"type:uuid"`
	Contract    string     `gorm:"type:uuid"`
	PayloadJSON *string    `gorm:"type:text"`
	PayloadRLP  *string    `gorm:"type:text"`

	PreReqTxs         []string `gorm:"type:text[]; serializer:json"`
	DispatchNode      string   `gorm:"type:text"`
	DispatchAddress   string   `gorm:"type:text"`
	DispatchTxID      string   `gorm:"type:text"`
	DispatchTxPayload string   `gorm:"type:text"`
	ConfirmedTxHash   string   `gorm:"type:text"`
}

type TransactionUpdate struct { // TODO define updatable fields
	SequenceID      *uuid.UUID // this is just an example used for testing, sequence ID might not be updatable
	DispatchTxID    *string
	DispatchAddress *string
}

func NewTransaction(ctx context.Context, txID uuid.UUID) TxStateManager {
	// TODO: this function should use a cache
	return &Transaction{
		ID: txID,
	}
}

func (t *Transaction) ApplyTxUpdates(ctx context.Context, txUpdates *TransactionUpdate) {
	if txUpdates.SequenceID != nil {
		t.SequenceID = txUpdates.SequenceID
	}

	if txUpdates.DispatchTxID != nil {
		t.DispatchTxID = *txUpdates.DispatchTxID
	}
	// TODO, plug in DB persistence
	// 1. persist to DB first
	// 2. update in memory object
}
func (t *Transaction) GetContract(ctx context.Context) string {
	return t.Contract
}

func (t *Transaction) GetTxID(ctx context.Context) string {
	return t.ID.String()
}

func (t *Transaction) GetDispatchAddress(ctx context.Context) string {
	return t.DispatchAddress
}

func (t *Transaction) GetDispatchNode(ctx context.Context) string {
	return t.DispatchNode
}

func (t *Transaction) GetDispatchTxID(ctx context.Context) string {
	return t.DispatchTxID
}

func (t *Transaction) GetDispatchTxPayload(ctx context.Context) string {
	return t.DispatchTxPayload
}

func (t *Transaction) GetConfirmedTxHash(ctx context.Context) string {
	return t.ConfirmedTxHash
}

func (t *Transaction) GetPreReqTransactions(ctx context.Context) []string {
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
