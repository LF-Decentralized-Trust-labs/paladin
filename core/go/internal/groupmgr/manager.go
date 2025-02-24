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

package groupmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"gorm.io/gorm"

	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

var groupDBOnlyFilters = filters.FieldMap{
	"id":               filters.HexBytesField("id"),
	"created":          filters.TimestampField("created"),
	"domain":           filters.StringField(`"privacy_groups"."domain"`),
	"genesisSchema":    filters.StringField("schema_id"),
	"genesisSignature": filters.StringField("schema_signature"),
}

type groupManager struct {
	bgCtx     context.Context
	cancelCtx context.CancelFunc

	rpcModule *rpcserver.RPCModule
	conf      *pldconf.GroupManagerConfig

	deployedPGCache  cache.Cache[string, *pldapi.PrivacyGroupWithABI]
	stateManager     components.StateManager
	txManager        components.TXManager
	domainManager    components.DomainManager
	transportManager components.TransportManager
	registryManager  components.RegistryManager
	p                persistence.Persistence
	rpcEventStreams  *rpcEventStreams

	messagesRetry                *retry.Retry
	messagesReadPageSize         int
	messageListenersLoadPageSize int
	messageListenerLock          sync.Mutex
	messageListeners             map[string]*messageListener
}

type referencedReceipt struct {
	Transaction     uuid.UUID           `gorm:"column:transaction;primaryKey"`
	ContractAddress *tktypes.EthAddress `gorm:"column:contract_address"`
}

func (rr referencedReceipt) TableName() string {
	return "transaction_receipts"
}

type persistedGroup struct {
	Domain          string             `gorm:"column:domain;primaryKey"`
	ID              tktypes.HexBytes   `gorm:"column:id;primaryKey"`
	Created         tktypes.Timestamp  `gorm:"column:created"`
	SchemaID        tktypes.Bytes32    `gorm:"column:schema_id"`
	SchemaSignature string             `gorm:"column:schema_signature"`
	GenesisTX       uuid.UUID          `gorm:"column:genesis_tx"`
	Receipt         *referencedReceipt `gorm:"foreignKey:genesis_tx;references:transaction"`
}

func (pg persistedGroup) TableName() string {
	return "privacy_groups"
}

type persistedGroupMember struct {
	Group    tktypes.HexBytes `gorm:"column:group;primaryKey"`
	Domain   string           `gorm:"column:domain;primaryKey"`
	Index    int              `gorm:"column:idx;primaryKey"`
	Identity string           `gorm:"column:identity"`
}

func (pgm persistedGroupMember) TableName() string {
	return "privacy_group_members"
}

func NewGroupManager(bgCtx context.Context, conf *pldconf.GroupManagerConfig) components.GroupManager {
	gm := &groupManager{
		conf:             conf,
		deployedPGCache:  cache.NewCache[string, *pldapi.PrivacyGroupWithABI](&conf.Cache, &pldconf.GroupManagerDefaults.Cache),
		messageListeners: make(map[string]*messageListener),
	}
	gm.messagesInit()
	gm.rpcEventStreams = newRPCEventStreams(gm)
	gm.bgCtx, gm.cancelCtx = context.WithCancel(bgCtx)
	return gm
}

func (gm *groupManager) PreInit(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	gm.initRPC()
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{gm.rpcModule},
	}, nil
}

func (gm *groupManager) PostInit(c components.AllComponents) error {
	gm.stateManager = c.StateManager()
	gm.txManager = c.TxManager()
	gm.domainManager = c.DomainManager()
	gm.p = c.Persistence()
	gm.transportManager = c.TransportManager()
	gm.registryManager = c.RegistryManager()
	return gm.loadMessageListeners()
}

func (gm *groupManager) Start() error {
	gm.startMessageListeners()
	return nil
}

func (gm *groupManager) Stop() {
	gm.rpcEventStreams.stop()
	gm.stopMessageListeners()
	gm.cancelCtx()
}

