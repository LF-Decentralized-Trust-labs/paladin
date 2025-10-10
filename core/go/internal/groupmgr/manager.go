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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

var groupDBOnlyFilters = filters.FieldMap{
	"id":              filters.HexBytesField("id"),
	"name":            filters.StringField("name"),
	"created":         filters.TimestampField("created"),
	"domain":          filters.StringField(`"privacy_groups"."domain"`),
	"contractAddress": filters.HexBytesField(`"Receipt"."contract_address"`),
	"genesisSalt":     filters.HexBytesField("genesis_salt"),
	"genesisSchema":   filters.HexBytesField("genesis_schema"),
}

type groupManager struct {
	bgCtx     context.Context
	cancelCtx context.CancelFunc

	rpcModule *rpcserver.RPCModule
	conf      *pldconf.GroupManagerConfig

	deployedPGCache  cache.Cache[string, *pldapi.PrivacyGroup]
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
	Transaction     uuid.UUID            `gorm:"column:transaction;primaryKey"`
	ContractAddress *pldtypes.EthAddress `gorm:"column:contract_address"`
}

func (rr referencedReceipt) TableName() string {
	return "transaction_receipts"
}

type persistedGroup struct {
	Domain        string             `gorm:"column:domain;primaryKey"`
	ID            pldtypes.HexBytes  `gorm:"column:id;primaryKey"`
	Created       pldtypes.Timestamp `gorm:"column:created"`
	Name          string             `gorm:"column:name"`
	GenesisTX     uuid.UUID          `gorm:"column:genesis_tx"`
	GenesisSchema pldtypes.Bytes32   `gorm:"column:genesis_schema"`
	GenesisSalt   pldtypes.Bytes32   `gorm:"column:genesis_salt"`
	Properties    pldtypes.RawJSON   `gorm:"column:properties"`
	Configuration pldtypes.RawJSON   `gorm:"column:configuration"`
	Receipt       *referencedReceipt `gorm:"foreignKey:genesis_tx;references:transaction"`
}

func (pg persistedGroup) TableName() string {
	return "privacy_groups"
}

type persistedGroupMember struct {
	Group    pldtypes.HexBytes `gorm:"column:group;primaryKey"`
	Domain   string            `gorm:"column:domain;primaryKey"`
	Index    int               `gorm:"column:idx;primaryKey"`
	Identity string            `gorm:"column:identity"`
}

func (pgm persistedGroupMember) TableName() string {
	return "privacy_group_members"
}

func NewGroupManager(bgCtx context.Context, conf *pldconf.GroupManagerConfig) components.GroupManager {
	gm := &groupManager{
		conf:             conf,
		deployedPGCache:  cache.NewCache[string, *pldapi.PrivacyGroup](&conf.Cache, &pldconf.GroupManagerDefaults.Cache),
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

func (gm *groupManager) validateMembers(ctx context.Context, members []string, checkConnectivity bool) (remoteMembers map[string][]string, err error) {
	localNode := gm.transportManager.LocalNodeName()
	remoteMembers = make(map[string][]string)
	if len(members) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsNoMembers)
	}
	for _, m := range members {
		_, node, err := pldtypes.PrivateIdentityLocator(m).Validate(ctx, "", false)
		if err != nil {
			return nil, err
		}
		// Validate we know about a registered transport for the node (for all non-local nodes)
		if node != localNode {
			membersForNode := remoteMembers[node]
			if membersForNode == nil && checkConnectivity {
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

func (gm *groupManager) insertGroup(ctx context.Context, dbTX persistence.DBTX, domainName string, genesisSchemaID pldtypes.Bytes32, stateID pldtypes.HexBytes, genesisTx uuid.UUID, pgGenesis *pldapi.PrivacyGroupGenesisState) (*persistedGroup, error) {
	pg := &persistedGroup{
		ID:            stateID,
		Created:       pldtypes.TimestampNow(),
		Domain:        domainName,
		Name:          pgGenesis.Name,
		GenesisSchema: genesisSchemaID,
		GenesisSalt:   pgGenesis.GenesisSalt,
		Properties:    pldtypes.JSONString(pgGenesis.Properties.Map()),
		Configuration: pldtypes.JSONString(pgGenesis.Configuration.Map()),
		GenesisTX:     genesisTx,
	}
	err := dbTX.DB().
		WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain"}, {Name: "id"}},
			DoNothing: true,
		}).
		Create(pg).Error
	if err == nil {
		pgms := make([]*persistedGroupMember, len(pgGenesis.Members))
		for i, identity := range pgGenesis.Members {
			pgms[i] = &persistedGroupMember{
				Domain:   pg.Domain,
				Group:    pg.ID,
				Index:    i,
				Identity: identity,
			}
		}
		err = dbTX.DB().WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain"}, {Name: "group"}, {Name: "idx"}},
				DoNothing: true,
			}).
			Create(pgms).
			Error
	}
	if err != nil {
		return nil, err
	}
	return pg, nil
}

