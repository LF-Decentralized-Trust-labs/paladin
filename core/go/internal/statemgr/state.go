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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
)

type transactionStateRecord struct {
	pldapi.StateBase
	State          pldtypes.HexBytes `gorm:"column:state"`
	RecordType     string            `gorm:"column:record_type"`
	SpentState     pldtypes.HexBytes `gorm:"column:spent_state"`
	ReadState      pldtypes.HexBytes `gorm:"column:read_state"`
	ConfirmedState pldtypes.HexBytes `gorm:"column:confirmed_state"`
}

func (transactionStateRecord) TableName() string {
	return "states"
}

func (ss *stateManager) WritePreVerifiedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, states []*components.StateUpsertOutsideContext) ([]*pldapi.State, error) {

	d, err := ss.domainManager.GetDomainByName(ctx, domainName)
	if err != nil {
		return nil, err
	}

	return ss.processInsertStates(ctx, dbTX, d, states)
}

func (ss *stateManager) WriteReceivedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, states []*components.StateUpsertOutsideContext) ([]*pldapi.State, error) {

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

func (ss *stateManager) WriteNullifiersForReceivedStates(ctx context.Context, dbTX persistence.DBTX, domainName string, upserts []*components.NullifierUpsert) (err error) {
	d, err := ss.domainManager.GetDomainByName(ctx, domainName)
	if err != nil {
		return err
	}

	stateNullifiers := make([]*pldapi.StateNullifier, len(upserts))
	for i, n := range upserts {
		stateNullifiers[i] = &pldapi.StateNullifier{
			DomainName: d.Name(),
			ID:         n.ID,
			State:      n.State,
		}
	}

	if len(stateNullifiers) > 0 {
		err = dbTX.DB().
			Table("state_nullifiers").
			Clauses(clause.OnConflict{
				DoNothing: true, // immutable
			}).
			Create(stateNullifiers).
			Error
	}

	return err
}

func (ss *stateManager) processInsertStates(ctx context.Context, dbTX persistence.DBTX, d components.Domain, inStates []*components.StateUpsertOutsideContext) (processedStates []*pldapi.State, err error) {

	processedStates = make([]*pldapi.State, len(inStates))
	for i, inState := range inStates {
		schema, err := ss.getSchemaByID(ctx, dbTX, d.Name(), inState.SchemaID, true)
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

	dbTX.AddPostCommit(ss.txManager.NotifyStatesDBChanged)
	return processedStates, nil
}

func (ss *stateManager) writeStates(ctx context.Context, dbTX persistence.DBTX, states []*pldapi.State) (err error) {
	var labels []*pldapi.StateLabel
	var int64Labels []*pldapi.StateInt64Label
	for _, s := range states {
		labels = append(labels, s.Labels...)
		int64Labels = append(int64Labels, s.Int64Labels...)
	}

	if len(states) > 0 {
		err = dbTX.DB().
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
		err = dbTX.DB().
			Table("state_labels").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain_name"}, {Name: "state"}, {Name: "label"}},
				DoNothing: true, // immutable
			}).
			Create(labels).
			Error
	}
	if err == nil && len(int64Labels) > 0 {
		err = dbTX.DB().
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

func (ss *stateManager) GetStatesByID(ctx context.Context, dbTX persistence.DBTX, domainName string, contractAddress *pldtypes.EthAddress, stateIDs []pldtypes.HexBytes, failNotFound, withLabels bool) ([]*pldapi.State, error) {
	q := dbTX.DB().Table("states")
	if withLabels {
		q = q.Preload("Labels").Preload("Int64Labels")
	}
	var states []*pldapi.State
	q = q.
		Where("domain_name = ?", domainName).
		Where("id IN ?", stateIDs)
	if contractAddress != nil {
		q = q.Where("contract_address = ?", contractAddress)
	}
	err := q.
		Find(&states).
		Error
	if err == nil && len(states) != len(stateIDs) && failNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgStateNotFound, stateIDs)
	}
	return states, err
}

// Built in fields all start with "." as that prevents them
// clashing with variable names in ABI structs ($ and _ are valid leading chars there)
var baseStateFields = map[string]filters.FieldResolver{
	".id":      filters.HexBytesField(`"states"."id"`),
	".created": filters.TimestampField(`"states"."created"`),
}