func (gm *groupManager) validateProperties(ctx context.Context, spec *pldapi.PrivacyGroupInput) (err error) {

	// If no specific ABI has been provided for the input properties, we infer one from the properties
	// that have been supplied. For example if `{"name": "group1", "transaction": "dvp-12345"}` is supplied in the properties, we would
	// infer a properties ABI spec of:
	//
	//  [ { "name": "name", "type": "string", "indexed": true }, { "name": "transaction", "type": "string", "indexed": true } ]
	//
	// Noting details in the ABIInferenceFromJSON() implementation on constraints, and field ordering etc.
	schemaGenerated := false
	if spec.PropertiesABI == nil {
		schemaGenerated = true
		spec.PropertiesABI, err = tktypes.ABIInferenceFromJSON(ctx, spec.Properties)
		if err != nil {
			return err
		}
	}

	// Now we do standardized formatting of the properties back again into the data we pass to the domain.
	// This validates that edge cases are handled (such as mixed arrays in the input properties, which are not supported with ABI inference).
	// It also standardizes the structure of the data format that is passed to the domain code, prior to the full state ABI/data being stored.
	if spec.Properties == nil {
		spec.Properties = tktypes.RawJSON(`{}`)
	}
	cv, err := spec.PropertiesABI.ParseJSONCtx(ctx, spec.Properties)
	if err == nil {
		spec.Properties, err = tktypes.StandardABISerializer().SerializeJSONCtx(ctx, cv)
	}
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgPGroupsDataDoesNotMatchSchema, schemaGenerated)
	}

	return nil

}

func (gm *groupManager) validateMembers(ctx context.Context, members []string) (remoteMembers map[string][]string, err error) {
	localNode := gm.transportManager.LocalNodeName()
	remoteMembers = make(map[string][]string)
	if len(members) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsNoMembers)
	}
	for _, m := range members {
		_, node, err := tktypes.PrivateIdentityLocator(m).Validate(ctx, "", false)
		if err != nil {
			return nil, err
		}
		// Validate we know about a registered transport for the node (for all non-local nodes)
		if node != localNode {
			membersForNode := remoteMembers[node]
			if membersForNode == nil {
				_, err := gm.registryManager.GetNodeTransports(ctx, node)
				if err != nil {
					return nil, err
				}
			}
			remoteMembers[node] = append(membersForNode, m)
		}
	}
	return remoteMembers, nil
}

func (gm *groupManager) insertGroup(ctx context.Context, dbTX persistence.DBTX, pg *persistedGroup, members []string) error {
	err := dbTX.DB().WithContext(ctx).Create(pg).Error
	if err == nil {
		pgms := make([]*persistedGroupMember, len(members))
		for i, identity := range members {
			pgms[i] = &persistedGroupMember{
				Domain:   pg.Domain,
				Group:    pg.ID,
				Index:    i,
				Identity: identity,
			}
		}
		err = dbTX.DB().WithContext(ctx).
			Create(pgms).
			Error
	}
	return err
}

