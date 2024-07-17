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

type SchemaType string

const (
	// ABI schema uses the same semantics as events for defining indexed fields (must be top-level)
	SchemaTypeABI SchemaType = "abi"
)

type labelType int

const (
	labelTypeInt64 labelType = iota
	labelTypeInt256
	labelTypeUint256
	labelTypeBytes
	labelTypeString
	labelTypeBool
)

type SchemaEntity struct {
	Hash      HashID          `gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
	CreatedAt types.Timestamp `gorm:"autoCreateTime:nano"`
	DomainID  string
	Type      SchemaType
	Signature string
	Content   string
	Labels    []string `gorm:"type:text[]; serializer:json"`
}

type Schema interface {
	Type() SchemaType
	Persisted() *SchemaEntity
	ProcessState(ctx context.Context, s *State) error
}

func schemaCacheKey(domainID string, hash *HashID) string {
	return domainID + "/" + hash.String()
}

func (ss *stateStore) PersistSchema(ctx context.Context, s Schema) error {
	op := ss.writer.newWriteOp(s.Persisted().DomainID)
	op.schemas = []*SchemaEntity{s.Persisted()}
	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) GetSchema(ctx context.Context, domainID string, hash *HashID) (Schema, error) {
	cacheKey := schemaCacheKey(domainID, hash)
	s, cached := ss.abiSchemaCache.Get(cacheKey)
	if cached {
		return s, nil
	}

	var persisted *SchemaEntity
	err := ss.p.DB().
		Table("schemas").
		Where("domain_id = ?", domainID).
		Where("hash_l = ?", hash.L.String()).
		Where("hash_h = ?", hash.H.String()).
		Limit(1).
		Find(&persisted).
		Error
	if err != nil || persisted == nil {
		return s, err
	}

	switch persisted.Type {
	case SchemaTypeABI:
		s, err = newABISchemaFromDB(ctx, persisted)
	default:
		err = i18n.NewError(ctx, msgs.MsgStateInvalidSchemaType, s.Type())
	}
	if err != nil {
		return nil, err
	}
	ss.abiSchemaCache.Set(cacheKey, s)
	return s, nil
}
