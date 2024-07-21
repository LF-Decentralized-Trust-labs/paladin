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

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"gorm.io/gorm"
)

// Queries against the state store can be made in the context of a
// sequence UUID, or one of the standard qualifiers
// (confirmed/unconfirmed/spent/all)
type StateStatusQualifier string

const StateStatusAvailable = "available"
const StateStatusConfirmed = "confirmed"
const StateStatusUnconfirmed = "unconfirmed"
const StateStatusLocked = "locked"
const StateStatusSpent = "spent"
const StateStatusAll = "all"

func (q *StateStatusQualifier) UnmarshalText(b []byte) error {
	text := strings.ToLower(string(b))
	switch text {
	case StateStatusAvailable,
		StateStatusConfirmed,
		StateStatusUnconfirmed,
		StateStatusLocked,
		StateStatusSpent,
		StateStatusAll:
		*q = StateStatusQualifier(text)
	default:
		u, err := uuid.Parse(string(text))
		if err != nil {
			return i18n.NewError(context.Background(), msgs.MsgStateInvalidQualifier)
		}
		*q = (StateStatusQualifier)(u.String())
	}
	return nil
}

func (q StateStatusQualifier) whereClause(db *gorm.DB /* must be the DB not the query */) *gorm.DB {
	switch q {
	case StateStatusAvailable:
		return db.
			Where("confirmed.transaction IS NOT NULL").
			Where("locked.sequence IS NULL")
	case StateStatusConfirmed:
		return db.
			Where("confirmed.transaction IS NOT NULL")
	case StateStatusUnconfirmed:
		return db.
			Where("confirmed.transaction IS NULL")
	case StateStatusLocked:
		return db.
			Where("locked.sequence IS NOT NULL")
	case StateStatusSpent:
		return db.
			Where("spent.transaction IS NOT NULL")
	case StateStatusAll:
		return db.Where("TRUE")
	default:
		return db.
			Where("locked.sequence = ?", q).
			Or("locked.sequence IS NULL")
	}
}
