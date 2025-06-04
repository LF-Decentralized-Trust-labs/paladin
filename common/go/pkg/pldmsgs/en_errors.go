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

package pldmsgs

import (
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

var pde = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	return i18n.PDE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	// Types PD0200XX
	MsgContextCanceled                       = pde("PD020000", "Context canceled")
	MsgTypesUnmarshalNil                     = pde("PD020001", "UnmarshalJSON on nil pointer")
	MsgTypesScanFail                         = pde("PD020002", "Unable to scan type %T into type %T")
	MsgTypesEnumValueInvalid                 = pde("PD020003", "Value must be one of %s")
	MsgTypesABIDefNotInBothStructs           = pde("PD020004", "ABI is not equal due to mismatch on: %s")
	MsgTypesInvalidNameSafeCharAlphaBoxed    = pde("PD020005", "Field '%s' must be 1-%d characters, including alphanumerics (a-zA-Z0-9), dot (.), dash (-) and underscore (_), and must start/end in an alphanumeric: %q")
	MsgTypesPrivateIdentityLocatorInvalid    = pde("PD020006", "Locator string %s is invalid")
	MsgTypesInvalidHex                       = pde("PD020007", "Invalid hex: %s")
	MsgTypesValueInvalidHexBytes32           = pde("PD020008", "Failed to parse value as 32 byte hex string (parsedBytes=%d)")
	MsgTypesInvalidHexInteger                = pde("PD020009", "Invalid integer: %s")
	MsgTypesInvalidUint64                    = pde("PD020010", "Integer cannot be converted to uint64 without losing precision: %s")
	MsgTypesInvalidDBInt64                   = pde("PD020011", "Integer too large for storage in a signed int64 database column: %s")
	MsgTypesInvalidDBInt256                  = pde("PD020012", "Integer incorrectly serialized to the database for a int256: %s")
	MsgTypesInvalidDBUint256                 = pde("PD020013", "Integer incorrectly serialized to the database for a uint256: %s")
	MsgTypesInvalidJSONFormatOptions         = pde("PD020014", "The JSON formatting options must be a valid set of key=value pairs in URL query string format '%s'")
	MsgTypesUnknownJSONFormatOptions         = pde("PD020015", "JSON formatting option unknown %s=%s")
	MsgTypesInvalidStateQualifier            = pde("PD020016", "Status must be one of 'available','confirmed','unconfirmed','spent','locked','all' or the UUID of a transaction")
	MsgTypesPrivateIdentityReqFullyQualified = pde("PD020017", "Locator string %s must be fully qualified with a node name")
	MsgTypesRestoreFailed                    = pde("PD020018", "Failed to restore type '%T' into '%T'")
	MsgTypesTimeParseFail                    = pde("PD020019", "Cannot parse time as RFC3339, Unix, or UnixNano: '%s'", 400)
	MsgTypesInvalidJSONObjectForABIInference = pde("PD020020", "Failed to parse JSON while inferring ABI types of properties")
	MsgTypesTypeInferenceNotSupportedForX    = pde("PD020021", "ABI type inference not supported for '%s' property of type %T")
	MsgTypesNumberTypeInferenceRequiresInt   = pde("PD020022", "ABI type inference only support integer JSON numbers. Property '%s' has non-integer value '%s'")
	MsgTypesCannotInferTypeOfEmptyArray      = pde("PD020023", "ABI type inference cannot determine type of empty array '%s'")
	MsgBigIntParseFailed                     = pde("PD020024", "Failed to parse JSON value '%s' into BigInt")
	MsgBigIntTooLarge                        = pde("PD020025", "Byte length of serialized integer is too large %d (max=%d)")
	MsgTypeRestoreFailed                     = pde("PD020026", "Failed to restore type '%T' into '%T'")

	// Inflight PD0201XX
	MsgInflightRequestCancelled = pde("PD020100", "Request cancelled after %s")

	// PldClient module PD0202XX
	MsgPaladinClientInvalidInput      = pde("PD020200", "Unable to convert to ABI function input (%s)")
	MsgPaladinClientMissingFrom       = pde("PD020201", "From (signing key identifier) missing")
	MsgPaladinClientMissingTo         = pde("PD020202", "To address missing for function '%s'")
	MsgPaladinClientMissingInput      = pde("PD020203", "Input missing for %s")
	MsgPaladinClientMissingOutput     = pde("PD020204", "Output missing")
	MsgPaladinClientBytecodeWithPriv  = pde("PD020205", "Bytecode cannot be specified with a private contract deployment")
	MsgPaladinClientBytecodeMissing   = pde("PD020206", "Bytecode required to deploy a public smart contract")
	MsgPaladinClientABIJson           = pde("PD020207", "JSON ABI parsing failed")
	MsgPaladinClientFunctionNotFound  = pde("PD020208", "Function %q not found on ABI")
	MsgPaladinClientToWithConstructor = pde("PD020209", "To address cannot be specified for constructor")
	MsgPaladinClientNoConnection      = pde("PD020210", "No JSON/RPC connection is available to this client")
	MsgPaladinClientMissingType       = pde("PD020211", "Type missing (public or private)")
	MsgPaladinClientNoFailureMsg      = pde("PD020212", "No failure message available")
	MsgPaladinClientNoABISupplied     = pde("PD020213", "No ABI supplied")
	MsgPaladinClientNoDomain          = pde("PD020214", "No domain specified for private transaction")
	MsgPaladinClientNoFunction        = pde("PD020215", "No function specified")
	MsgPaladinClientPollTxTimedOut    = pde("PD020216", "Polling timed out after %d attempts in %s for transaction %s")
	MsgPaladinClientWebSocketRequired = pde("PD020217", "WebSocket connection required for async notifications")

	// Plugin PD0203XX
	MsgPluginUnsupportedRequest   = pde("PD020300", "Unsupported request %T")
	MsgPluginUnexpectedResponse   = pde("PD020301", "Unexpected response %T (expected %T)")
	MsgPluginUnimplementedRequest = pde("PD020302", "Unimplemented plugin request %T")
	MsgPluginErrorFromServerNoMsg = pde("PD020303", "Error from server (no detailed message in response)")

	// TLS PD0204XX
	MsgTLSInvalidCAFile             = pde("PD020400", "Invalid CA certificates file")
	MsgTLSConfigFailed              = pde("PD020401", "Failed to initialize TLS configuration")
	MsgTLSInvalidKeyPairFiles       = pde("PD020402", "Invalid certificate and key pair files")
	MsgTLSInvalidTLSDnMatcherAttr   = pde("PD020403", "Unknown DN attribute '%s'")
	MsgTLSInvalidTLSDnMatcherRegexp = pde("PD020404", "Invalid regexp '%s' for requiredDNAttributes[%s]: %s")
	MsgTLSInvalidTLSDnChain         = pde("PD020405", "Cannot match subject distinguished name as cert chain is not verified")
	MsgTLSInvalidTLSDnMismatch      = pde("PD020406", "Certificate subject does not meet requirements")

	// RPCClient PD0205XX
	MsgRPCClientInvalidWebSocketURL      = pde("PD020500", "Invalid WebSocket URL: %s")
	MsgRPCClientInvalidHTTPURL           = pde("PD020501", "Invalid HTTP URL: %s")
	MsgRPCClientRequestFailed            = pde("PD020502", "Backend RPC request failed: %s")
	MsgRPCClientWebSocketReconnected     = pde("PD020503", "WebSocket reconnected during JSON/RPC call")
	MsgRPCClientResultParseFailed        = pde("PD020504", "Failed to parse result (expected=%T): %s")
	MsgRPCClientInvalidParam             = pde("PD020505", "Invalid parameter at position %d for method %s: %s")
	MsgRPCClientSubscribeResponseInvalid = pde("PD020506", "Subscription response invalid")

	// HTTPServer PD0108XX
	MsgHTTPServerStartFailed        = pde("PD020600", "Failed to start server on '%s'")
	MsgHTTPServerMissingPort        = pde("PD020601", "HTTP server port must be specified for '%s'")
	MsgHTTPServerNoWSUpgradeSupport = pde("PD020602", "HTTP server does not support WebSocket upgrade (%T)")
	MsgUIServerFailed               = pde("PD020603", "HTTP server failed to load index file", 500)

	// JSON/RPC PD0207XX
	MsgJSONRPCInvalidRequest      = pde("PD020700", "Invalid JSON/RPC request data")
	MsgJSONRPCMissingRequestID    = pde("PD020701", "Invalid JSON/RPC request. Must set request ID")
	MsgJSONRPCUnsupportedMethod   = pde("PD020702", "method not supported %s")
	MsgJSONRPCIncorrectParamCount = pde("PD020703", "method %s requires %d params (supplied=%d)")
	MsgJSONRPCInvalidParam        = pde("PD020704", "method %s parameter %d invalid: %s")
	MsgJSONRPCResultSerialization = pde("PD020705", "method %s result serialization failed: %s")
	MsgJSONRPCAysncNonWSConn      = pde("PD020706", "method %s only available on WebSocket connections")

	// Signing module PD0208XX
	MsgSigningModuleBadPathError                = pde("PD020800", "Path '%s' does not exist, or it is not a directory")
	MsgSigningModuleBadKeyFile                  = pde("PD020801", "Key file '%s' does not exist")
	MsgSigningModuleBadPassFile                 = pde("PD020802", "Password file '%s' does not exist")
	MsgSigningModuleBadKeyHandle                = pde("PD020803", "Invalid key handle")
	MsgSigningModuleFSError                     = pde("PD020804", "Filesystem error")
	MsgSigningModuleKeyHandleClash              = pde("PD020805", "Invalid key handle (clash)")
	MsgSigningModuleKeyNotExist                 = pde("PD020806", "Key '%s' does not exist")
	MsgSigningUnsupportedKeyStoreType           = pde("PD020807", "Unsupported key store type: '%s'")
	MsgSigningHierarchicalRequiresLoading       = pde("PD020808", "Signing module has been configured to disallow in-memory key material. Hierarchical Deterministic (HD) wallet function implemented in the signing module requires in-memory key material")
	MsgSigningKeyStoreNoInStoreSingingSupport   = pde("PD020809", "They configured key store '%s' does not support signing within the keystore itself (keys must be loaded into memory in the module to sign)")
	MsgSigningUnsupportedAlgoForInMemorySigning = pde("PD020810", "Unsupported algorithm for in-memory signing: %s")
	MsgSigningMustSpecifyAlgorithms             = pde("PD020811", "Must specify at least one algorithm for key resolution")
	MsgSigningHDSeedMustBe32BytesOrMnemonic     = pde("PD020812", "Seed key material for HD Wallet must be either a 32byte value, or a BIP-39 compliant mnemonic seed phrase")
	MsgSignerBIP44DerivationInvalid             = pde("PD020813", "Invalid key handle - BIP44 key identifier expected (invalid derivation: '%s')")
	MsgSignerBIP32DerivationTooLarge            = pde("PD020814", "BIP-32 key index must be between 0 and 2^31-1 at each level in the hierarchy")
	MsgSigningKeyListingNotSupported            = pde("PD020815", "Listing keys in the key store is not supported by this signing module")
	MsgSigningStaticKeyInvalid                  = pde("PD020816", "Statically configured key with handle %s is invalid")
	MsgSigningStaticBadEncoding                 = pde("PD020817", "Statically configured key with handle %s has invalid encoding (must be one of 'none', 'hex', 'base64') '%s'")
	MsgSigningKeyCannotBeResolved               = pde("PD020818", "No key exists that matches the request")
	MsgSigningUnsupportedKeyDerivationType      = pde("PD020819", "Unsupported key derivation type: '%s'")
	MsgSigningKeyCannotBeEmpty                  = pde("PD020820", "Cannot resolve a signing key for the empty string")
	MsgSigningFailedToLoadStaticKeyFile         = pde("PD020821", "Failed to load static key file")
	MsgSigningUnsupportedECDSACurve             = pde("PD020822", "Unsupported ECDSA curve: '%s'")
	MsgSigningUnsupportedVerifierCombination    = pde("PD020823", "Unsupported verifier type '%s' for algorithm '%s'")
	MsgSigningUnsupportedPayloadCombination     = pde("PD020824", "Unsupported payload type '%s' for algorithm '%s'")
	MsgSigningEmptyPayload                      = pde("PD020825", "No payload supplied for signing")
	MsgSigningInvalidDomainAlgorithmNoPrefix    = pde("PD020826", "Invalid domain algorithm (no 'domain:' prefix): %s")
	MsgSigningNoDomainRegisteredWithModule      = pde("PD020827", "Domain '%s' has not been registered in this signing module")

	// Reference markdown PD0209XX
	MsgReferenceMarkdownMissing = pde("PD020900", "Reference markdown file missing: '%s'")
	MsgFieldDescriptionMissing  = pde("PD020901", "Missing description for field '%s' in struct '%s'")

	// SolUtils module PD0210XX
	MsgSolBuildParseFailed = pde("PD021000", "Invalid link hash at position %d in bytecode. Fully qualified lib name: %s. Placeholder: %s. Lib name hash prefix: %s")
	MsgSolBuildMissingLink = pde("PD021001", "The solidity build is unlinked and requires an address for '%s'")

	// WSClient module PD0211XX
	MsgWSClientInvalidWebSocketURL = pde("PD021100", "Invalid WebSocket URL: %s")
	MsgWSClientSendTimedOut        = pde("PD021101", "Websocket send timed out")
	MsgWSClientClosing             = pde("PD021102", "Websocket closing")
	MsgWSClientConnectFailed       = pde("PD021103", "Websocket connect failed")
	MsgWSClientHeartbeatTimeout    = pde("PD021104", "Websocket heartbeat timed out after %.2fms", 500)
)
