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

package statestore

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type State struct {
	Hash          HashID          `gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
	CreatedAt     types.Timestamp `gorm:"autoCreateTime:nano"`
	DomainID      string
	Schema        HashID `gorm:"embedded;embeddedPrefix:schema_;"`
	Data          string
	TextLabels    []StateTextLabel    `gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
	IntegerLabels []StateIntegerLabel `gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
}

type StateTextLabel struct {
	State HashID `gorm:"primaryKey;embedded;embeddedPrefix:state_;"`
	Label string
	Value string
}

type StateIntegerLabel struct {
	State HashID `gorm:"primaryKey;embedded;embeddedPrefix:state_;"`
	Label string
	Value int64
}

type StateUpdate struct {
	TXCreated *string
	TXSpent   *string
}

func (ss *stateStore) PersistState(ctx context.Context, s *State) error {

	schema, err := ss.GetSchema(ctx, s.DomainID, &s.Schema)
	if err != nil {
		return err
	}
	if schema == nil {
		return i18n.NewError(ctx, msgs.MsgStateSchemaNotFound, &s.Schema)
	}

	if err := schema.ProcessState(ctx, s); err != nil {
		return err
	}

	op := ss.writer.newWriteOp(s.DomainID)
	op.states = []*State{s}
	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) GetState(ctx context.Context, domainID string, hash *HashID, withLabels bool) (s *State, err error) {
	q := ss.p.DB().Table("states")
	if withLabels {
		q = q.Preload("TextLabels").Preload("IntegerLabels")
	}
	err = q.
		Where("domain_id = ?", domainID).
		Where("hash_l = ?", hash.L.String()).
		Where("hash_h = ?", hash.H.String()).
		Limit(1).
		Find(&s).
		Error
	return s, err
}
