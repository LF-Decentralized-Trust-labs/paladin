/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package registrymgr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"gorm.io/gorm/clause"
)

type registry struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf *pldconf.RegistryConfig
	rm   *registryManager
	id   uuid.UUID
	name string
	api  components.RegistryManagerToRegistry

	initialized atomic.Bool
	initRetry   *retry.Retry

	initError atomic.Pointer[error]
	initDone  chan struct{}

	config      *prototk.RegistryConfig
	eventStream *blockindexer.EventStream
}

func (rm *registryManager) newRegistry(id uuid.UUID, name string, conf *pldconf.RegistryConfig, toRegistry components.RegistryManagerToRegistry) *registry {
	r := &registry{
		rm:        rm,
		conf:      conf,
		initRetry: retry.NewRetryIndefinite(&conf.Init.Retry),
		name:      name,
		id:        id,
		api:       toRegistry,
		initDone:  make(chan struct{}),
	}
	r.ctx, r.cancelCtx = context.WithCancel(log.WithLogField(rm.bgCtx, "registry", r.name))
	return r
}

func (r *registry) init() {
	defer close(r.initDone)

	// We block retrying each part of init until we succeed, or are cancelled
	// (which the plugin manager will do if the registry disconnects)
	err := r.initRetry.Do(r.ctx, func(attempt int) (bool, error) {
		// Send the configuration to the registry for processing
		confJSON, _ := json.Marshal(&r.conf.Config)
		res, err := r.api.ConfigureRegistry(r.ctx, &prototk.ConfigureRegistryRequest{
			Name:       r.name,
			ConfigJson: string(confJSON),
		})
		if err == nil {
			r.config = res.RegistryConfig
			err = r.configureEventStream(r.ctx, r.rm.p.NOTX())
		}
		return true, err
	})
	if err != nil {
		log.L(r.ctx).Debugf("registry initialization cancelled before completion: %s", err)
		r.initError.Store(&err)
	} else {
		log.L(r.ctx).Debugf("registry initialization complete")
		r.initialized.Store(true)
		// Inform the plugin manager callback
		r.api.Initialized()
	}
}

func (r *registry) configureEventStream(ctx context.Context, dbTX persistence.DBTX) (err error) {

	if len(r.config.EventSources) == 0 {
		return nil
	}

	stream := &blockindexer.EventStream{
		Type:    blockindexer.EventStreamTypeInternal.Enum(),
		Sources: []blockindexer.EventStreamSource{},
	}

	for i, es := range r.config.EventSources {

		var contractAddr *pldtypes.EthAddress
		if es.ContractAddress != "" {
			contractAddr, err = pldtypes.ParseEthAddress(es.ContractAddress)
			if err != nil {
				return i18n.WrapError(ctx, err, msgs.MsgRegistryInvalidEventSource, i)
			}
		}

		var eventsABI abi.ABI
		if err := json.Unmarshal([]byte(es.AbiEventsJson), &eventsABI); err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgRegistryInvalidEventSource, i)
		}

		stream.Sources = append(stream.Sources, blockindexer.EventStreamSource{
			Address: contractAddr,
			ABI:     eventsABI,
		})
	}

	streamHash, err := stream.Sources.Hash(ctx)
	if err != nil {
		return err
	}
	stream.Name = fmt.Sprintf("registry_%s_%s", r.name, streamHash)

	r.eventStream, err = r.rm.blockIndexer.AddEventStream(ctx, dbTX, &blockindexer.InternalEventStream{
		Definition:  stream,
		HandlerDBTX: r.handleEventBatch,
	})
	return err
}

func (r *registry) UpsertRegistryRecords(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest) (*prototk.UpsertRegistryRecordsResponse, error) {
	err := r.rm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		err = r.upsertRegistryRecords(ctx, dbTX, req.Entries, req.Properties)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &prototk.UpsertRegistryRecordsResponse{}, nil
}

