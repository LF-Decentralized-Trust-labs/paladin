// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package msgs

import (
	"fmt"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"golang.org/x/text/language"
)

const kataPrefix = "PD01"

var registered = false
var ffe = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	if !registered {
		i18n.RegisterPrefix(kataPrefix, "Paladin Transaction Manager")
		registered = true
	}
	if !strings.HasPrefix(key, kataPrefix) {
		panic(fmt.Errorf("must have prefix '%s': %s", kataPrefix, key))
	}
	return i18n.FFE(language.AmericanEnglish, key, translation, statusHint...)
}

var (

	// States PD0101XX
	MsgStateInvalidHex                = ffe("PD010100", "Invalid hex: %s")
	MsgStateInvalidLength             = ffe("PD010101", "Invalid hash len expected=%d actual=%d")
	MsgStateInvalidABIParam           = ffe("PD010102", "Invalid ABI parameter")
	MsgStateInvalidSchemaType         = ffe("PD010103", "Invalid state schema type: %s")
	MsgStateManagerQuiescing          = ffe("PD010104", "State store shutting down")
	MsgStateOpInvalid                 = ffe("PD010105", "State operation invalid")
	MsgStateSchemaNotFound            = ffe("PD010106", "Schema not found with hash %s")
	MsgStateLabelFieldNotElementary   = ffe("PD010107", "Label field %s is not elementary type (%s)")
	MsgStateLabelFieldNotNamed        = ffe("PD010108", "Label field with index %d is not named")
	MsgStateLabelFieldUnsupportedType = ffe("PD010109", "Text label field %s has unsupported type '%s'")
	MsgStateLabelFieldMissing         = ffe("PD010110", "Label field %s missing")

	// Persistence PD0102XX
	MsgPersistenceInvalidType         = ffe("PD010200", "Invalid persistence type: %s")
	MsgPersistenceMissingURI          = ffe("PD010201", "Missing database connection URI")
	MsgPersistenceInitFailed          = ffe("PD010202", "Database init failed")
	MsgPersistenceMigrationFailed     = ffe("PD010203", "Database migration failed")
	MsgPersistenceMissingMigrationDir = ffe("PD010204", "Missing database migration directory for autoMigrate")

	// Filters PD0103XX
	MsgFiltersUnknownField              = ffe("PD010300", "Unknown field '%s'")
	MsgFiltersJSONQueryValueUnsupported = ffe("PD010301", "JSON query value not supported: %s")
	MsgFiltersJSONQueryOpUnsupportedMod = ffe("PD010302", "Operation '%s' does not support modifiers: %v")
	MsgFiltersValueInvalidForInt64      = ffe("PD010303", "Value '%s' cannot be parsed as a 64bit signed integer")
	MsgFiltersValueInvalidForBool       = ffe("PD010304", "Value '%s' cannot be parsed as a boolean")
	MsgFiltersValueInvalidForString     = ffe("PD010305", "Value '%s' cannot be parsed as a string")
	MsgFiltersValueMissing              = ffe("PD010306", "Value missing for filter field '%s'")
	MsgFiltersMustBeBuiltUnscoped       = ffe("PD010307", "Scoped SQL builder (rather than DB) incorrect passed into filter builder")
	MsgFiltersJSONValueParseError       = ffe("PD010308", "Failed to parse value for field '%s' (as %T): %v")
)
