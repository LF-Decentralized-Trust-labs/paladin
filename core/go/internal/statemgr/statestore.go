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
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

type stateManager struct {
	p                 persistence.Persistence
	bgCtx             context.Context
	cancelCtx         context.CancelFunc
	conf              *pldconf.StateStoreConfig
	domainManager     components.DomainManager
	txManager         components.TXManager
	abiSchemaCache    cache.Cache[string, components.Schema]
	rpcModule         *rpcserver.RPCModule
	domainContextLock sync.Mutex
	domainContexts    map[uuid.UUID]*domainContext
}

var SchemaCacheDefaults = &pldconf.CacheConfig{
	Capacity: confutil.P(1000),
}

func NewStateManager(ctx context.Context, conf *pldconf.StateStoreConfig, p persistence.Persistence) components.StateManager {
	ss := &stateManager{
		p:              p,
		conf:           conf,
		abiSchemaCache: cache.NewCache[string, components.Schema](&conf.SchemaCache, SchemaCacheDefaults),
		domainContexts: make(map[uuid.UUID]*domainContext),
	}
	ss.bgCtx, ss.cancelCtx = context.WithCancel(ctx)
	return ss
}

func (ss *stateManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	ss.initRPC()
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{ss.rpcModule},
	}, nil
}

func (ss *stateManager) PostInit(c components.AllComponents) error {
	ss.domainManager = c.DomainManager()
	ss.txManager = c.TxManager()
	return nil
}

func (ss *stateManager) Start() error {
	return nil
}

func (ss *stateManager) Stop() {
	ss.cancelCtx()
}

// Confirmation and spending records are not managed via the in-memory cached model of states,
// rather they are written to the database in the DB transaction of the block indexer,
// such that any failure in that DB transaction will be atomic with the writing of the records.
//
// By their nature they happen asynchronously from the coordination and assembly of new
// transactions, and it is the private transaction manager's responsibility to process
// them when notified post-commit about the domains/transactions that are affected and
// might have in-memory processing.
//
// As such, no attempt is made to coordinate these changes with the queries that might
// be happening concurrently against the database, and after commit of these changes
// might find new states become available and/or states marked locked for spending
// become fully unavailable.
func (ss *stateManager) WriteStateFinalizations(ctx context.Context, dbTX persistence.DBTX, spends []*pldapi.StateSpendRecord, reads []*pldapi.StateReadRecord, confirms []*pldapi.StateConfirmRecord, infoRecords []*pldapi.StateInfoRecord) (err error) {
	if len(spends) > 0 {
		err = dbTX.DB().
			WithContext(ctx).
			Table("state_spend_records").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(spends).
			Error
	}
	if err == nil && len(reads) > 0 {
		err = dbTX.DB().
			WithContext(ctx).
			Table("state_read_records").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(reads).
			Error
	}
	if err == nil && len(confirms) > 0 {
		err = dbTX.DB().
			WithContext(ctx).
			Table("state_confirm_records").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(confirms).
			Error
	}
	if err == nil && len(infoRecords) > 0 {
		err = dbTX.DB().
			WithContext(ctx).
			Table("state_info_records").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(infoRecords).
			Error
	}
	return err
}

func (ss *stateManager) GetTransactionStates(ctx context.Context, dbTX persistence.DBTX, txID uuid.UUID) (*pldapi.TransactionStates, error) {

	// We query from the records table, joining in the other fields
	var records []*transactionStateRecord
	err := dbTX.DB().
		WithContext(ctx).
		// This query joins across three tables in a single query - pushing the complexity to the DB.
		// The reason we have three tables is to make the queries for available states simpler.
		Raw(`SELECT * from "states" RIGHT JOIN ( `+
			`SELECT "transaction", "state", 'spent'     AS "record_type" FROM "state_spend_records"   WHERE "transaction" = ? UNION ALL `+
			`SELECT "transaction", "state", 'read'      AS "record_type" FROM "state_read_records"    WHERE "transaction" = ? UNION ALL `+
			`SELECT "transaction", "state", 'confirmed' AS "record_type" FROM "state_confirm_records" WHERE "transaction" = ? UNION ALL `+
			`SELECT "transaction", "state", 'info'      AS "record_type" FROM "state_info_records"    WHERE "transaction" = ? ) "records" `+
			`ON "states"."id" = "records"."state"`,
			txID, txID, txID, txID).
		Scan(&records).
		Error
	if err != nil {
		return nil, err
	}
	hasUnavailable := false
	unavailable := &pldapi.UnavailableStates{}
	txStates := &pldapi.TransactionStates{
		None: len(records) == 0, // if we have no confirmation records at all then this is an unknown transaction
	}
	for _, s := range records {
		switch s.RecordType {
		case "spent":
			if s.ID == nil {
				hasUnavailable = true
				unavailable.Spent = append(unavailable.Spent, s.State)
			} else {
				txStates.Spent = append(txStates.Spent, &s.StateBase)
			}
		case "read":
			if s.ID == nil {
				hasUnavailable = true
				unavailable.Read = append(unavailable.Read, s.State)
			} else {
				txStates.Read = append(txStates.Read, &s.StateBase)
			}
		case "confirmed":
			if s.ID == nil {
				hasUnavailable = true
				unavailable.Confirmed = append(unavailable.Confirmed, s.State)
			} else {
				txStates.Confirmed = append(txStates.Confirmed, &s.StateBase)
			}
		case "info":
			if s.ID == nil {
				hasUnavailable = true
				unavailable.Info = append(unavailable.Info, s.State)
			} else {
				txStates.Info = append(txStates.Info, &s.StateBase)
			}
		}
	}
	// Only set to non-nil if we have unavailable
	if hasUnavailable {
		txStates.Unavailable = unavailable
	}
	return txStates, nil

}
