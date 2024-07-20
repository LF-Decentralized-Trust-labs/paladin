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
	MsgStateLabelFieldUnexpectedValue = ffe("PD010109", "Value type for field %s %T from ABI decoding library does not match expected value type %T")
	MsgStateLabelFieldMissing         = ffe("PD010110", "Label field %s missing")
	MsgStateLabelFieldNotSupported    = ffe("PD010111", "Label field %s is not a supported elementary type (%s)")
	MsgStateNotFound                  = ffe("PD010112", "State not found with hash %s")
	MsgStateInvalidSchema             = ffe("PD010113", "Invalid schema")
	MsgStateABITypeMustBeTuple        = ffe("PD010114", "ABI type definition must be a tuple parameter with an internalType such as 'struct StructName'")
	MsgStateLabelFieldNotUnique       = ffe("PD010115", "Label field with index %d has a duplicate name '%s'")
	MsgStateInvalidValue              = ffe("PD010116", "Invalid value")

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
	MsgFiltersValueInvalidForBigInt     = ffe("PD010306", "Type '%T' cannot be converted to a big integer")
	MsgFiltersValueIntStringParseFail   = ffe("PD010307", "Value '%s' cannot be converted to a big integer")
	MsgFiltersValueMissing              = ffe("PD010308", "Value missing for filter field '%s'")
	MsgFiltersMustBeBuiltUnscoped       = ffe("PD010309", "Scoped SQL builder (rather than DB) incorrect passed into filter builder")
	MsgFiltersJSONValueParseError       = ffe("PD010310", "Failed to parse value for field '%s' (as %T): %v")
	MsgFiltersValueInvalidHex           = ffe("PD010311", "Failed to parse value as hex: %v")

	// Transaction store PD0104XX
	MsgTransactionMissingField = ffe("PD010400", "Must provide a payload (one of PayloadJSON or PayloadRLP), from, and contract address.  Mising %v")

	// Config PD0105XX
	MsgConfigFileMissing    = ffe("PD010500", "Config file not found at path: %s")
	MsgConfigFileReadError  = ffe("PD010501", "Failed to read config file %s with error: %s")
	MsgConfigFileParseError = ffe("PD010502", "Failed to parse config file %s with error: %s")

	// HTTPServer PD0106XX
	MsgHTTPServerStartFailed        = ffe("PD010600", "Failed to start server on '%s'")
	MsgHTTPServerMissingPort        = ffe("PD010601", "HTTP server port must be specified for '%s'")
	MsgHTTPServerNoWSUpgradeSupport = ffe("PD010602", "HTTP server does not support WebSocket upgrade (%T)")

	// TLS PD0107XX
	MsgTLSInvalidCAFile             = ffe("PD010700", "Invalid CA certificates file")
	MsgTLSConfigFailed              = ffe("PD010701", "Failed to initialize TLS configuration")
	MsgTLSInvalidKeyPairFiles       = ffe("PD010702", "Invalid certificate and key pair files")
	MsgTLSInvalidTLSDnMatcherAttr   = ffe("PD010703", "Unknown DN attribute '%s'")
	MsgTLSInvalidTLSDnMatcherType   = ffe("PD010704", "Expected string value for '%s' field of requiredDNAttributes (found %T)")
	MsgTLSInvalidTLSDnMatcherRegexp = ffe("PD010705", "Invalid regexp '%s' for requiredDNAttributes[%s]: %s")
	MsgTLSInvalidTLSDnChain         = ffe("PD010706", "Cannot match subject distinguished name as cert chain is not verified")
	MsgTLSInvalidTLSDnMismatch      = ffe("PD010707", "Certificate subject does not meet requirements")

	// JSON/RPC PD0108XX
	MsgJSONRPCInvalidRequest      = ffe("PD010800", "Invalid JSON/RPC request data")
	MsgJSONRPCMissingRequestID    = ffe("PD010801", "Invalid JSON/RPC request. Must set request ID")
	MsgJSONRPCUnsupportedMethod   = ffe("PD010802", "method not supported")
	MsgJSONRPCIncorrectParamCount = ffe("PD010803", "method %s requires %d params (supplied=%d)")
	MsgJSONRPCInvalidParam        = ffe("PD010804", "method %s parameter %d invalid: %s")
	MsgJSONRPCResultSerialization = ffe("PD010805", "method %s result serialization failed: %s")
)