func (r *registry) handleEventBatch(ctx context.Context, dbTX persistence.DBTX, batch *blockindexer.EventDeliveryBatch) error {

	// Build the proto version of these events
	events := make([]*prototk.OnChainEvent, len(batch.Events))
	for i, be := range batch.Events {
		events[i] = &prototk.OnChainEvent{
			Location: &prototk.OnChainEventLocation{
				TransactionHash:  be.TransactionHash.String(),
				BlockNumber:      be.BlockNumber,
				TransactionIndex: be.TransactionIndex,
				LogIndex:         be.LogIndex,
			},
			Signature:         be.Signature.String(),
			SoliditySignature: be.SoliditySignature,
			DataJson:          string(be.Data),
		}
	}

	// Push them down synchronously to the registry to parse
	res, err := r.api.HandleRegistryEvents(ctx, &prototk.HandleRegistryEventsRequest{
		BatchId: batch.BatchID.String(),
		Events:  events,
	})
	if err != nil {
		return err
	}

	// Upsert any transport details that are detected by the registry
	return r.upsertRegistryRecords(ctx, dbTX, res.Entries, res.Properties)

}

func (r *registry) upsertRegistryRecords(ctx context.Context, dbTX persistence.DBTX, protoEntries []*prototk.RegistryEntry, protoProps []*prototk.RegistryProperty) error {

	dbEntries := make([]*DBEntry, len(protoEntries))
	for i, protoEntry := range protoEntries {
		// The registry plugin code is responsible for ensuring these rules are followed
		// before pushing any data to the registry manager.
		// If the registry detects any data that is invalid according to these rules
		// published in their underlying store (such as the blockchain) it must
		// exclude it and act appropriately.
		// Otherwise the registry plugin will receive failures, and it might then stall indexing the
		// its event source in a failure loop with the registry manager.

		// The ID must be parsable as Hex bytes - this could be a 16 byte UUID formatted as plain hex,
		// or it could (more likely) be a hash of the parent_id and the name of the entry meaning
		// it is unique within the whole registry scope.
		entryID, err := pldtypes.ParseHexBytes(ctx, protoEntry.Id)
		if err != nil || len(entryID) == 0 {
			return i18n.WrapError(ctx, err, msgs.MsgRegistryInvalidEntryID, protoEntry.Id)
		}

		var parentID pldtypes.HexBytes
		if protoEntry.ParentId != "" {
			parentID, err = pldtypes.ParseHexBytes(ctx, protoEntry.ParentId)
			if err != nil || len(parentID) == 0 {
				return i18n.WrapError(ctx, err, msgs.MsgRegistryInvalidParentID, protoEntry.ParentId)
			}
		}

		// Names must meet the criteria that is set out in pldtypes.PrivateIdentryLocator for use
		// as a node name. That is not to say this is the only use of entries, but applying this
		// common rule to all entry names ensures we meet the criteria of node names.
		if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, protoEntry.Name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgRegistryInvalidEntryName, protoEntry.Name)
		}

		dbe := &DBEntry{
			Registry: r.name,
			ID:       entryID,
			ParentID: parentID,
			Name:     protoEntry.Name,
			Active:   protoEntry.Active,
		}
		if protoEntry.Location != nil {
			txHash, _ := pldtypes.ParseBytes32(protoEntry.Location.TransactionHash)
			dbe.TransactionHash = &txHash
			dbe.BlockNumber = &protoEntry.Location.BlockNumber
			dbe.TransactionIndex = &protoEntry.Location.LogIndex
			dbe.LogIndex = &protoEntry.Location.LogIndex
		}
		dbEntries[i] = dbe
	}

	dbProps := make([]*DBProperty, len(protoProps))
	for i, protoProp := range protoProps {

		// DB will check for relationship to entry, but we need to parse the ID consistently into bytes
		entryID, err := pldtypes.ParseHexBytes(ctx, protoProp.EntryId)
		if err != nil || len(entryID) == 0 {
			return i18n.WrapError(ctx, err, msgs.MsgRegistryInvalidEntryID, protoProp.EntryId)
		}

		// Plugin reserved property names must start with $, which is not valid in the name so we
		// cut it before checking the rest of the string.
		nameToCheck, hasReservedPrefix := strings.CutPrefix(protoProp.Name, "$")
		if protoProp.PluginReserved != hasReservedPrefix {
			return i18n.WrapError(ctx, err, msgs.MsgRegistryDollarPrefixReserved, protoProp.Name, protoProp.PluginReserved)
		}

		// We require the names of properties to conform to rules, so that we can distinguish
		// these properties from our ".id", ".created", ".updated" properties.
		// Note as above it is the registry plugin's responsibility to handle cases where a
		// value that does not conform is published to it (by logging and discarding it etc.)
		if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, nameToCheck, pldtypes.DefaultNameMaxLen, "name"); err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgRegistryInvalidPropertyName, protoProp.Name)
		}

		dbp := &DBProperty{
			Registry: r.name,
			EntryID:  entryID,
			Name:     protoProp.Name,
			Active:   protoProp.Active,
			Value:    protoProp.Value,
		}
		if protoProp.Location != nil {
			txHash, _ := pldtypes.ParseBytes32(protoProp.Location.TransactionHash)
			dbp.TransactionHash = &txHash
			dbp.BlockNumber = &protoProp.Location.BlockNumber
			dbp.TransactionIndex = &protoProp.Location.LogIndex
			dbp.LogIndex = &protoProp.Location.LogIndex
		}
		dbProps[i] = dbp
	}

	var err error

	if len(dbEntries) > 0 {
		err = dbTX.DB().
			WithContext(ctx).
			Table("reg_entries").
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "registry"},
					{Name: "id"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"updated",
					"active", // this is the primary thing that can actually be mutated
					"tx_hash",
					"block_number",
					"tx_index",
					"log_index",
				}),
			}).
			Create(dbEntries).
			Error
	}

	if err != nil {
		return err
	}

	if len(dbProps) > 0 {
		err = dbTX.DB().
			WithContext(ctx).
			Table("reg_props").
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "registry"},
					{Name: "entry_id"},
					{Name: "name"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"updated",
					"active",
					"value",
					"tx_hash",
					"block_number",
					"tx_index",
					"log_index",
				}),
			}).
			Create(dbProps).
			Error
	}

	if err != nil {
		return err
	}

	dbTX.AddPostCommit(func(ctx context.Context) {
		// It's a lot of work to determine which parts of the node transport cache are affected,
		// as the upserts above happen simply by storing properties that might/might-not match
		// queries that resolve node transports to names.
		//
		// So instead we just zap the whole cache when we have an update.
		r.rm.transportDetailsCache.Clear()
	})
	return nil
}

