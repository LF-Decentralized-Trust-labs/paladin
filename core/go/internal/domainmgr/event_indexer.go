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

package domainmgr

import (
	"context"
	"encoding/json"

	_ "embed"

	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (dm *domainManager) eventIndexer(ctx context.Context, tx *gorm.DB, batch *blockindexer.EventDeliveryBatch) (blockindexer.PostCommit, error) {

	var contracts []*PrivateSmartContract

	for _, ev := range batch.Events {
		// We compare against the fully qualified string provided by the blockindexer at serialization time,
		// which includes variables names and whether fields are indexed
		switch ev.SoliditySignature {
		case eventSolSig_PaladinRegisterSmartContract_V0:
			var parsedEvent event_PaladinRegisterSmartContract_V0
			parseErr := json.Unmarshal(ev.Data, &parsedEvent)
			if parseErr != nil {
				log.L(ctx).Errorf("Failed to parse domain event (%s): %s", parseErr, tktypes.JSONString(ev))
				continue
			}
			contracts = append(contracts, &PrivateSmartContract{
				DeployTX:        parsedEvent.TXId.UUIDFirst16(),
				RegistryAddress: ev.Address,
				Address:         parsedEvent.Instance,
				ConfigBytes:     parsedEvent.Config,
			})
		}
	}

	if len(contracts) > 0 {
		// We have some contracts to persist
		err := tx.
			Table("private_smart_contracts").
			WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "address"}},
				DoNothing: true, // immutable
			}).
			Create(contracts).
			Error
		if err != nil {
			return nil, err
		}
	}

	return func() {
		dm.notifyTransactions(contracts)
	}, nil
}

func (dm *domainManager) notifyTransactions(contracts []*PrivateSmartContract) {
	for _, c := range contracts {
		inflight := dm.contractWaiter.GetInflight(c.DeployTX)
		if inflight != nil {
			inflight.Complete(c)
		}
	}
}
