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
	"github.com/kaleido-io/paladin/sdk/go/pkg/tktypes"
)

type DBEntry struct {
	Registry         string            `gorm:"column:registry;primaryKey"`
	ID               tktypes.HexBytes  `gorm:"column:id;primaryKey"`
	Name             string            `gorm:"column:name"`
	Created          tktypes.Timestamp `gorm:"column:created;autoCreateTime:nano"`
	Updated          tktypes.Timestamp `gorm:"column:updated;autoUpdateTime:nano"`
	Active           bool              `gorm:"column:active"`
	ParentID         tktypes.HexBytes  `gorm:"column:parent_id"`
	TransactionHash  *tktypes.Bytes32  `gorm:"column:tx_hash"`
	BlockNumber      *int64            `gorm:"column:block_number"`
	TransactionIndex *int64            `gorm:"column:tx_index"`
	LogIndex         *int64            `gorm:"column:log_index"`
}

func (dbe DBEntry) TableName() string {
	return "reg_entries"
}

type DBProperty struct {
	Registry         string            `gorm:"column:registry;primaryKey"`
	EntryID          tktypes.HexBytes  `gorm:"column:entry_id;primaryKey"`
	Name             string            `gorm:"column:name;primaryKey"`
	Created          tktypes.Timestamp `gorm:"column:created;autoCreateTime:nano"`
	Updated          tktypes.Timestamp `gorm:"column:updated;autoUpdateTime:nano"`
	Active           bool              `gorm:"column:active"`
	Value            string            `gorm:"column:value"`
	TransactionHash  *tktypes.Bytes32  `gorm:"column:tx_hash"`
	BlockNumber      *int64            `gorm:"column:block_number"`
	TransactionIndex *int64            `gorm:"column:tx_index"`
	LogIndex         *int64            `gorm:"column:log_index"`
}

func (dbe DBProperty) TableName() string {
	return "reg_props"
}