type dynamicFieldSet struct {
	props       []string
	propIndexes map[string]int
}

// Track which fields are used in the query, as we create a dynamic join for each
func (dfs *dynamicFieldSet) ResolverFor(propName string) filters.FieldResolver {
	switch propName {
	case ".id":
		return filters.HexBytesField(`"reg_entries"."id"`)
	case ".parentId":
		return filters.HexBytesField(`"reg_entries"."parent_id"`)
	case ".name":
		return filters.StringField(`"reg_entries"."name"`)
	case ".created":
		return filters.TimestampField(`"reg_entries"."created"`)
	case ".updated":
		return filters.TimestampField(`"reg_entries"."updated"`)
	}

	idx, exists := dfs.propIndexes[propName]
	if !exists {
		idx = len(dfs.props)
		dfs.propIndexes[propName] = idx
		dfs.props = append(dfs.props, propName)
	}
	return filters.StringField(fmt.Sprintf("p%d.value", idx))
}

func (r *registry) QueryEntries(ctx context.Context, dbTX persistence.DBTX, fActive pldapi.ActiveFilter, jq *query.QueryJSON) ([]*pldapi.RegistryEntry, error) {

	if jq.Limit == nil || *jq.Limit == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgRegistryQueryLimitRequired)
	}

	dfs := &dynamicFieldSet{propIndexes: make(map[string]int)}

	q := filters.BuildGORM(ctx, jq,
		dbTX.DB().WithContext(ctx).
			Table("reg_entries").
			Where(`"reg_entries"."registry" = ?`, r.name),
		dfs)

	switch fActive {
	case pldapi.ActiveFilterAny: // no filter
	case pldapi.ActiveFilterInactive:
		q = q.Where(`"reg_entries"."active" IS FALSE`)
	case pldapi.ActiveFilterActive:
		fallthrough
	default:
		q = q.Where(`"reg_entries"."active" IS TRUE`)
	}

	// After BuildGORM completes, dfs will have a list of all the fields used in the query.
	// We create a join to a virtual column for each.
	for idx, prop := range dfs.props {
		q = q.Joins(fmt.Sprintf(
			// The property might not exist, so LEFT JOIN (assured to be zero or one),
			// this will give us NULL for unset properties.
			`LEFT JOIN reg_props AS p%[1]d `+
				`ON p%[1]d.registry = ? `+
				`AND p%[1]d.active IS TRUE `+ // only select on active props, regardless of active query on entry
				`AND p%[1]d.entry_id = "reg_entries"."id" `+
				`AND p%[1]d.name = ?`, idx),
			r.name,
			prop)
	}

	var dbEntries []*DBEntry
	err := q.Find(&dbEntries).Error
	if err != nil {
		return nil, err
	}

	entries := make([]*pldapi.RegistryEntry, len(dbEntries))
	for i, dbe := range dbEntries {
		entry := &pldapi.RegistryEntry{
			Registry: dbe.Registry,
			ID:       dbe.ID,
			Name:     dbe.Name,
		}
		// Return nil (not empty) for parent string here - this avoids DB index complexity with null values
		if len(dbe.ParentID) > 0 {
			entry.ParentID = dbe.ParentID
		}
		// Return the active field in the JSON if the query was anything apart from "active"
		if fActive != pldapi.ActiveFilterActive {
			entry.ActiveFlag = &pldapi.ActiveFlag{Active: dbe.Active}
		}
		// For block info, our insert logic ensures if one is set they are all set
		if dbe.BlockNumber != nil {
			entry.OnChainLocation = &pldapi.OnChainLocation{
				BlockNumber:      *dbe.BlockNumber,
				TransactionIndex: *dbe.TransactionIndex,
				LogIndex:         *dbe.TransactionIndex,
			}
		}
		entries[i] = entry
	}

	return entries, nil

}

