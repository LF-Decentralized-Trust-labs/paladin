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

package tkmsgs

import (
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"golang.org/x/text/language"
)

var registered sync.Once
var ffe = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix("PD02", "Paladin Toolkit")
	})
	return i18n.FFE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	// Types PD0200XX
	MsgContextCanceled                    = ffe("PD020000", "Context canceled")
	MsgTypesUnmarshalNil                  = ffe("PD020001", "UnmarshalJSON on nil pointer")
	MsgTypesScanFail                      = ffe("PD020002", "Unable to scan type %T into type %T")
	MsgTypesEnumValueInvalid              = ffe("PD020003", "Value must be one of %s")
	MsgTypesABIDefNotInBothStructs        = ffe("PD020004", "ABI is not equal due to mismatch on: %s")
	MsgTypesInvalidNameSafeCharAlphaBoxed = ffe("PD020005", "Field '%s' must be 1-%d characters, including alphanumerics (a-zA-Z0-9), dot (.), dash (-) and underscore (_), and must start/end in an alphanumeric: %q")
	MsgTypesPrivateIdentityLocatorInvalid = ffe("PD020006", "Locator string %s is invalid")
	MsgTypesInvalidHex                    = ffe("PD020007", "Invalid hex: %s")
	MsgTypesValueInvalidHexBytes32        = ffe("PD020008", "Failed to parse value as 32 byte hex string (parsedBytes=%d)")
	MsgTypesInvalidHexInteger             = ffe("PD020009", "Invalid integer: %s")
	MsgTypesInvalidUint64                 = ffe("PD020010", "Integer cannot be converted to uint64 without losing precision: %s")
	MsgTypesInvalidDBInt64                = ffe("PD020011", "Integer too large for storage in a signed int64 database column: %s")
	MsgTypesInvalidDBInt256               = ffe("PD020012", "Integer incorrectly serialized to the database for a int256: %s")
	MsgTypesInvalidDBUint256              = ffe("PD020013", "Integer incorrectly serialized to the database for a uint256: %s")

	// Inflight PD0201XX
	MsgInflightRequestCancelled = ffe("PD020100", "Request cancelled after %s")

	// PldClient module PD0202XX
	MsgPaladinClientInvalidInput      = ffe("PD020200", "Unable to convert to ABI function input (func=%s)")
	MsgPaladinClientMissingFrom       = ffe("PD020201", "From (signing key identifier) missing")
	MsgPaladinClientMissingTo         = ffe("PD020202", "To missing")
	MsgPaladinClientMissingInput      = ffe("PD020203", "Input missing")
	MsgPaladinClientMissingOutput     = ffe("PD020204", "Output missing")
	MsgPaladinClientBytecodeWithPriv  = ffe("PD020205", "Bytecode cannot be specified with a private contract deployment")
	MsgPaladinClientBytecodeMissing   = ffe("PD020206", "Bytecode required to deploy a public smart contract")
	MsgPaladinClientABIJson           = ffe("PD020207", "JSON ABI parsing failed")
	MsgPaladinClientFunctionNotFound  = ffe("PD020208", "Function %q not found on ABI")
	MsgPaladinClientToWithConstructor = ffe("PD020209", "To address cannot be specified for constructor")
	MsgPaladinClientNoConnection      = ffe("PD020210", "No JSON/RPC connection is available to this client")
	MsgPaladinClientMissingType       = ffe("PD020211", "Type missing (public or private)")
	MsgPaladinClientNoFailureMsg      = ffe("PD020212", "No failure message available")

	// Plugin PD0203XX
	MsgPluginUnsupportedRequest   = ffe("PD020300", "Unsupported request %T")
	MsgPluginUnexpectedResponse   = ffe("PD020301", "Unexpected response %T (expected %T)")
	MsgPluginUnimplementedRequest = ffe("PD020302", "Unimplemented plugin request %T")
	MsgPluginErrorFromServerNoMsg = ffe("PD020303", "Error from server (no detailed message in response)")

	// TLS PD0204XX
	MsgTLSInvalidCAFile             = ffe("PD020400", "Invalid CA certificates file")
	MsgTLSConfigFailed              = ffe("PD020401", "Failed to initialize TLS configuration")
	MsgTLSInvalidKeyPairFiles       = ffe("PD020402", "Invalid certificate and key pair files")
	MsgTLSInvalidTLSDnMatcherAttr   = ffe("PD020403", "Unknown DN attribute '%s'")
	MsgTLSInvalidTLSDnMatcherRegexp = ffe("PD020404", "Invalid regexp '%s' for requiredDNAttributes[%s]: %s")
	MsgTLSInvalidTLSDnChain         = ffe("PD020405", "Cannot match subject distinguished name as cert chain is not verified")
	MsgTLSInvalidTLSDnMismatch      = ffe("PD020406", "Certificate subject does not meet requirements")

	// RPCClient PD0205XX
	MsgRPCClientInvalidWebSocketURL = ffe("PD020500", "Invalid WebSocket URL: %s")
	MsgRPCClientInvalidHTTPURL      = ffe("PD020501", "Invalid HTTP URL: %s")

	// HTTPServer PD0108XX
	MsgHTTPServerStartFailed        = ffe("PD020600", "Failed to start server on '%s'")
	MsgHTTPServerMissingPort        = ffe("PD020601", "HTTP server port must be specified for '%s'")
	MsgHTTPServerNoWSUpgradeSupport = ffe("PD020602", "HTTP server does not support WebSocket upgrade (%T)")

	// JSON/RPC PD0207XX
	MsgJSONRPCInvalidRequest      = ffe("PD020700", "Invalid JSON/RPC request data")
	MsgJSONRPCMissingRequestID    = ffe("PD020701", "Invalid JSON/RPC request. Must set request ID")
	MsgJSONRPCUnsupportedMethod   = ffe("PD020702", "method not supported")
	MsgJSONRPCIncorrectParamCount = ffe("PD020703", "method %s requires %d params (supplied=%d)")
	MsgJSONRPCInvalidParam        = ffe("PD020704", "method %s parameter %d invalid: %s")
	MsgJSONRPCResultSerialization = ffe("PD020705", "method %s result serialization failed: %s")

	// Signing module PD020800
	MsgSigningModuleBadPathError                = ffe("PD020800", "Path '%s' does not exist, or it is not a directory")
	MsgSigningModuleBadKeyFile                  = ffe("PD020801", "Key file '%s' does not exist")
	MsgSigningModuleBadPassFile                 = ffe("PD020802", "Password file '%s' does not exist")
	MsgSigningModuleBadKeyHandle                = ffe("PD020803", "Invalid key handle")
	MsgSigningModuleFSError                     = ffe("PD020804", "Filesystem error")
	MsgSigningModuleKeyHandleClash              = ffe("PD020805", "Invalid key handle (clash)")
	MsgSigningModuleKeyNotExist                 = ffe("PD020806", "Key '%s' does not exist")
	MsgSigningUnsupportedKeyStoreType           = ffe("PD020807", "Unsupported key store type: '%s'")
	MsgSigningHierarchicalRequiresLoading       = ffe("PD020808", "Signing module has been configured to disallow in-memory key material. Hierarchical Deterministic (HD) wallet function implemented in the signing module requires in-memory key material")
	MsgSigningKeyStoreNoInStoreSingingSupport   = ffe("PD020809", "They configured key store '%s' does not support signing within the keystore itself (keys must be loaded into memory in the module to sign)")
	MsgSigningUnsupportedAlgoForInMemorySigning = ffe("PD020810", "Unsupported algorithm for in-memory signing: %s")
	MsgSigningMustSpecifyAlgorithms             = ffe("PD020811", "Must specify at least one algorithm for key resolution")
	MsgSigningHDSeedMustBe32BytesOrMnemonic     = ffe("PD020812", "Seed key material for HD Wallet must be either a 32byte value, or a BIP-39 compliant mnemonic seed phrase")
	MsgSignerBIP44DerivationInvalid             = ffe("PD020813", "Invalid key handle - BIP44 key identifier expected (invalid derivation: '%s')")
	MsgSignerBIP32DerivationTooLarge            = ffe("PD020814", "BIP-32 key index must be between 0 and 2^31-1 at each level in the hierarchy")
	MsgSigningKeyListingNotSupported            = ffe("PD020815", "Listing keys in the key store is not supported by this signing module")
	MsgSigningStaticKeyInvalid                  = ffe("PD020816", "Statically configured key with handle %s is invalid")
	MsgSigningStaticBadEncoding                 = ffe("PD020817", "Statically configured key with handle %s has invalid encoding (must be one of 'none', 'hex', 'base64') '%s'")
	MsgSigningKeyCannotBeResolved               = ffe("PD020818", "No key exists that matches the request")
	MsgSigningUnsupportedKeyDerivationType      = ffe("PD020819", "Unsupported key derivation type: '%s'")
	MsgSigningKeyCannotBeEmpty                  = ffe("PD020820", "Cannot resolve a signing key for the empty string")
	MsgSigningFailedToLoadStaticKeyFile         = ffe("PD020821", "Failed to load static key file")
	MsgSigningUnsupportedECDSACurve             = ffe("PD020822", "Unsupported ECDSA curve: '%s'")
	MsgSigningUnsupportedVerifierCombination    = ffe("PD020823", "Unsupported verifier type '%s' for algorithm '%s'")
	MsgSigningUnsupportedPayloadCombination     = ffe("PD020824", "Unsupported payload type '%s' for algorithm '%s'")
	MsgSigningEmptyPayload                      = ffe("PD020825", "No payload supplied for signing")
	MsgSigningInvalidDomainAlgorithmNoPrefix    = ffe("PD020826", "Invalid domain algorithm (no 'domain:' prefix): %s")
	MsgSigningNoDomainRegisteredWithModule      = ffe("PD020827", "Domain '%s' has not been registered in this signing module")
)
