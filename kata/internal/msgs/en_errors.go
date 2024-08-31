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
	// Components PD0100XX
	MsgComponentKeyManagerInitError    = ffe("PD010000", "Error initializing key manager")
	MsgComponentKeyManagerStartError   = ffe("PD010001", "Error starting key manager")
	MsgComponentEthClientInitError     = ffe("PD010002", "Error initializing ethereum client")
	MsgComponentEthClientStartError    = ffe("PD010003", "Error starting ethereum client")
	MsgComponentDBInitError            = ffe("PD010004", "Error initializing database")
	MsgComponentDBStartError           = ffe("PD010005", "Error starting database")
	MsgComponentStateStoreInitError    = ffe("PD010006", "Error initializing state store")
	MsgComponentStateStoreStartError   = ffe("PD010007", "Error starting state store")
	MsgComponentBlockIndexerInitError  = ffe("PD010008", "Error initializing block indexer")
	MsgComponentBlockIndexerStartError = ffe("PD010009", "Error starting block indexer ")
	MsgComponentRPCServerInitError     = ffe("PD010010", "Error initializing RPC server")
	MsgComponentRPCServerStartError    = ffe("PD010011", "Error starting RPC server ")
	MsgComponentDomainInitError        = ffe("PD010012", "Error initializing domains")
	MsgComponentDomainStartError       = ffe("PD010013", "Error starting domain manager")
	MsgComponentPluginCtrlInitError    = ffe("PD010014", "Error initializing plugin controller")
	MsgComponentPluginCtrlStartError   = ffe("PD010015", "Error starting plugin controller ")
	MsgComponentWaitPluginStartError   = ffe("PD010016", "Error waiting for plugins to start")
	MsgComponentEngineInitError        = ffe("PD010017", "Error initializing engine")
	MsgComponentEngineStartError       = ffe("PD010018", "Error starting engine")

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
	MsgStateInvalidQualifier          = ffe("PD010117", "Status must be one of 'available','confirmed','unconfirmed','spent','locked','all' or the UUID of a transaction")
	MsgStateLockConflictUnexpected    = ffe("PD010118", "Pending lock for transaction %s found when attempting to lock to transaction %s")
	MsgStateFlushFailedDomainReset    = ffe("PD010119", "Flush of state for domain %s has failed, and the domain has been reset")

	// Persistence PD0102XX
	MsgPersistenceInvalidType         = ffe("PD010200", "Invalid persistence type: %s")
	MsgPersistenceMissingURI          = ffe("PD010201", "Missing database connection URI")
	MsgPersistenceInitFailed          = ffe("PD010202", "Database init failed")
	MsgPersistenceMigrationFailed     = ffe("PD010203", "Database migration failed")
	MsgPersistenceMissingMigrationDir = ffe("PD010204", "Missing database migration directory for autoMigrate")

	// Transaction Processor PD0103XX
	MsgTransactionProcessorInvalidStage = ffe("PD010300", "Invalid stage: %s")
	MsgContextCanceled                  = ffe("PD010301", "Context canceled")

	// Transaction store PD0104XX
	MsgTransactionMissingField   = ffe("PD010400", "Must provide a payload (one of PayloadJSON or PayloadRLP), from, and contract address.  Mising %v")
	MsgTransactionParseError     = ffe("PD010401", "Failed to parse transaction message.")
	MsgTransactionSerializeError = ffe("PD010402", "Failed to serialise transaction response.")

	// Comms bus PD0106XX
	MsgDestinationNotFound     = ffe("PD010600", "Destination not found: %s")
	MsgHandlerError            = ffe("PD010601", "Error from message handler")
	MsgDuplicateSubscription   = ffe("PD010602", "Destination %s already subscribed to topic %s")
	MsgErrorStoppingGRPCServer = ffe("PD010603", "Error stopping GRPC server")

	// Filters PD0107XX
	MsgFiltersUnknownField                = ffe("PD010700", "Unknown field '%s'")
	MsgFiltersJSONQueryValueUnsupported   = ffe("PD010701", "JSON query value not supported: %s")
	MsgFiltersJSONQueryOpUnsupportedMod   = ffe("PD010702", "Operation '%s' does not support modifiers: %v")
	MsgFiltersValueInvalidForInt64        = ffe("PD010703", "Value '%s' cannot be parsed as a 64bit signed integer")
	MsgFiltersValueInvalidForBool         = ffe("PD010704", "Value '%s' cannot be parsed as a boolean")
	MsgFiltersValueInvalidForString       = ffe("PD010705", "Value '%s' cannot be parsed as a string")
	MsgFiltersValueInvalidForBigInt       = ffe("PD010706", "Type '%T' cannot be converted to a big integer")
	MsgFiltersValueIntStringParseFail     = ffe("PD010707", "Value '%s' cannot be converted to a big integer")
	MsgFiltersValueMissing                = ffe("PD010708", "Value missing for filter field '%s'")
	MsgFiltersMustBeBuiltUnscoped         = ffe("PD010709", "Scoped SQL builder (rather than DB) incorrect passed into filter builder")
	MsgFiltersJSONValueParseError         = ffe("PD010710", "Failed to parse value for field '%s' (as %T): %v")
	MsgFiltersValueInvalidHex             = ffe("PD010711", "Failed to parse value as hex: %v")
	MsgFiltersUnexpectedFieldResolverType = ffe("PD010712", "Unsupported type '%T' returned from field resolver '%T'")
	MsgFiltersUnexpectedResolvedValueType = ffe("PD010713", "Value type '%T' mismatched from compare type '%T'")
	MsgFiltersLikeNotSupportedForIntValue = ffe("PD010714", "LIKE operation not supported for int64 stored values")
	MsgFiltersLikeConversionToRegexpFail  = ffe("PD010715", "Failed to convert LIKE string '%s' to regexp: %s")
	MsgFiltersFieldTypeDoesNotSupportLike = ffe("PD010716", "Field does not support LIKE comparison (%T)")
	MsgFiltersTypeErrorDuringCompare      = ffe("PD010717", "Mismatched types during compare t1=%T t2=%T")
	MsgFiltersMissingSortField            = ffe("PD010718", "Must specify at least one sort field")
	MsgFiltersValueInvalidHexBytes32      = ffe("PD010719", "Failed to parse value as 32 byte hex string (parsedBytes=%d)")

	// HTTPServer PD0108XX
	MsgHTTPServerStartFailed        = ffe("PD010800", "Failed to start server on '%s'")
	MsgHTTPServerMissingPort        = ffe("PD010801", "HTTP server port must be specified for '%s'")
	MsgHTTPServerNoWSUpgradeSupport = ffe("PD010802", "HTTP server does not support WebSocket upgrade (%T)")

	// TLS PD0198XX
	MsgTLSInvalidCAFile             = ffe("PD010900", "Invalid CA certificates file")
	MsgTLSConfigFailed              = ffe("PD010901", "Failed to initialize TLS configuration")
	MsgTLSInvalidKeyPairFiles       = ffe("PD010902", "Invalid certificate and key pair files")
	MsgTLSInvalidTLSDnMatcherAttr   = ffe("PD010903", "Unknown DN attribute '%s'")
	MsgTLSInvalidTLSDnMatcherRegexp = ffe("PD010904", "Invalid regexp '%s' for requiredDNAttributes[%s]: %s")
	MsgTLSInvalidTLSDnChain         = ffe("PD010905", "Cannot match subject distinguished name as cert chain is not verified")
	MsgTLSInvalidTLSDnMismatch      = ffe("PD010906", "Certificate subject does not meet requirements")

	// JSON/RPC PD0110XX
	MsgJSONRPCInvalidRequest      = ffe("PD011000", "Invalid JSON/RPC request data")
	MsgJSONRPCMissingRequestID    = ffe("PD011001", "Invalid JSON/RPC request. Must set request ID")
	MsgJSONRPCUnsupportedMethod   = ffe("PD011002", "method not supported")
	MsgJSONRPCIncorrectParamCount = ffe("PD011003", "method %s requires %d params (supplied=%d)")
	MsgJSONRPCInvalidParam        = ffe("PD011004", "method %s parameter %d invalid: %s")
	MsgJSONRPCResultSerialization = ffe("PD011005", "method %s result serialization failed: %s")

	// Types PD0111XX
	MsgTypesUnmarshalNil                  = ffe("PD011100", "UnmarshalJSON on nil pointer")
	MsgTypesScanFail                      = ffe("PD011101", "Unable to scan type %T into type %T")
	MsgTypesEnumValueInvalid              = ffe("PD011102", "Value must be one of %s")
	MsgTypesABIDefNotInBothStructs        = ffe("PD011103", "ABI is not equal due to mismatch on: %s")
	MsgTypesInvalidNameSafeCharAlphaBoxed = ffe("PD011106", "Field '%s' must be 1-%d characters, including alphanumerics (a-zA-Z0-9), dot (.), dash (-) and underscore (_), and must start/end in an alphanumeric: %q")

	// Plugin controller PD0112XX
	MsgPluginLoaderUUIDError   = ffe("PD011200", "Plugin loader UUID incorrect")
	MsgPluginLoaderAlreadyInit = ffe("PD011201", "Plugin loader already initialized")
	MsgPluginUUIDNotFound      = ffe("PD011202", "Plugin runtime instance of type %s with UUID %s does not exist")
	MsgPluginBadRequestBody    = ffe("PD011203", "Invalid request body %T")
	MsgPluginUDSPathTooLong    = ffe("PD011204", "Unix domain socket path too log (len=%d,limit=100)")
	MsgPluginBadResponseBody   = ffe("PD011205", "%s %s returned invalid response body %T")
	MsgPluginError             = ffe("PD011206", "%s %s returned error: %s")
	MsgPluginLoadFailed        = ffe("PD011207", "Plugin load failed: %s")

	// BlockIndexer PD0113XX
	MsgBlockIndexerInvalidFromBlock         = ffe("PD011300", "Invalid from block '%s' (must be 'latest' or number)")
	MsgBlockIndexerESInitFail               = ffe("PD011303", "Event stream initialization failed")
	MsgBlockIndexerESAlreadyInit            = ffe("PD011304", "Event stream already initialized")
	MsgBlockIndexerConfirmedReceiptNotFound = ffe("PD011305", "Expected received for confirmed transaction %s not found")
	MsgBlockIndexerInvalidEventStreamType   = ffe("PD011306", "Unsupported event stream type: %s")
	MsgBlockMissingHandler                  = ffe("PD011307", "Handler not registered for stream")
	MsgBlockIndexerNoBlocksIndexed          = ffe("PD011308", "No confirmed blocks have yet been indexed")

	// Signing module PD0114XX
	MsgSigningModuleBadPathError                = ffe("PD011400", "Path '%s' does not exist, or it is not a directory")
	MsgSigningModuleBadKeyFile                  = ffe("PD011401", "Key file '%s' does not exist")
	MsgSigningModuleBadPassFile                 = ffe("PD011402", "Password file '%s' does not exist")
	MsgSigningModuleBadKeyHandle                = ffe("PD011403", "Invalid key handle")
	MsgSigningModuleFSError                     = ffe("PD011404", "Filesystem error")
	MsgSigningModuleKeyHandleClash              = ffe("PD011405", "Invalid key handle (clash)")
	MsgSigningModuleKeyNotExist                 = ffe("PD011406", "Key '%s' does not exist")
	MsgSigningUnsupportedKeyStoreType           = ffe("PD011407", "Unsupported key store type: '%s'")
	MsgSigningHierarchicalRequiresLoading       = ffe("PD011408", "Signing module has been configured to disallow in-memory key material. Hierarchical Deterministic (HD) wallet function implemented in the signing module requires in-memory key material")
	MsgSigningStoreRequiresKeyLoadingForAlgo    = ffe("PD011409", "Signing module has been configured to disallow in-memory key material. In-memory signing is required for algorithms %s")
	MsgSigningUnsupportedAlgoForInMemorySigning = ffe("PD011410", "Unsupported algorithm for in-memory signing: %s")
	MsgSigningMustSpecifyAlgorithms             = ffe("PD011411", "Must specify at least one algorithm for key resolution")
	MsgSigningHDSeedMustBe32BytesOrMnemonic     = ffe("PD011412", "Seed key material for HD Wallet must be either a 32byte value, or a BIP-39 compliant mnemonic seed phrase")
	MsgSignerBIP44DerivationInvalid             = ffe("PD011413", "Invalid key handle - BIP44 key identifier expected (invalid derivation: '%s')")
	MsgSingerBIP32DerivationTooLarge            = ffe("PD011414", "BIP-32 key index must be between 0 and 2^31-1 at each level in the hierarchy")
	MsgSigningKeyListingNotSupported            = ffe("PD011415", "Listing keys in the key store is not supported by this signing module")
	MsgSigningStaticKeyInvalid                  = ffe("PD011416", "Statically configured key with handle %s is invalid")
	MsgSigningStaticBadEncoding                 = ffe("PD011417", "Statically configured key with handle %s has invalid encoding (must be one of 'none', 'hex', 'base64') '%s'")
	MsgSigningKeyCannotBeResolved               = ffe("PD011418", "No key exists that matches the request")
	MsgSigningUnsupportedKeyDerivationType      = ffe("PD011419", "Unsupported key derivation type: '%s'")
	MsgSigningKeyCannotBeEmpty                  = ffe("PD011420", "Cannot resolve a signing key for the empty string")

	// EthClient module PD0115XX
	MsgEthClientInvalidInput        = ffe("PD011500", "Unable to convert to ABI function input (func=%s)")
	MsgEthClientMissingFrom         = ffe("PD011501", "Signer (from) missing")
	MsgEthClientMissingTo           = ffe("PD011502", "To missing")
	MsgEthClientMissingInput        = ffe("PD011503", "Input missing")
	MsgEthClientMissingOutput       = ffe("PD011504", "Output missing")
	MsgEthClientInvalidTXVersion    = ffe("PD011505", "Invalid TX Version (%s)")
	MsgEthClientABIJson             = ffe("PD011506", "JSON ABI parsing failed")
	MsgEthClientFunctionNotFound    = ffe("PD011507", "Function %q not found on ABI")
	MsgEthClientChainIDFailed       = ffe("PD011508", "Failed to query chain ID")
	MsgEthClientKeyMismatch         = ffe("PD011509", "Resolved %q to different key handle expected=%q received=%q")
	MsgEthClientToWithConstructor   = ffe("PD011510", "To address cannot be specified for constructor")
	MsgEthClientHTTPURLMissing      = ffe("PD011511", "HTTP URL missing in configuration")
	MsgEthClientChainIDMismatch     = ffe("PD011512", "ChainID mismatch between HTTP and WebSocket JSON/RPC connections http=%d ws=%d")
	MsgEthClientInvalidWebSocketURL = ffe("PD011513", "Invalid WebSocket URL: %s")
	MsgEthClientInvalidHTTPURL      = ffe("PD011514", "Invalid HTTP URL: %s")

	// DomainManager module PD0116XX
	MsgDomainNotFound                   = ffe("PD011600", "Domain %q not found")
	MsgDomainNotInitialized             = ffe("PD011601", "Domain not initialized")
	MsgDomainInvalidSchema              = ffe("PD011602", "Domain schema %d is invalid")
	MsgDomainConstructorAbiJsonInvalid  = ffe("PD011603", "Constructor ABI function definition invalid")
	MsgDomainConstructorABITypeWrong    = ffe("PD011604", "Constructor ABI function definition has wrong type: %s")
	MsgDomainFactoryAbiJsonInvalid      = ffe("PD011605", "Factory contract ABI invalid")
	MsgDomainFactoryAddressInvalid      = ffe("PD011606", "Factory contract address invalid")
	MsgDomainPrivateAbiJsonInvalid      = ffe("PD011607", "Private contract ABI invalid")
	MsgDomainInvalidQueryJSON           = ffe("PD011608", "Invalid query JSON")
	MsgDomainContractNotFoundByAddr     = ffe("PD011609", "A smart contract with address %s has not yet been indexed")
	MsgDomainInvalidConstructorParams   = ffe("PD011610", "Invalid constructor parameters for %s")
	MsgDomainInvalidPrepareDeployResult = ffe("PD011611", "Prepare deploy did not result in exactly one of a invoke transaction or a deploy transaction")
	MsgDomainInvalidFunctionParams      = ffe("PD011612", "Invalid function parameters for %s")
	MsgDomainUnknownSchema              = ffe("PD011613", "Unknown schema %s")
	MsgDomainInvalidStateIDFromDomain   = ffe("PD011614", "Domain returned invalid id '%s' for state %d")
	MsgDomainInputStateNotFound         = ffe("PD011615", "Input state %d [%s] not found")
	MsgDomainMissingStates              = ffe("PD011616", "Missing in-memory states")
	MsgDomainEndorsementReverted        = ffe("PD011617", "Endorsement from '%s' reverted: %s")
	MsgDomainFunctionNotFound           = ffe("PD011618", "Function with name '%s' not found on ABI")
	MsgDomainBaseLedgerSubmitInvalid    = ffe("PD011619", "Base ledger submission config is invalid")

	// TransportManager module PD0119XX

	// GRPC Transport plugin PD0120XX
	MsgGRPCPluginAlreadyRunning     = ffe("PD012000", "GRPC Transport is already running")
	MsgGRPCPluginCouldNotInitialize = ffe("PD012001", "Error during initialization of plugin %s")
)