func (gm *groupManager) CreateGroup(ctx context.Context, dbTX persistence.DBTX, spec *pldapi.PrivacyGroupInput) (id tktypes.HexBytes, err error) {

	// Do local validation of the supplied properties
	if err := gm.validateProperties(ctx, spec); err != nil {
		return nil, err
	}

	// Validate the members
	remoteMembers, err := gm.validateMembers(ctx, spec.Members)
	if err != nil {
		return nil, err
	}

	// Get the domain
	domain, err := gm.domainManager.GetDomainByName(ctx, spec.Domain)
	if err != nil {
		return nil, err
	}

	// Now we can ask the domain code to take the input properties, and validated members, and come back with the
	// complete genesis state object for the group
	tx, err := domain.InitPrivacyGroup(ctx, spec)
	if err != nil {
		return nil, err
	}

	// We need to ensure the ABI exists, before we can store the state
	stateABIs, err := gm.stateManager.EnsureABISchemas(ctx, dbTX, spec.Domain, []*abi.Parameter{tx.GenesisSchema})
	if err != nil {
		return nil, err
	}
	genesisSchemaID := stateABIs[0].ID()

	// Now we can upsert the state, to get the ID of the group from the state hash.
	states, err := gm.stateManager.WriteReceivedStates(ctx, dbTX, spec.Domain, []*components.StateUpsertOutsideContext{
		{
			SchemaID: genesisSchemaID,
			Data:     tx.GenesisState,
			// Note there is no contract address associated with this state - as it comes into existence before the deploy
		},
	})
	if err != nil {
		return nil, err
	}
	id = states[0].ID

	// Propagate over input TX options
	if spec.TransactionOptions != nil {
		tx.TX.IdempotencyKey = spec.TransactionOptions.IdempotencyKey
		tx.TX.PublicTxOptions = spec.TransactionOptions.PublicTxOptions
	}
	if tx.TX.From == "" {
		tx.TX.From = fmt.Sprintf("domains.%s.pgroupinit.%s", spec.Domain, id)
	}

	// Insert the transaction
	txIDs, err := gm.txManager.SendTransactions(ctx, dbTX, tx.TX)
	if err != nil {
		return nil, err
	}

	// We have the privacy group, and the state, so we can store all of these in the DB transaction - along with a reliable
	// message transfer to all the parties in the group so they get notification it's there.
	dbPG := &persistedGroup{
		ID:              id,
		Created:         tktypes.TimestampNow(),
		Domain:          spec.Domain,
		SchemaID:        genesisSchemaID,
		SchemaSignature: stateABIs[0].Signature(),
		GenesisTX:       txIDs[0],
	}
	err = gm.insertGroup(ctx, dbTX, dbPG, spec.Members)
	if err != nil {
		return nil, err
	}

	// We also need to create a reliable send the state to all the remote members
	msgs := make([]*components.ReliableMessage, 0, len(remoteMembers))
	for node, members := range remoteMembers {
		for _, identity := range members {
			msgs = append(msgs, &components.ReliableMessage{
				Node:        node,
				MessageType: components.RMTPrivacyGroup.Enum(),
				Metadata: tktypes.JSONString(&components.StateDistribution{
					IdentityLocator: identity,
					Domain:          spec.Domain,
					StateID:         id.String(),
					SchemaID:        tx.GenesisSchema.String(),
				}),
			})
		}
	}
	if len(msgs) > 0 {
		if err := gm.transportManager.SendReliable(ctx, dbTX, msgs...); err != nil {
			return nil, err
		}
	}

	return id, nil
}

func (gm *groupManager) enrichMembers(ctx context.Context, dbTX persistence.DBTX, pgs []*pldapi.PrivacyGroup) error {
	if len(pgs) == 0 {
		return nil
	}
	groupIDs := make([]tktypes.HexBytes, len(pgs))
	for i, pg := range pgs {
		groupIDs[i] = pg.ID
	}
	var dbMembers []*persistedGroupMember
	err := dbTX.DB().WithContext(ctx).
		Where(`"group" IN ( ? )`, groupIDs).
		Order("domain").
		Order(`"group"`).
		Order("idx").
		Find(&dbMembers).
		Error
	if err != nil {
		return err
	}

	for _, dbMember := range dbMembers {
		for _, pg := range pgs {
			if pg.Domain == dbMember.Domain && pg.ID.Equals(dbMember.Group) {
				pg.Members = append(pg.Members, dbMember.Identity)
				break
			}
		}
	}

	return nil
}

func (gm *groupManager) enrichGenesisData(ctx context.Context, dbTX persistence.DBTX, pgs []*pldapi.PrivacyGroup) error {

	groupIDsByDomain := make(map[string]map[tktypes.Bytes32][]tktypes.HexBytes, len(pgs))
	for _, pg := range pgs {
		forDomain := groupIDsByDomain[pg.Domain]
		if forDomain == nil {
			forDomain = make(map[tktypes.Bytes32][]tktypes.HexBytes)
			groupIDsByDomain[pg.Domain] = forDomain
		}
		forDomain[pg.GenesisSchema] = append(forDomain[pg.GenesisSchema], pg.ID)
	}

	for domainName, forDomain := range groupIDsByDomain {
		for _, stateIDs := range forDomain {
			states, err := gm.stateManager.GetStatesByID(ctx, dbTX, domainName, nil, stateIDs, false, false)
			if err != nil {
				return err
			}
			for _, s := range states {
				for _, pg := range pgs {
					if pg.Domain == domainName && pg.GenesisSchema.Equals(&s.Schema) && pg.ID.Equals(s.ID) {
						pg.Genesis = s.Data
						break
					}
				}
			}
		}
	}

	return nil

}

