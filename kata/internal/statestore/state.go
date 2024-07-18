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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type State struct {
	Hash        HashID          `gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
	CreatedAt   types.Timestamp `gorm:"autoCreateTime:nano"`
	DomainID    string
	Schema      HashID `gorm:"embedded;embeddedPrefix:schema_;"`
	Data        string
	Labels      []*StateLabel      `gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
	Int64Labels []*StateInt64Label `gorm:"foreignKey:state_l,state_h;references:hash_l,hash_h;"`
}

type StateLabel struct {
	State HashID `gorm:"primaryKey;embedded;embeddedPrefix:state_;"`
	Label string
	Value string
}

type StateInt64Label struct {
	State HashID `gorm:"primaryKey;embedded;embeddedPrefix:state_;"`
	Label string
	Value int64
}

type StateUpdate struct {
	TXCreated *string
	TXSpent   *string
}

func (ss *stateStore) PersistState(ctx context.Context, s *State) error {

	schema, err := ss.GetSchema(ctx, s.DomainID, &s.Schema, true)
	if err != nil {
		return err
	}

	if err := schema.ProcessState(ctx, s); err != nil {
		return err
	}

	op := ss.writer.newWriteOp(s.DomainID)
	op.states = []*State{s}
	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) GetState(ctx context.Context, domainID string, hash *HashID, failNotFound, withLabels bool) (*State, error) {
	q := ss.p.DB().Table("states")
	if withLabels {
		q = q.Preload("Labels").Preload("Int64Labels")
	}
	var states []*State
	err := q.
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

type labelTracker struct {
	labels map[string]*schemaLabelInfo
	used   map[string]*schemaLabelInfo
}

func (ft labelTracker) ResolverFor(fieldName string) filters.FieldResolver {
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

func (ss *stateStore) FindStates(ctx context.Context, domainID string, schemaID *HashID, query *filters.QueryJSON) (s []*State, err error) {

	schema, err := ss.GetSchema(ctx, domainID, schemaID, true)
	if err != nil {
		return nil, err
	}

	tracker := labelTracker{labels: make(map[string]*schemaLabelInfo), used: make(map[string]*schemaLabelInfo)}
	for _, fi := range schema.LabelInfo() {
		tracker.labels[fi.label] = fi
	}

	// Build the query
	q := query.Build(ctx, ss.p.DB().Table("states"), tracker)
	if q.Error != nil {
		return nil, q.Error
	}

	// Add joins only for the fields actually used in the query
	for _, fi := range tracker.used {
		typeMod := ""
		if fi.labelType == labelTypeInt64 || fi.labelType == labelTypeBool {
			typeMod = "int64_"
		}
		q = q.Joins(fmt.Sprintf("INNER JOIN state_%[1]slabels AS %[2]s ON %[2]s.state_l = hash_l AND %[2]s.state_h = hash_h AND %[2]s.label = ?", typeMod, fi.virtualColumn), fi.label)
	}

	var states []*State
	q = q.
		Where("domain_id = ?", domainID).
		Where("schema_l = ?", schemaID.L).
		Where("schema_h = ?", schemaID.H).
		Find(&states)
	if q.Error != nil {
		return nil, q.Error
	}
	return states, nil
}