func (gm *groupManager) validateGroupGenesisSet(ctx context.Context, domainName string, pgGenesis *pldapi.PrivacyGroupGenesisState, checkConnectivity bool) (domain components.Domain, remoteMembers map[string][]string, err error) {
	domain, err = gm.domainManager.GetDomainByName(ctx, domainName)
	if err != nil {
		return nil, nil, err
	}

	// Validate the name - which is optional, but must be valid if supplied
	if pgGenesis.Name != "" {
		if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, pgGenesis.Name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
			return nil, nil, err
		}
	}

	// Validate the members
	remoteMembers, err = gm.validateMembers(ctx, pgGenesis.Members, checkConnectivity)
	if err != nil {
		return nil, nil, err
	}

	// Salt must be non-zero
	if pgGenesis.GenesisSalt == (pldtypes.Bytes32{}) {
		return nil, nil, i18n.NewError(ctx, msgs.MsgPGroupsGenesisSaltUnset)
	}

	return domain, remoteMembers, nil
}

func (gm *groupManager) CreateGroup(ctx context.Context, dbTX persistence.DBTX, spec *pldapi.PrivacyGroupInput) (group *pldapi.PrivacyGroup, err error) {
	pgGenesis := &pldapi.PrivacyGroupGenesisState{
		GenesisSalt: pldtypes.RandBytes32(),
		Name:        spec.Name,
		Members:     spec.Members,
		Properties:  pldapi.NewKeyValueStringProperties(spec.Properties),
	}

	domain, remoteMembers, err := gm.validateGroupGenesisSet(ctx, spec.Domain, pgGenesis, true /* check connectivity */)
	if err != nil {
		return nil, err
	}

	if spec.Configuration == nil {
		spec.Configuration = map[string]string{}
	}
	fullConfig, err := domain.ConfigurePrivacyGroup(ctx, spec.Configuration)
	if err != nil {
		return nil, err
	}
	pgGenesis.Configuration = pldapi.NewKeyValueStringProperties(fullConfig)

	// We need to ensure the ABI exists, before we can store the state
	stateABIs, err := gm.stateManager.EnsureABISchemas(ctx, dbTX, spec.Domain, []*abi.Parameter{pldapi.PrivacyGroupABISchema()})
	if err != nil {
		return nil, err
	}
	genesisSchemaID := stateABIs[0].ID()

	// Now we can upsert the state, to get the ID of the group from the state hash.
	states, err := gm.stateManager.WriteReceivedStates(ctx, dbTX, spec.Domain, []*components.StateUpsertOutsideContext{
		{
			SchemaID: genesisSchemaID,
			Data:     pldtypes.JSONString(&pgGenesis),
			// Note there is no contract address associated with this state - as it comes into existence before the deploy
		},
	})
	if err != nil {
		return nil, err
	}
	id := states[0].ID

	// Now we can ask the domain code to take the input properties, and validated members, and come back with the
	// complete genesis state object for the group
	tx, err := domain.InitPrivacyGroup(ctx, id, pgGenesis)
	if err != nil {
		return nil, err
	}

	// Propagate over input TX options
	if spec.TransactionOptions != nil {
		tx.IdempotencyKey = spec.TransactionOptions.IdempotencyKey
		tx.PublicTxOptions = spec.TransactionOptions.PublicTxOptions
	}
	if tx.From == "" {
		tx.From = fmt.Sprintf("domains.%s.pgroupinit.%s", spec.Domain, id)
	}

	// Insert the transaction
	txIDs, err := gm.txManager.SendTransactions(ctx, dbTX, tx)
	if err != nil {
		return nil, err
	}

	// We have the privacy group, and the state, so we can store all of these in the DB transaction - along with a reliable
	// message transfer to all the parties in the group so they get notification it's there.
	dbPG, err := gm.insertGroup(ctx, dbTX, spec.Domain, genesisSchemaID, states[0].ID, txIDs[0], pgGenesis)
	if err != nil {
		return nil, err
	}
	group = dbPG.mapToAPI()
	group.Members = spec.Members

	// We also need to create a reliable message to send the state to all the remote members
	msgs := make([]*pldapi.ReliableMessage, 0, len(remoteMembers))
	for node, members := range remoteMembers {
		for _, identity := range members {
			msgs = append(msgs, &pldapi.ReliableMessage{
				Node:        node,
				MessageType: pldapi.RMTPrivacyGroup.Enum(),
				Metadata: pldtypes.JSONString(&components.PrivacyGroupDistribution{
					GenesisTransaction: txIDs[0],
					GenesisState: components.StateDistributionWithData{
						StateDistribution: components.StateDistribution{
							IdentityLocator: identity,
							Domain:          spec.Domain,
							StateID:         id.String(),
							SchemaID:        genesisSchemaID.String(),
						},
					},
				}),
			})
		}
	}
	if len(msgs) > 0 {
		if err := gm.transportManager.SendReliable(ctx, dbTX, msgs...); err != nil {
			return nil, err
		}
	}

	return group, nil
}