func (r *registry) GetEntryProperties(ctx context.Context, dbTX persistence.DBTX, fActive pldapi.ActiveFilter, entryIDs ...pldtypes.HexBytes) ([]*pldapi.RegistryProperty, error) {

	var dbProps []*DBProperty
	q := dbTX.DB().WithContext(ctx).
		Table("reg_props").
		Where("registry = ?", r.name).
		Where("entry_id IN (?)", entryIDs)

	switch fActive {
	case pldapi.ActiveFilterAny: // no filter
	case pldapi.ActiveFilterInactive:
		q = q.Where("active IS FALSE")
	case pldapi.ActiveFilterActive:
		fallthrough
	default:
		q = q.Where("active IS TRUE")
	}

	err := q.Order("name").Find(&dbProps).Error
	if err != nil {
		return nil, err
	}

	props := make([]*pldapi.RegistryProperty, len(dbProps))
	for i, dbp := range dbProps {
		prop := &pldapi.RegistryProperty{
			Registry: dbp.Registry,
			EntryID:  dbp.EntryID,
			Name:     dbp.Name,
			Value:    dbp.Value,
		}
		// Return the active field in the JSON if the query was anything apart from "active"
		if fActive != pldapi.ActiveFilterActive {
			prop.ActiveFlag = &pldapi.ActiveFlag{Active: dbp.Active}
		}
		// For block info, our insert logic ensures if one is set they are all set
		if dbp.BlockNumber != nil {
			prop.OnChainLocation = &pldapi.OnChainLocation{
				BlockNumber:      *dbp.BlockNumber,
				TransactionIndex: *dbp.TransactionIndex,
				LogIndex:         *dbp.TransactionIndex,
			}
		}
		props[i] = prop
	}

	return props, nil

}

func filteredPropsMap(entryProps []*pldapi.RegistryProperty, entryID pldtypes.HexBytes) map[string]string {
	props := make(map[string]string)
	for _, p := range entryProps {
		if p.EntryID.Equals(entryID) {
			props[p.Name] = p.Value
		}
	}
	return props
}

func (r *registry) QueryEntriesWithProps(ctx context.Context, dbTX persistence.DBTX, fActive pldapi.ActiveFilter, jq *query.QueryJSON) ([]*pldapi.RegistryEntryWithProperties, error) {

	entries, err := r.QueryEntries(ctx, dbTX, fActive, jq)
	if err != nil {
		return nil, err
	}

	entryIDs := make([]pldtypes.HexBytes, len(entries))
	for i, e := range entries {
		entryIDs[i] = e.ID
	}

	withProps := make([]*pldapi.RegistryEntryWithProperties, len(entries))
	if len(entryIDs) > 0 {
		entryProps, err := r.GetEntryProperties(ctx, dbTX, pldapi.ActiveFilterActive /* still active props regardless of filter on active for entry */, entryIDs...)
		if err != nil {
			return nil, err
		}
		for i, e := range entries {
			withProps[i] = &pldapi.RegistryEntryWithProperties{
				RegistryEntry: e,
				Properties:    filteredPropsMap(entryProps, e.ID),
			}
		}

	}

	return withProps, nil
}

func (r *registry) close() {
	r.cancelCtx()
	<-r.initDone
}