func addStateBaseLabels(labelValues filters.PassthroughValueSet, id pldtypes.HexBytes, createdAt pldtypes.Timestamp) filters.PassthroughValueSet {
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

func (ss *stateManager) FindContractStates(ctx context.Context, dbTX persistence.DBTX, domainName string, contractAddress *pldtypes.EthAddress, schemaID pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (s []*pldapi.State, err error) {
	_, s, err = ss.findStates(ctx, dbTX, domainName, contractAddress, schemaID, query, &components.StateQueryOptions{StatusQualifier: status})
	return s, err
}

func (ss *stateManager) FindStates(ctx context.Context, dbTX persistence.DBTX, domainName string, schemaID pldtypes.Bytes32, query *query.QueryJSON, options *components.StateQueryOptions) (s []*pldapi.State, err error) {
	_, s, err = ss.findStates(ctx, dbTX, domainName, nil, schemaID, query, options)
	return s, err
}

func (ss *stateManager) FindContractNullifiers(ctx context.Context, dbTX persistence.DBTX, domainName string, contractAddress pldtypes.EthAddress, schemaID pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (s []*pldapi.State, err error) {
	_, s, err = ss.findNullifiers(ctx, dbTX, domainName, &contractAddress, schemaID, query, status, nil, nil)
	return s, err
}

func (ss *stateManager) FindNullifiers(ctx context.Context, dbTX persistence.DBTX, domainName string, schemaID pldtypes.Bytes32, query *query.QueryJSON, status pldapi.StateStatusQualifier) (s []*pldapi.State, err error) {
	_, s, err = ss.findNullifiers(ctx, dbTX, domainName, nil, schemaID, query, status, nil, nil)
	return s, err
}

func (ss *stateManager) findStates(
	ctx context.Context,
	dbTX persistence.DBTX,
	domainName string,
	contractAddress *pldtypes.EthAddress,
	schemaID pldtypes.Bytes32,
	jq *query.QueryJSON,
	options *components.StateQueryOptions,
) (schema components.Schema, s []*pldapi.State, err error) {
	if options == nil {
		options = &components.StateQueryOptions{}
	}
	if options.StatusQualifier == "" {
		options.StatusQualifier = pldapi.StateStatusAll
	}
	whereClause, isPlainDB := whereClauseForQual(dbTX.DB(), options.StatusQualifier, "Spent")
	if isPlainDB {
		return ss.findStatesCommon(ctx, dbTX, domainName, contractAddress, schemaID, jq, func(dbTX persistence.DBTX, q *gorm.DB) *gorm.DB {
			q = q.Joins("Confirmed", dbTX.DB().Select("transaction")).
				Joins("Spent", dbTX.DB().Select("transaction"))

			if len(options.ExcludedIDs) > 0 {
				q = q.Not(`"states"."id" IN(?)`, options.ExcludedIDs)
			}

			// Scope the query based on the status qualifier
			q = q.Where(whereClause)

			if options.QueryModifier != nil {
				q = options.QueryModifier(dbTX, q)
			}
			return q
		})
	}

	// Otherwise, we need to run it against the specified domain context
	var dc components.DomainContext
	dcID, err := uuid.Parse(string(options.StatusQualifier))
	if err == nil {
		if dc = ss.GetDomainContext(ctx, dcID); dc == nil {
			err = i18n.NewError(ctx, msgs.MsgStateDomainContextNotActive, dcID)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	return dc.FindAvailableStates(dbTX, schemaID, jq)
}

func (ss *stateManager) findNullifiers(
	ctx context.Context,
	dbTX persistence.DBTX,
	domainName string,
	contractAddress *pldtypes.EthAddress,
	schemaID pldtypes.Bytes32,
	jq *query.QueryJSON,
	status pldapi.StateStatusQualifier,
	spendingStates []pldtypes.HexBytes,
	spendingNullifiers []pldtypes.HexBytes,
) (schema components.Schema, s []*pldapi.State, err error) {
	whereClause, isPlainDB := whereClauseForQual(dbTX.DB(), status, "Nullifier__Spent")
	if isPlainDB {
		return ss.findStatesCommon(ctx, dbTX, domainName, contractAddress, schemaID, jq, func(dbTX persistence.DBTX, q *gorm.DB) *gorm.DB {
			hasNullifier := dbTX.DB().Where(`"Nullifier"."id" IS NOT NULL`)

			q = q.Joins("Confirmed", dbTX.DB().Select("transaction")).
				Joins("Nullifier", dbTX.DB().Select(`"Nullifier"."id"`)).
				Joins("Nullifier.Spent", dbTX.DB().Select("transaction")).
				Where(hasNullifier)

			if len(spendingStates) > 0 {
				q = q.Not(`"states"."id" IN(?)`, spendingStates)
			}
			if len(spendingNullifiers) > 0 {
				q = q.Not(`"Nullifier"."id" IN(?)`, spendingNullifiers)
			}

			// Scope to only unspent
			q = q.Where(whereClause)
			return q
		})
	}

	// Otherwise, we need to run it against the specified domain context
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
	return dc.FindAvailableNullifiers(dbTX, schemaID, jq)
}

func (ss *stateManager) findStatesCommon(
	ctx context.Context,
	dbTX persistence.DBTX,
	domainName string,
	contractAddress *pldtypes.EthAddress,
	schemaID pldtypes.Bytes32,
	jq *query.QueryJSON,
	modifyQuery func(dbTX persistence.DBTX, q *gorm.DB) *gorm.DB,
) (schema components.Schema, s []*pldapi.State, err error) {
	if len(jq.Sort) == 0 {
		jq.Sort = []string{".created"}
	}

	schema, err = ss.getSchemaByID(ctx, dbTX, domainName, schemaID, true)
	if err != nil {
		return nil, nil, err
	}

	tracker := ss.labelSetFor(schema)

	// Build the query
	q := filters.BuildGORM(ctx, jq, dbTX.DB().Table("states"), tracker)
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
		Where("states.schema = ?", schema.Persisted().ID)
	if contractAddress != nil {
		q = q.Where("states.contract_address = ?", contractAddress)
	}
	q = modifyQuery(dbTX, q)

	var states []*pldapi.State
	q = q.Find(&states)
	if q.Error != nil {
		return nil, nil, q.Error
	}
	return schema, states, nil
}