func (gm *groupManager) StoreReceivedGroup(ctx context.Context, dbTX persistence.DBTX, domainName string, tx uuid.UUID, state *pldapi.State) (rejectionErr, err error) {

	var pgGenesis pldapi.PrivacyGroupGenesisState
	if err := json.Unmarshal(state.Data, &pgGenesis); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgPGroupsReceivedGenesisInvalid)
	}

	// We need to call the domain to validate the state schema before we can insert it
	_, _, rejectionErr = gm.validateGroupGenesisSet(ctx, domainName, &pgGenesis, false)
	if rejectionErr != nil {
		return rejectionErr, nil
	}

	// Now do the insert
	_, err = gm.insertGroup(ctx, dbTX, domainName, state.Schema, state.ID, tx, &pgGenesis)
	return nil, err

}

func (gm *groupManager) enrichMembers(ctx context.Context, dbTX persistence.DBTX, pgs []*pldapi.PrivacyGroup) error {
	if len(pgs) == 0 {
		return nil
	}
	groupIDs := make([]pldtypes.HexBytes, len(pgs))
	for i, pg := range pgs {
		groupIDs[i] = pg.ID
	}
	var dbMembers []*persistedGroupMember
	err := dbTX.DB().WithContext(ctx).
		Where(`"group" IN ?`, groupIDs).
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

func (dbPG *persistedGroup) mapToAPI() *pldapi.PrivacyGroup {
	pg := &pldapi.PrivacyGroup{
		ID:                 dbPG.ID,
		Domain:             dbPG.Domain,
		Created:            dbPG.Created,
		Name:               dbPG.Name,
		GenesisSalt:        dbPG.GenesisSalt,
		GenesisSchema:      dbPG.GenesisSchema,
		GenesisTransaction: dbPG.GenesisTX,
	}
	if dbPG.Receipt != nil {
		pg.ContractAddress = dbPG.Receipt.ContractAddress
	}
	_ = json.Unmarshal(dbPG.Properties, &pg.Properties)
	_ = json.Unmarshal(dbPG.Configuration, &pg.Configuration)
	return pg
}

func (gm *groupManager) GetGroupByID(ctx context.Context, dbTX persistence.DBTX, domainName string, groupID pldtypes.HexBytes) (*pldapi.PrivacyGroup, error) {
	groupIDStr := fmt.Sprintf("%s:%s", domainName, groupID.String())
	pg, found := gm.deployedPGCache.Get(groupIDStr)
	if found {
		return pg, nil
	}

	groups, err := gm.QueryGroups(ctx, dbTX, query.NewQueryBuilder().Equal("domain", domainName).Equal("id", groupID).Limit(1).Query())
	if err != nil || len(groups) == 0 {
		return nil, err
	}

	pg = groups[0]

	// ONLY cache if there is a contract address set (that one-time bind is immutable, but until it happens we need to do the DB JOIN)
	if pg.ContractAddress != nil {
		gm.deployedPGCache.Set(groupIDStr, pg)
	}
	return pg, nil
}

func (gm *groupManager) GetGroupByAddress(ctx context.Context, dbTX persistence.DBTX, addr *pldtypes.EthAddress) (*pldapi.PrivacyGroup, error) {
	groups, err := gm.QueryGroups(ctx, dbTX, query.NewQueryBuilder().Equal("contractAddress", addr).Limit(1).Query())
	if err != nil || len(groups) == 0 {
		return nil, err
	}
	return groups[0], nil
}

// This function queries the groups only using what's in the DB, without allowing properties of the group to be used to do the query
func (gm *groupManager) queryGroupsCommon(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON, finalizers ...func(db *gorm.DB) *gorm.DB) ([]*pldapi.PrivacyGroup, error) {
	qw := &filters.QueryWrapper[persistedGroup, pldapi.PrivacyGroup]{
		P:           gm.p,
		DefaultSort: "-created",
		Filters:     groupDBOnlyFilters,
		Query:       jq,
		MapResult: func(dbPG *persistedGroup) (*pldapi.PrivacyGroup, error) {
			return dbPG.mapToAPI(), nil
		},
		Finalize: func(db *gorm.DB) *gorm.DB {
			for _, fn := range finalizers {
				db = fn(db)
			}
			return db.Joins("Receipt")
		},
	}
	pgs, err := qw.Run(ctx, dbTX)
	if err == nil {
		err = gm.enrichMembers(ctx, dbTX, pgs)
	}
	if err != nil {
		return nil, err
	}
	return pgs, nil
}

func (gm *groupManager) QueryGroups(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PrivacyGroup, error) {
	return gm.queryGroupsCommon(ctx, dbTX, jq)
}

func (gm *groupManager) QueryGroupsWithMember(ctx context.Context, dbTX persistence.DBTX, member string, jq *query.QueryJSON) ([]*pldapi.PrivacyGroup, error) {
	return gm.queryGroupsCommon(ctx, dbTX, jq, func(db *gorm.DB) *gorm.DB {
		return db.Joins(`LEFT JOIN "privacy_group_members" AS "pgm" ON "pgm"."group" = "privacy_groups"."id"`).Where(`"pgm".identity = ?`, member)
	})
}

func (gm *groupManager) prepareTransaction(ctx context.Context, dbTX persistence.DBTX, domain string, groupID pldtypes.HexBytes, pgTX *pldapi.PrivacyGroupEVMTX) (*pldapi.TransactionInput, error) {

	if domain == "" {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsNoDomain)
	}

	if groupID == nil {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsNoGroupID)
	}

	// Fluff up the privacy group
	pg, err := gm.GetGroupByID(ctx, dbTX, domain, groupID)
	if err != nil {
		return nil, err
	}
	if pg == nil {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsGroupNotFound, groupID)
	}
	if pg.ContractAddress == nil {
		return nil, i18n.NewError(ctx, msgs.MsgPGroupsNotReady, groupID, pg.GenesisTransaction)
	}

	// Get the domain smart contract object from domain mgr
	psc, err := gm.domainManager.GetSmartContractByAddress(ctx, dbTX, *pg.ContractAddress)
	if err != nil {
		return nil, err
	}

	// Call the domain to take the transaction details that need to be run in the privacy group, and wrap them
	// to build the transaction to call against the domain.
	return psc.WrapPrivacyGroupEVMTX(ctx, pg, pgTX)

}

