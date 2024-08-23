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
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/types"
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

type SchemaPersisted struct {
	ID         types.Bytes32   `json:"id"          gorm:"primaryKey"`
	CreatedAt  types.Timestamp `json:"created"     gorm:"autoCreateTime:nano"`
	DomainID   string          `json:"domain"`
	Type       SchemaType      `json:"type"`
	Signature  string          `json:"signature"`
	Definition types.RawJSON   `json:"definition"`
	Labels     []string        `json:"labels"      gorm:"type:text[]; serializer:json"`
}

type schemaLabelInfo struct {
	label         string
	virtualColumn string
	labelType     labelType
	resolver      filters.FieldResolver
}

type idOnly struct {
	ID types.Bytes32 `gorm:"primaryKey"`
}

type Schema interface {
	Type() SchemaType
	IDString() string
	Signature() string
	Persisted() *SchemaPersisted
	ProcessState(ctx context.Context, data types.RawJSON) (*StateWithLabels, error)
	RecoverLabels(ctx context.Context, s *State) (*StateWithLabels, error)
}

type labelInfoAccess interface {
	labelInfo() []*schemaLabelInfo
}

func schemaCacheKey(domainID string, id *types.Bytes32) string {
	return domainID + "/" + id.String()
}

func (ss *stateStore) PersistSchema(ctx context.Context, s Schema) error {
	op := ss.writer.newWriteOp(s.Persisted().DomainID)
	op.schemas = []*SchemaPersisted{s.Persisted()}
	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) GetSchema(ctx context.Context, domainID, schemaID string, failNotFound bool) (Schema, error) {
	id, err := types.ParseBytes32Ctx(ctx, schemaID)
	if err != nil {
		return nil, err
	}
	return ss.getSchemaByID(ctx, domainID, id, failNotFound)
}

func (ss *stateStore) getSchemaByID(ctx context.Context, domainID string, schemaID *types.Bytes32, failNotFound bool) (Schema, error) {

	cacheKey := schemaCacheKey(domainID, schemaID)
	s, cached := ss.abiSchemaCache.Get(cacheKey)
	if cached {
		return s, nil
	}

	var results []*SchemaPersisted
	err := ss.p.DB().
		Table("schemas").
		Where("domain_id = ?", domainID).
		Where("id = ?", schemaID).
		Limit(1).
		Find(&results).
		Error
	if err != nil || len(results) == 0 {
		if err == nil && failNotFound {
			return nil, i18n.NewError(ctx, msgs.MsgStateSchemaNotFound, schemaID)
		}
		return s, err
	}

	persisted := results[0]
	switch persisted.Type {
	case SchemaTypeABI:
		s, err = newABISchemaFromDB(ctx, persisted)
	default:
		err = i18n.NewError(ctx, msgs.MsgStateInvalidSchemaType, persisted.Type)
	}
	if err != nil {
		return nil, err
	}
	ss.abiSchemaCache.Set(cacheKey, s)
	return s, nil
}

func (ss *stateStore) ListSchemas(ctx context.Context, domainID string) (results []Schema, err error) {
	var ids []*idOnly
	err = ss.p.DB().
		Table("schemas").
		Select("id").
		Where("domain_id = ?", domainID).
		Find(&ids).
		Error
	if err != nil {
		return nil, err
	}
	results = make([]Schema, len(ids))
	for i, id := range ids {
		if results[i], err = ss.getSchemaByID(ctx, domainID, &id.ID, true); err != nil {
			return nil, err
		}
	}
	return results, nil
}
