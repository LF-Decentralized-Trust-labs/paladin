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
	MsgStateInvalidHex                 = ffe("PD010100", "Invalid hex: %s")
	MsgStateInvalidLength              = ffe("PD010101", "Invalid hash len expected=%d actual=%d")
	MsgStateInvalidABIParam            = ffe("PD010102", "Invalid ABI parameter")
	MsgStateInvalidSchemaType          = ffe("PD010103", "Invalid state schema type: %s")
	MsgStateManagerQuiescing           = ffe("PD010104", "State store shutting down")
	MsgStateOpInvalid                  = ffe("PD010105", "State operation invalid")
	MsgStateSchemaNotFound             = ffe("PD010106", "Schema not found with hash %s")
	MsgStateLabelFieldNotElementary    = ffe("PD010107", "Label field %s is not elementary type (%s)")
	MsgStateLabelFieldUnsupportedValue = ffe("PD010108", "Label field %s has unsupported value type (%T)")
	MsgStateLabelFieldMissing          = ffe("PD010110", "Label field %s missing")

	// Persistence PD0102XX
	MsgPersistenceInvalidType         = ffe("PD010200", "Invalid persistence type: %s")
	MsgPersistenceMissingURI          = ffe("PD010201", "Missing database connection URI")
	MsgPersistenceInitFailed          = ffe("PD010202", "Database init failed")
	MsgPersistenceMigrationFailed     = ffe("PD010203", "Database migration failed")
	MsgPersistenceMissingMigrationDir = ffe("PD010204", "Missing database migration directory for autoMigrate")

	// Transaction Processor PD0103XX
	MsgTransactionProcessorInvalidStage        = ffe("PD010300", "Invalid stage: %s")
	MsgTransactionProcessorUndeterminedStage   = ffe("PD010301", "Cannot determine a processing stage for transaction with ID: %s")
	MsgTransactionProcessorActionFailed        = ffe("PD010302", "Action for transaction with ID: %s at stage %s failed")
	MsgTransactionProcessorBlockedOnDependency = ffe("PD010303", "Transaction with ID: %s cannot be processed by %s stage due to dependent transactions.")

	// Transaction store PD0104XX
	MsgTransactionMissingField = ffe("PD010400", "Must provide a payload (one of PayloadJSON or PayloadRLP), from, and contract address.  Mising %v")

	// Config PD0105XX
	MsgConfigFileMissing    = ffe("PD010500", "Config file not found at path: %s")
	MsgConfigFileReadError  = ffe("PD010501", "Failed to read config file %s with error: %s")
	MsgConfigFileParseError = ffe("PD010502", "Failed to parse config file %s with error: %s")
)