func (gm *groupManager) SendTransaction(ctx context.Context, dbTX persistence.DBTX, pgTX *pldapi.PrivacyGroupEVMTXInput) (*uuid.UUID, error) {

	tx, err := gm.prepareTransaction(ctx, dbTX, pgTX.Domain, pgTX.Group, &pgTX.PrivacyGroupEVMTX)
	if err != nil {
		return nil, err
	}
	tx.IdempotencyKey = pgTX.IdempotencyKey
	tx.PublicTxOptions = pgTX.PublicTxOptions
	tx.From = pgTX.From

	txIDs, err := gm.txManager.SendTransactions(ctx, dbTX, tx)
	if err != nil {
		return nil, err
	}

	return &txIDs[0], nil

}

func (gm *groupManager) Call(ctx context.Context, dbTX persistence.DBTX, result any, call *pldapi.PrivacyGroupEVMCall) error {

	tx, err := gm.prepareTransaction(ctx, dbTX, call.Domain, call.Group, &call.PrivacyGroupEVMTX)
	if err != nil {
		return err
	}
	tx.From = call.From

	return gm.txManager.CallTransaction(ctx, dbTX, result, &pldapi.TransactionCall{
		TransactionInput:  *tx,
		PublicCallOptions: call.PublicCallOptions,
		DataFormat:        call.DataFormat,
	})

}
