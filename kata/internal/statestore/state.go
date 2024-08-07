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
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type State struct {
	Hash        types.HashID       `json:"hash"                gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
	CreatedAt   types.Timestamp    `json:"created"             gorm:"autoCreateTime:nano"`
	DomainID    string             `json:"domain"`
	Schema      types.HashID       `json:"schema"              gorm:"embedded;embeddedPrefix:schema_;"`
	Data        types.RawJSON      `json:"data"`
	Labels      []*StateLabel      `json:"-"                   gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
	Int64Labels []*StateInt64Label `json:"-"                   gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
	Confirmed   *StateConfirm      `json:"confirmed,omitempty" gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
	Spent       *StateSpend        `json:"spent,omitempty"     gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
	Locked      *StateLock         `json:"locked,omitempty"    gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
}

// StateWithLabels is a newly prepared state that has not yet been persisted
type StateWithLabels struct {
	*State
	LabelValues filters.ValueSet
}

type StateLabel struct {
	State types.HashID `gorm:"primaryKey;embedded;embeddedPrefix:state_;"`
	Label string
	Value string
}

type StateInt64Label struct {
	State types.HashID `gorm:"primaryKey;embedded;embeddedPrefix:state_;"`
	Label string
	Value int64
}

type StateUpdate struct {
	TXCreated *string
	TXSpent   *string
}

func (s *StateWithLabels) ValueSet() filters.ValueSet {
	return s.LabelValues
}

func (ss *stateStore) PersistState(ctx context.Context, domainID string, schemaID string, data types.RawJSON) (*StateWithLabels, error) {

	schema, err := ss.GetSchema(ctx, domainID, schemaID, true)
	if err != nil {
		return nil, err
	}

	s, err := schema.ProcessState(ctx, data)
	if err != nil {
		return nil, err
	}

	op := ss.writer.newWriteOp(s.State.DomainID)
	op.states = []*StateWithLabels{s}
	ss.writer.queue(ctx, op)
	return s, op.flush(ctx)
}

func (ss *stateStore) GetState(ctx context.Context, domainID, stateID string, failNotFound, withLabels bool) (*State, error) {
	hash, err := types.ParseHashID(ctx, stateID)
	if err != nil {
		return nil, err
	}

	q := ss.p.DB().Table("states")
	if withLabels {
		q = q.Preload("Labels").Preload("Int64Labels")
	}
	var states []*State
	err = q.
		Where("domain_id = ?", domainID).
		Where("hash_l = ?", hash.L.String()).
		Where("hash_h = ?", hash.H.String()).
		Limit(1).
		Find(&states).
		Error
	if err == nil && len(states) == 0 && failNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgStateNotFound, hash)
	}
	return states[0], err
}

// Built in fields all start with "." as that prevents them
// clashing with variable names in ABI structs ($ and _ are valid leading chars there)
var baseStateFields = map[string]filters.FieldResolver{
	// Only field you can query on outside of the labels, is the created timestamp.
	// - if getting by the state ID you make a different API call
	// - when submitting a query you have to specify the domain + schema to scope your query
	".created": filters.TimestampField("created_at"),
}

type trackingLabelSet struct {
	labels map[string]*schemaLabelInfo
	used   map[string]*schemaLabelInfo
}

func (ft trackingLabelSet) ResolverFor(fieldName string) filters.FieldResolver {
	baseField := baseStateFields[fieldName]
	if baseField != nil {
		return baseField
	}
	f := ft.labels[fieldName]
	if f != nil {
		ft.used[fieldName] = f
		return f.resolver
	}
	return nil
}

func (ss *stateStore) labelSetFor(schema SchemaCommon) *trackingLabelSet {
	tls := trackingLabelSet{labels: make(map[string]*schemaLabelInfo), used: make(map[string]*schemaLabelInfo)}
	for _, fi := range schema.LabelInfo() {
		tls.labels[fi.label] = fi
	}
	return &tls
}

func (ss *stateStore) FindStates(ctx context.Context, domainID, schemaID string, query *filters.QueryJSON, status StateStatusQualifier) (s []*State, err error) {
	_, s, err = ss.findStates(ctx, domainID, schemaID, query, status)
	return s, err
}

func (ss *stateStore) findStates(ctx context.Context, domainID, schemaID string, query *filters.QueryJSON, status StateStatusQualifier, excluded ...*hashIDOnly) (schema SchemaCommon, s []*State, err error) {
	schema, err = ss.GetSchema(ctx, domainID, schemaID, true)
	if err != nil {
		return nil, nil, err
	}

	tracker := ss.labelSetFor(schema)

	// Build the query
	db := ss.p.DB()
	q := query.BuildGORM(ctx, db.Table("states"), tracker)
	if q.Error != nil {
		return nil, nil, q.Error
	}

	// Add joins only for the fields actually used in the query
	for _, fi := range tracker.used {
		typeMod := ""
		if fi.labelType == labelTypeInt64 || fi.labelType == labelTypeBool {
			typeMod = "int64_"
		}
		q = q.Joins(fmt.Sprintf("INNER JOIN state_%[1]slabels AS %[2]s ON %[2]s.state_l = hash_l AND %[2]s.state_h = hash_h AND %[2]s.label = ?", typeMod, fi.virtualColumn), fi.label)
	}

	q = q.Joins("Confirmed", db.Select("transaction")).
		Joins("Spent", db.Select("transaction")).
		Joins("Locked", db.Select("sequence")).
		Where("domain_id = ?", domainID).
		Where("schema_l = ?", schema.Persisted().Hash.L).
		Where("schema_h = ?", schema.Persisted().Hash.H)

	if len(excluded) > 0 {
		q = q.Not(&State{}, excluded)
	}

	// Scope the query based of the qualifier
	q = q.Where(status.whereClause(db))

	var states []*State
	q = q.Find(&states)
	if q.Error != nil {
		return nil, nil, q.Error
	}
	return schema, states, nil
}

func (ss *stateStore) MarkConfirmed(ctx context.Context, domainID, stateID string, transactionID uuid.UUID) error {
	hash, err := types.ParseHashID(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainID)
	op.stateConfirms = []*StateConfirm{
		{State: *hash, Transaction: transactionID},
	}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) MarkSpent(ctx context.Context, domainID, stateID string, transactionID uuid.UUID) error {
	hash, err := types.ParseHashID(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainID)
	op.stateSpends = []*StateSpend{
		{State: *hash, Transaction: transactionID},
	}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) MarkLocked(ctx context.Context, domainID, stateID string, sequenceID uuid.UUID, creating, spending bool) error {
	hash, err := types.ParseHashID(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainID)
	op.stateLocks = []*StateLock{
		{State: *hash, Sequence: sequenceID, Creating: creating, Spending: spending},
	}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) ResetSequence(ctx context.Context, domainID string, sequenceID uuid.UUID) error {
	op := ss.writer.newWriteOp(domainID)
	op.sequenceLockDeletes = []uuid.UUID{sequenceID}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}
