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
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type State struct {
	ID          types.Bytes32      `json:"id"                  gorm:"primaryKey"`
	CreatedAt   types.Timestamp    `json:"created"             gorm:"autoCreateTime:nano"`
	DomainID    string             `json:"domain"`
	Schema      types.Bytes32      `json:"schema"`
	Data        types.RawJSON      `json:"data"`
	Labels      []*StateLabel      `json:"-"                   gorm:"foreignKey:state;references:id;"`
	Int64Labels []*StateInt64Label `json:"-"                   gorm:"foreignKey:state;references:id;"`
	Confirmed   *StateConfirm      `json:"confirmed,omitempty" gorm:"foreignKey:state;references:id;"`
	Spent       *StateSpend        `json:"spent,omitempty"     gorm:"foreignKey:state;references:id;"`
	Locked      *StateLock         `json:"locked,omitempty"    gorm:"foreignKey:state;references:id;"`
}

type NewState struct {
	SchemaID string
	Data     types.RawJSON
}

// StateWithLabels is a newly prepared state that has not yet been persisted
type StateWithLabels struct {
	*State
	LabelValues filters.ValueSet
}

type StateLabel struct {
	State types.Bytes32 `gorm:"primaryKey"`
	Label string
	Value string
}

type StateInt64Label struct {
	State types.Bytes32 `gorm:"primaryKey"`
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
	id, err := types.ParseBytes32(ctx, stateID)
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
		Where("id = ?", id).
		Limit(1).
		Find(&states).
		Error
	if err == nil && len(states) == 0 && failNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgStateNotFound, id)
	}
	return states[0], err
}

// Built in fields all start with "." as that prevents them
// clashing with variable names in ABI structs ($ and _ are valid leading chars there)
var baseStateFields = map[string]filters.FieldResolver{
	".id":      filters.Bytes32Field("id"),
	".created": filters.TimestampField("created_at"),
}

func addStateBaseLabels(labelValues filters.PassthroughValueSet, id types.Bytes32, createdAt types.Timestamp) filters.PassthroughValueSet {
	labelValues[".id"] = id.HexString()
	labelValues[".created"] = int64(createdAt)
	return labelValues
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

func (ss *stateStore) labelSetFor(schema Schema) *trackingLabelSet {
	tls := trackingLabelSet{labels: make(map[string]*schemaLabelInfo), used: make(map[string]*schemaLabelInfo)}
	for _, fi := range schema.(labelInfoAccess).labelInfo() {
		tls.labels[fi.label] = fi
	}
	return &tls
}

func (ss *stateStore) FindStates(ctx context.Context, domainID, schemaID string, query *filters.QueryJSON, status StateStatusQualifier) (s []*State, err error) {
	_, s, err = ss.findStates(ctx, domainID, schemaID, query, status)
	return s, err
}

func (ss *stateStore) findStates(ctx context.Context, domainID, schemaID string, query *filters.QueryJSON, status StateStatusQualifier, excluded ...*idOnly) (schema Schema, s []*State, err error) {
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
		q = q.Joins(fmt.Sprintf("INNER JOIN state_%[1]slabels AS %[2]s ON %[2]s.state = id AND %[2]s.label = ?", typeMod, fi.virtualColumn), fi.label)
	}

	q = q.Joins("Confirmed", db.Select("transaction")).
		Joins("Spent", db.Select("transaction")).
		Joins("Locked", db.Select("sequence")).
		Where("domain_id = ?", domainID).
		Where("schema = ?", schema.Persisted().ID)

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
	id, err := types.ParseBytes32(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainID)
	op.stateConfirms = []*StateConfirm{
		{State: *id, Transaction: transactionID},
	}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) MarkSpent(ctx context.Context, domainID, stateID string, transactionID uuid.UUID) error {
	id, err := types.ParseBytes32(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainID)
	op.stateSpends = []*StateSpend{
		{State: *id, Transaction: transactionID},
	}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) MarkLocked(ctx context.Context, domainID, stateID string, sequenceID uuid.UUID, creating, spending bool) error {
	id, err := types.ParseBytes32(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainID)
	op.stateLocks = []*StateLock{
		{State: *id, Sequence: sequenceID, Creating: creating, Spending: spending},
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