func (dbPG *persistedGroup) mapToAPI() *pldapi.PrivacyGroup {
	pg := &pldapi.PrivacyGroup{
		ID:                 dbPG.ID,
		Domain:             dbPG.Domain,
		Created:            dbPG.Created,
		GenesisSchema:      dbPG.SchemaID,
		GenesisSignature:   dbPG.SchemaSignature,
		GenesisTransaction: dbPG.GenesisTX,
	}
	if dbPG.Receipt != nil {
		pg.ContractAddress = dbPG.Receipt.ContractAddress
	}
	return pg
}

func (gm *groupManager) GetGroupByID(ctx context.Context, dbTX persistence.DBTX, domainName string, groupID tktypes.HexBytes) (*pldapi.PrivacyGroupWithABI, error) {
	groupIDStr := groupID.String()
	pg, found := gm.deployedPGCache.Get(groupIDStr)
	if found {
		return pg, nil
	}

	groups, err := gm.QueryGroups(ctx, dbTX, query.NewQueryBuilder().Equal("id", groupID).Limit(1).Query())
	if err != nil || len(groups) == 0 {
		return nil, err
	}

	pg = &pldapi.PrivacyGroupWithABI{
		PrivacyGroup: groups[0],
	}
	schema, err := gm.stateManager.GetSchemaByID(ctx, dbTX, domainName, pg.GenesisSchema, true)
	if err == nil {
		err = json.Unmarshal(schema.Definition.Bytes(), &pg.GenesisABI)
	}
	if err != nil {
		return nil, err
	}
	// ONLY cache if there is a contract address set (that one-time bind is immutable, but until it happens we need to do the DB JOIN)
	if pg.ContractAddress != nil {
		gm.deployedPGCache.Set(groupIDStr, pg)
	}
	return pg, nil
}

// This function queries the groups only using what's in the DB, without allowing properties of the group to be used to do the query
func (gm *groupManager) QueryGroups(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PrivacyGroup, error) {
	qw := &filters.QueryWrapper[persistedGroup, pldapi.PrivacyGroup]{
		P:           gm.p,
		DefaultSort: "-created",
		Filters:     groupDBOnlyFilters,
		Query:       jq,
		MapResult: func(dbPG *persistedGroup) (*pldapi.PrivacyGroup, error) {
			return dbPG.mapToAPI(), nil
		},
		Finalize: func(db *gorm.DB) *gorm.DB {
			return db.Joins("Receipt")
		},
	}
	pgs, err := qw.Run(ctx, dbTX)
	if err == nil {
		err = gm.enrichMembers(ctx, dbTX, pgs)
	}
	if err == nil {
		err = gm.enrichGenesisData(ctx, dbTX, pgs)
	}
	if err != nil {
		return nil, err
	}
	return pgs, nil
}

