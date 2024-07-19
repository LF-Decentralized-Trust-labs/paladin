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

type Transaction struct {
	gorm.Model
	ID          uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4()"`
	From        string     `gorm:"type:text"`
	SequenceID  *uuid.UUID `gorm:"type:uuid"`
	Contract    string     `gorm:"type:uuid"`
	PayloadJSON *string    `gorm:"type:text"`
	PayloadRLP  *string    `gorm:"type:text"`
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
