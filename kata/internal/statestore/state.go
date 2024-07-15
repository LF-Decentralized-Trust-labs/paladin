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

import "github.com/kaleido-io/paladin/kata/internal/types"

type State struct {
	Hash      HashID          `gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
	CreatedAt types.Timestamp `gorm:"autoUpdateTime:nano"`
	UpdatedAt types.Timestamp `gorm:"autoCreateTime:nano"`
	DomainID  string
	Schema    HashID `gorm:"embedded;embeddedPrefix:schema_;"`
}

type StateUpdate struct {
	ID        int64           `gorm:"primaryKey;autoIncrement;"`
	CreatedAt types.Timestamp `gorm:"autoUpdateTime:nano"`
	UpdatedAt types.Timestamp `gorm:"autoCreateTime:nano"`
	Status    string
	Ref       string
	State     HashID `gorm:"embedded;embeddedPrefix:state_;"`
}