// This function queries groups using their properties. Because that requires a schema to know the valid properties that can be queried
// and their types (numeric, string, etc.), it's necessary to provide a reference to the domain and schema. This means that the query
// will be constrained to that particular schema.
func (gm *groupManager) QueryGroupsByProperties(ctx context.Context, dbTX persistence.DBTX, domainName string, schemaID tktypes.Bytes32, jq *query.QueryJSON) ([]*pldapi.PrivacyGroup, error) {

	// Query states, using a JOIN to sub-select only those with a corresponding entry in the privacy group table
	states, err := gm.stateManager.FindStates(ctx, dbTX, domainName, schemaID, jq, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAll,
		QueryModifier: func(db persistence.DBTX, q *gorm.DB) *gorm.DB {
			return q.
				Joins(`LEFT JOIN "privacy_groups" AS "pg" ON "pg"."domain" = "states"."domain_name" AND "pg"."id" = "states"."id"`).
				Where(`"pg"."id" IS NOT NULL`)
		},
	})
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return []*pldapi.PrivacyGroup{}, nil
	}

	// Now query the privacy groups using this list of IDs in the page
	stateIDs := make([]tktypes.HexBytes, len(states))
	for i, s := range states {
		stateIDs[i] = s.ID
	}
	var dbPGs []*persistedGroup
	err = dbTX.DB().WithContext(ctx).
		Where("id IN (?)", stateIDs).
		Joins("Receipt").
		Find(&dbPGs).
		Error
	if err != nil {
		return nil, err
	}

	// Map them all
	pgs := make([]*pldapi.PrivacyGroup, len(dbPGs))
	for i, dbPG := range dbPGs {
		pgs[i] = dbPG.mapToAPI()
	}
	// Members mapped the same as a DB-first query
	if err = gm.enrichMembers(ctx, dbTX, pgs); err != nil {
		return nil, err
	}
	// ... but for states we already have them in hand to map across
	for _, s := range states {
		for _, pg := range pgs {
			if pg.ID.Equals(s.ID) {
				pg.Genesis = s.Data
				break
			}
		}
	}
	return pgs, nil
}

func (gm *groupManager) prepareTransaction(ctx context.Context, dbTX persistence.DBTX, groupID tktypes.HexBytes, tx *pldapi.TransactionInput) error {

	if tx.Type.V() != pldapi.TransactionTypePrivate {
		return i18n.NewError(ctx, msgs.MsgPGroupsTXMustBePrivate)
	}

	if tx.Domain == "" {
		return i18n.NewError(ctx, msgs.MsgPGroupsNoDomain)
	}

	if groupID == nil {
		return i18n.NewError(ctx, msgs.MsgPGroupsNoGroupID)
	}

	// Fluff up the privacy group
	pg, err := gm.GetGroupByID(ctx, dbTX, tx.Domain, groupID)
	if err != nil {
		return err
	}
	if pg == nil {
		return i18n.NewError(ctx, msgs.MsgPGroupsGroupNotFound, groupID)
	}
	if pg.ContractAddress == nil || pg.Genesis == nil {
		return i18n.NewError(ctx, msgs.MsgPGroupsNotReady, groupID, pg.GenesisTransaction)
	}

	// Get the domain smart contract object from domain mgr
	psc, err := gm.domainManager.GetSmartContractByAddress(ctx, dbTX, *pg.ContractAddress)
	if err != nil {
		return err
	}

	// Resolve the input data of the transaction to obtain the ABI etc.
	fn, _, normalizedJSON, err := gm.txManager.ResolveTransactionInputs(ctx, dbTX, tx)
	if err != nil {
		return err
	}
	tx.Data = normalizedJSON

	// Call the domain to take the transaction details that need to be run in the privacy group, and wrap them
	// to build the transaction to call against the domain.
	return psc.WrapPrivacyGroupTransaction(ctx, pg, fn.Definition, tx)

}

func (gm *groupManager) SendTransaction(ctx context.Context, dbTX persistence.DBTX, tx *pldapi.PrivacyGroupTransactionInput) (*uuid.UUID, error) {

	if err := gm.prepareTransaction(ctx, dbTX, tx.GroupID, &tx.TransactionInput); err != nil {
		return nil, err
	}

	txIDs, err := gm.txManager.SendTransactions(ctx, dbTX, &tx.TransactionInput)
	if err != nil {
		return nil, err
	}

	return &txIDs[0], nil

}

func (gm *groupManager) Call(ctx context.Context, dbTX persistence.DBTX, result any, call *pldapi.PrivacyGroupTransactionCall) error {

	if err := gm.prepareTransaction(ctx, dbTX, call.GroupID, &call.TransactionInput); err != nil {
		return err
	}

	return gm.txManager.CallTransaction(ctx, dbTX, result, &call.TransactionCall)

}
