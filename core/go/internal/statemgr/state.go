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
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (ss *stateManager) WritePreVerifiedStates(ctx context.Context, dbTX *gorm.DB, domainName string, states []*components.StateUpsertOutsideContext) ([]*components.State, error) {

	d, err := ss.domainManager.GetDomainByName(ctx, domainName)
	if err != nil {
		return nil, err
	}

	return ss.processInsertStates(ctx, dbTX, d, states)
}

func (ss *stateManager) WriteReceivedStates(ctx context.Context, dbTX *gorm.DB, domainName string, states []*components.StateUpsertOutsideContext) ([]*components.State, error) {

	d, err := ss.domainManager.GetDomainByName(ctx, domainName)
	if err != nil {
		return nil, err
	}

	if d.CustomHashFunction() {
		dStates := make([]*components.FullState, len(states))
		for i, s := range states {
			dStates[i] = &components.FullState{
				ID:     s.ID,
				Schema: s.SchemaID,
				Data:   s.Data,
			}
		}
		ids, err := d.ValidateStateHashes(ctx, dStates)
		if err != nil {
			// Whole batch fails if any state in the batch is invalid
			return nil, err
		}
		for i, s := range states {
			// The domain is responsible for generating any missing IDs
			s.ID = ids[i]
		}
	}

	return ss.processInsertStates(ctx, dbTX, d, states)
}

func (ss *stateManager) processInsertStates(ctx context.Context, dbTX *gorm.DB, d components.Domain, inStates []*components.StateUpsertOutsideContext) (processedStates []*components.State, err error) {

	processedStates = make([]*components.State, len(inStates))
	for i, inState := range inStates {
		schema, err := ss.GetSchema(ctx, d.Name(), inState.SchemaID, dbTX, true)
		if err != nil {
			return nil, err
		}

		s, err := schema.ProcessState(ctx, inState.ContractAddress, inState.Data, inState.ID, d.CustomHashFunction())
		if err != nil {
			return nil, err
		}
		processedStates[i] = s.State
	}

	// Write them directly
	if err = ss.writeStates(ctx, dbTX, processedStates); err != nil {
		return nil, err
	}

	return processedStates, nil
}

func (ss *stateManager) writeStates(ctx context.Context, dbTX *gorm.DB, states []*components.State) (err error) {
	var labels []*components.StateLabel
	var int64Labels []*components.StateInt64Label
	for _, s := range states {
		labels = append(labels, s.Labels...)
		int64Labels = append(int64Labels, s.Int64Labels...)
	}

	if len(states) > 0 {
		err = dbTX.
			Table("states").
			WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain_name"}, {Name: "id"}},
				DoNothing: true, // immutable
			}).
			Omit("Labels", "Int64Labels", "Confirmed", "Spent"). // we do this ourselves below
			Create(states).
			Error
	}
	if err == nil && len(labels) > 0 {
		err = dbTX.
			Table("state_labels").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain_name"}, {Name: "state"}, {Name: "label"}},
				DoNothing: true, // immutable
			}).
			Create(labels).
			Error
	}
	if err == nil && len(int64Labels) > 0 {
		err = dbTX.
			Table("state_int64_labels").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain_name"}, {Name: "state"}, {Name: "label"}},
				DoNothing: true, // immutable
			}).
			Create(int64Labels).
			Error
	}
	return err
}

func (ss *stateManager) GetState(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, stateID tktypes.HexBytes, failNotFound, withLabels bool) (*components.State, error) {
	q := ss.p.DB().Table("states")
	if withLabels {
		q = q.Preload("Labels").Preload("Int64Labels")
	}
	var states []*components.State
	err := q.
		Where("domain_name = ?", domainName).
		Where("contract_address = ?", contractAddress).
		Where("id = ?", stateID).
		Limit(1).
		Find(&states).
		Error
	if err == nil && len(states) == 0 && failNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgStateNotFound, stateID)
	}
	return states[0], err
}

// Built in fields all start with "." as that prevents them
// clashing with variable names in ABI structs ($ and _ are valid leading chars there)
var baseStateFields = map[string]filters.FieldResolver{
	".id":      filters.HexBytesField(`"states"."id"`),
	".created": filters.TimestampField(`"states"."created"`),
}

