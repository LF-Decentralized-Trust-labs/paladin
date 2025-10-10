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
	"fmt"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"gorm.io/gorm"
)

// Only called for one of the static qualifiers - not for a domain context
func whereClauseForQual(db *gorm.DB /* must be the DB not the query */, q pldapi.StateStatusQualifier, spentColumn string) (*gorm.DB, bool) {
	switch q {
	case pldapi.StateStatusAvailable:
		return db.
				Where(fmt.Sprintf(`"%s"."transaction" IS NULL`, spentColumn)).
				Where(`"Confirmed"."transaction" IS NOT NULL`),
			true
	case pldapi.StateStatusConfirmed:
		return db.
				Where(`"Confirmed"."transaction" IS NOT NULL`).
				Where(fmt.Sprintf(`"%s"."transaction" IS NULL`, spentColumn)),
			true
	case pldapi.StateStatusUnconfirmed:
		return db.
				Where(`"Confirmed"."transaction" IS NULL`),
			true
	case pldapi.StateStatusSpent:
		return db.
				Where(fmt.Sprintf(`"%s"."transaction" IS NOT NULL`, spentColumn)),
			true
	case pldapi.StateStatusAll:
		return db.Where("TRUE"),
			true
	default:
		// This is a domain context query - so the caller should pass it to the appropriate domain context
		// rather than just executing the query directly against the DB
		return nil, false
	}
}
