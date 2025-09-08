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

package txmgr

import (
	"context"
	"encoding/json"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"gorm.io/gorm/clause"
)

type PersistedABI struct {
	Hash    pldtypes.Bytes32   `gorm:"column:hash"`
	ABI     pldtypes.RawJSON   `gorm:"column:abi"`
	Created pldtypes.Timestamp `gorm:"column:created;autoCreateTime:nano"`
}

type PersistedABIEntry struct {
	Selector   pldtypes.HexBytes `gorm:"column:selector"`
	Type       string            `gorm:"column:type"`
	FullHash   pldtypes.HexBytes `gorm:"column:full_hash"`
	ABIHash    pldtypes.Bytes32  `gorm:"column:abi_hash"`
	Definition pldtypes.RawJSON  `gorm:"column:definition"`
}

var abiFilters = filters.FieldMap{
	"id":      filters.UUIDField("id"),
	"created": filters.TimestampField("created"),
}

func (tm *txManager) getABIByHash(ctx context.Context, dbTX persistence.DBTX, hash pldtypes.Bytes32) (*pldapi.StoredABI, error) {
	pa, found := tm.abiCache.Get(hash)
	if found {
		return pa, nil
	}
	var pABIs []*PersistedABI
	err := dbTX.DB().
		WithContext(ctx).
		Table("abis").
		Where("hash = ?", hash).
		Find(&pABIs).
		Error
	if err != nil || len(pABIs) == 0 {
		return nil, err
	}
	pa = &pldapi.StoredABI{Hash: hash}
	if err = json.Unmarshal(pABIs[0].ABI, &pa.ABI); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrInvalidStoredData)
	}
	tm.abiCache.Set(hash, pa)
	return pa, nil
}

func (tm *txManager) storeABINewDBTX(ctx context.Context, a abi.ABI) (hash *pldtypes.Bytes32, err error) {
	err = tm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		hash, err = tm.storeABI(ctx, dbTX, a)
		return err
	})
	return hash, err

}

func (tm *txManager) storeABI(ctx context.Context, dbTX persistence.DBTX, a abi.ABI) (*pldtypes.Bytes32, error) {
	pa, err := tm.UpsertABI(ctx, dbTX, a)
	if err != nil {
		return nil, err
	}
	return &pa.Hash, err
}

func (tm *txManager) UpsertABI(ctx context.Context, dbTX persistence.DBTX, a abi.ABI) (*pldapi.StoredABI, error) {
	hash, err := pldtypes.ABISolDefinitionHash(ctx, a)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrInvalidABI)
	}

	// If cached, nothing to do (note must not cache until written for this to be true)
	pa, existing := tm.abiCache.Get(*hash)
	if existing {
		log.L(ctx).Debugf("ABI %s already cached", hash)
		return pa, nil
	}

	// Grab all the error definitions for reverse lookup
	var abiEntries []*PersistedABIEntry
	for _, entry := range a {
		fullHash, _ := entry.SignatureHashCtx(ctx)
		defBytes, _ := json.Marshal(entry)
		if fullHash != nil && len(defBytes) > 0 { // note we've already validated it in ABISolDefinitionHash
			abiEntries = append(abiEntries, &PersistedABIEntry{
				ABIHash:    *hash,
				Type:       string(entry.Type),
				Selector:   pldtypes.HexBytes(fullHash[0:4]),
				FullHash:   pldtypes.HexBytes(fullHash),
				Definition: defBytes,
			})
		}
	}

	// Otherwise ask the DB to store
	abiBytes, err := json.Marshal(a)
	if err == nil {
		err = dbTX.DB().
			Table("abis").
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "hash"},
				},
				DoNothing: true, // immutable
			}).
			Create(&PersistedABI{
				Hash: *hash,
				ABI:  abiBytes,
			}).
			Error
	}
	if err == nil && len(abiEntries) > 0 {
		err = dbTX.DB().
			Table("abi_entries").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(abiEntries).
			Error
	}
	if err != nil {
		return nil, err
	}
	pa = &pldapi.StoredABI{Hash: *hash, ABI: a}
	dbTX.AddPostCommit(func(ctx context.Context) {
		// Caching must only be done post-commit of the DB transaction
		tm.abiCache.Set(*hash, pa)
	})
	return pa, err
}

func (tm *txManager) queryABIs(ctx context.Context, jq *query.QueryJSON) ([]*pldapi.StoredABI, error) {
	qw := &filters.QueryWrapper[PersistedABI, pldapi.StoredABI]{
		P:           tm.p,
		Table:       "abis",
		DefaultSort: "-created",
		Filters:     abiFilters,
		Query:       jq,
		MapResult: func(pa *PersistedABI) (*pldapi.StoredABI, error) {
			var a abi.ABI
			err := json.Unmarshal(pa.ABI, &a)
			return &pldapi.StoredABI{
				Hash: pa.Hash,
				ABI:  a,
			}, err
		},
	}
	return qw.Run(ctx, nil)
}