func addStateBaseLabels(labelValues filters.PassthroughValueSet, id tktypes.HexBytes, createdAt tktypes.Timestamp) filters.PassthroughValueSet {
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

func (ss *stateManager) labelSetFor(schema components.Schema) *trackingLabelSet {
	tls := trackingLabelSet{labels: make(map[string]*schemaLabelInfo), used: make(map[string]*schemaLabelInfo)}
	for _, fi := range schema.(labelInfoAccess).labelInfo() {
		tls.labels[fi.label] = fi
	}
	return &tls
}

func (ss *stateManager) FindStates(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, schemaID tktypes.Bytes32, query *query.QueryJSON, status StateStatusQualifier) (s []*components.State, err error) {
	_, s, err = ss.findStates(ctx, domainName, contractAddress, schemaID, query, status)
	return s, err
}

func (ss *stateManager) findStates(
	ctx context.Context,
	domainName string,
	contractAddress tktypes.EthAddress,
	schemaID tktypes.Bytes32,
	jq *query.QueryJSON,
	status StateStatusQualifier,
	excluded ...tktypes.HexBytes,
) (schema components.Schema, s []*components.State, err error) {
	db := ss.p.DB()
	whereClause, isPlainDB := status.whereClause(db)
	if isPlainDB {
		return ss.findStatesCommon(ctx, domainName, contractAddress, schemaID, jq, func(q *gorm.DB) *gorm.DB {
			q = q.Joins("Confirmed", db.Select("transaction")).
				Joins("Spent", db.Select("transaction"))

			if len(excluded) > 0 {
				q = q.Not(`"states"."id" IN(?)`, excluded)
			}

			// Scope the query based on the status qualifier
			q = q.Where(whereClause)
			return q
		})
	}

	// We need to run it against the specified domain context
	var dc components.DomainContext
	dcID, err := uuid.Parse(string(status))
	if err == nil {
		if dc = ss.GetDomainContext(ctx, dcID); dc == nil {
			err = i18n.NewError(ctx, msgs.MsgStateDomainContextNotActive, dcID)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	return dc.FindAvailableStates(schemaID, jq)
}

func (ss *stateManager) findAvailableNullifiers(
	ctx context.Context,
	domainName string,
	contractAddress tktypes.EthAddress,
	schemaID tktypes.Bytes32,
	jq *query.QueryJSON,
	spendingStates []tktypes.HexBytes,
	spendingNullifiers []tktypes.HexBytes,
) (schema components.Schema, s []*components.State, err error) {
	return ss.findStatesCommon(ctx, domainName, contractAddress, schemaID, jq, func(q *gorm.DB) *gorm.DB {
		db := ss.p.DB()
		hasNullifier := db.Where(`"Nullifier"."id" IS NOT NULL`)

		q = q.Joins("Confirmed", db.Select("transaction")).
			Joins("Nullifier", db.Select(`"Nullifier"."id"`)).
			Joins("Nullifier.Spent", db.Select("transaction")).
			Where(hasNullifier)

		if len(spendingStates) > 0 {
			q = q.Not(`"states"."id" IN(?)`, spendingStates)
		}
		if len(spendingNullifiers) > 0 {
			q = q.Not(`"Nullifier"."id" IN(?)`, spendingNullifiers)
		}

		// Scope to only unspent
		q = q.Where(`"Nullifier__Spent"."transaction" IS NULL`).
			Where(db.
				Or(`"Confirmed"."transaction" IS NOT NULL`),
			)
		return q
	})
}

func (ss *stateManager) findStatesCommon(
	ctx context.Context,
	domainName string,
	contractAddress tktypes.EthAddress,
	schemaID tktypes.Bytes32,
	jq *query.QueryJSON,
	addQuery func(q *gorm.DB) *gorm.DB,
) (schema components.Schema, s []*components.State, err error) {
	if len(jq.Sort) == 0 {
		jq.Sort = []string{".created"}
	}

	schema, err = ss.GetSchema(ctx, domainName, schemaID, nil, true)
	if err != nil {
		return nil, nil, err
	}

	tracker := ss.labelSetFor(schema)

	// Build the query
	db := ss.p.DB()
	q := filters.BuildGORM(ctx, jq, db.Table("states"), tracker)
	if q.Error != nil {
		return nil, nil, q.Error
	}

	// Add joins only for the fields actually used in the query
	for _, fi := range tracker.used {
		typeMod := ""
		if fi.labelType == labelTypeInt64 || fi.labelType == labelTypeBool {
			typeMod = "int64_"
		}
		q = q.Joins(fmt.Sprintf(`INNER JOIN state_%[1]slabels AS %[2]s ON %[2]s.state = "states"."id" AND %[2]s.label = ?`, typeMod, fi.virtualColumn), fi.label)
	}

	q = q.Where("states.domain_name = ?", domainName).
		Where("states.contract_address = ?", contractAddress).
		Where("states.schema = ?", schema.Persisted().ID)
	q = addQuery(q)

	var states []*components.State
	q = q.Find(&states)
	if q.Error != nil {
		return nil, nil, q.Error
	}
	return schema, states, nil
}
