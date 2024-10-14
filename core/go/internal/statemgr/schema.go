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

package statemgr

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
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

type schemaLabelInfo struct {
	label         string
	virtualColumn string
	labelType     labelType
	resolver      filters.FieldResolver
}

type idOnly struct {
	ID tktypes.HexBytes `gorm:"primaryKey"`
}

type labelInfoAccess interface {
	labelInfo() []*schemaLabelInfo
}

func schemaCacheKey(domainName string, id tktypes.Bytes32) string {
	return domainName + "/" + id.String()
}

func (ss *stateManager) persistSchemas(ctx context.Context, dbTX *gorm.DB, schemas []*components.SchemaPersisted) error {
	return dbTX.
		Table("schemas").
		WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "domain_name"},
				{Name: "id"},
			},
			DoNothing: true, // immutable
		}).
		Create(schemas).
		Error
}

func (ss *stateManager) GetSchema(ctx context.Context, domainName string, schemaID tktypes.Bytes32, dbTX *gorm.DB, failNotFound bool) (components.Schema, error) {
	return ss.getSchemaByID(ctx, domainName, schemaID, dbTX, failNotFound)
}

func (ss *stateManager) getSchemaByID(ctx context.Context, domainName string, schemaID tktypes.Bytes32, dbTX *gorm.DB, failNotFound bool) (components.Schema, error) {

	cacheKey := schemaCacheKey(domainName, schemaID)
	s, cached := ss.abiSchemaCache.Get(cacheKey)
	if cached {
		return s, nil
	}

	if dbTX == nil {
		dbTX = ss.p.DB()
	}

	var results []*components.SchemaPersisted
	err := dbTX.
		Table("schemas").
		Where("domain_name = ?", domainName).
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
	case components.SchemaTypeABI:
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

func (ss *stateManager) ListSchemas(ctx context.Context, domainName string) (results []components.Schema, err error) {
	var ids []*idOnly
	err = ss.p.DB().
		Table("schemas").
		Select("id").
		Where("domain_name = ?", domainName).
		Find(&ids).
		Error
	if err != nil {
		return nil, err
	}
	results = make([]components.Schema, len(ids))
	for i, id := range ids {
		if results[i], err = ss.getSchemaByID(ctx, domainName, tktypes.Bytes32(id.ID), nil, true); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (ss *stateManager) EnsureABISchemas(ctx context.Context, dbTX *gorm.DB, domainName string, defs []*abi.Parameter) ([]components.Schema, error) {
	if len(defs) == 0 {
		return nil, nil
	}

	// Validate all the schemas
	prepared := make([]components.Schema, len(defs))
	toFlush := make([]*components.SchemaPersisted, len(defs))
	for i, def := range defs {
		s, err := newABISchema(ctx, domainName, def)
		if err != nil {
			return nil, err
		}
		prepared[i] = s
		toFlush[i] = s.SchemaPersisted
	}

	return prepared, ss.persistSchemas(ctx, dbTX, toFlush)
}
