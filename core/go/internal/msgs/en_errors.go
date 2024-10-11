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
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"golang.org/x/text/language"
)

const paladinCoreGoPrefix = "PD01"

var registered sync.Once
var ffe = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix(paladinCoreGoPrefix, "Paladin Transaction Manager")
	})
	if !strings.HasPrefix(key, paladinCoreGoPrefix) {
		panic(fmt.Errorf("must have prefix '%s': %s", paladinCoreGoPrefix, key))
	}
	return i18n.FFE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	// Components PD0100XX
	MsgComponentKeyManagerInitError        = ffe("PD010000", "Error initializing key manager")
	MsgComponentKeyManagerStartError       = ffe("PD010001", "Error starting key manager")
	MsgComponentEthClientInitError         = ffe("PD010002", "Error initializing ethereum client")
	MsgComponentEthClientStartError        = ffe("PD010003", "Error starting ethereum client")
	MsgComponentDBInitError                = ffe("PD010004", "Error initializing database")
	MsgComponentDBStartError               = ffe("PD010005", "Error starting database")
	MsgComponentStateManagerInitError      = ffe("PD010006", "Error initializing state store")
	MsgComponentStateManagerStartError     = ffe("PD010007", "Error starting state store")
	MsgComponentBlockIndexerInitError      = ffe("PD010008", "Error initializing block indexer")
	MsgComponentBlockIndexerStartError     = ffe("PD010009", "Error starting block indexer ")
	MsgComponentRPCServerInitError         = ffe("PD010010", "Error initializing RPC server")
	MsgComponentRPCServerStartError        = ffe("PD010011", "Error starting RPC server ")
	MsgComponentDomainInitError            = ffe("PD010012", "Error initializing domains")
	MsgComponentDomainStartError           = ffe("PD010013", "Error starting domain manager")
	MsgComponentPluginInitError            = ffe("PD010014", "Error initializing plugin manager")
	MsgComponentPluginStartError           = ffe("PD010015", "Error starting plugin manager ")
	MsgComponentWaitPluginStartError       = ffe("PD010016", "Error waiting for plugins to start")
	MsgComponentEngineInitError            = ffe("PD010017", "Error initializing engine")
	MsgComponentEngineStartError           = ffe("PD010018", "Error starting engine")
	MsgComponentTransportInitError         = ffe("PD010019", "Error initializing transport manager")
	MsgComponentTransportStartError        = ffe("PD010020", "Error starting transport manager")
	MsgComponentRegistryInitError          = ffe("PD010021", "Error initializing registry manager")
	MsgComponentRegistryStartError         = ffe("PD010022", "Error starting registry manager")
	MsgComponentPublicTxnManagerInitError  = ffe("PD010023", "Error initializing public transaction manager")
	MsgComponentPublicTxManagerStartError  = ffe("PD010024", "Error starting public transaction manager ")
	MsgComponentPrivateTxManagerInitError  = ffe("PD010025", "Error initializing private transaction manager")
	MsgComponentPrivateTxManagerStartError = ffe("PD010026", "Error starting private transaction manager ")
	MsgComponentTxManagerInitError         = ffe("PD010027", "Error initializing transaction manager")
	MsgComponentTxManagerStartError        = ffe("PD010028", "Error starting transaction manager ")
	MsgComponentIdentityResolverInitError  = ffe("PD010029", "Error initializing identity resolver")
	MsgComponentIdentityResolverStartError = ffe("PD010030", "Error starting identity resolver")
	MsgComponentAdditionalMgrInitError     = ffe("PD010031", "Error initializing %s manager")
	MsgComponentAdditionalMgrStartError    = ffe("PD010032", "Error initializing %s manager")

	// States PD0101XX
	MsgStateInvalidLength             = ffe("PD010101", "Invalid hash len expected=%d actual=%d")
	MsgStateInvalidABIParam           = ffe("PD010102", "Invalid ABI parameter")
	MsgStateInvalidSchemaType         = ffe("PD010103", "Invalid state schema type: %s")
	MsgStateManagerQuiescing          = ffe("PD010104", "State store shutting down")
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
	MsgStateLockCreateNotInContext    = ffe("PD010118", "Cannot mark a creating lock for state %s as it was not added in this context")
	MsgStateFlushFailedDomainReset    = ffe("PD010119", "Flush of state for domain %s contract %s has failed. The domain context must be reset")
	MsgStateSpendConflictUnexpected   = ffe("PD010120", "Pending spend for transaction %s found when attempting to spend from transaction %s")
	MsgStateConfirmConflictUnexpected = ffe("PD010121", "Pending confirmation for transaction %s found when attempting to confirm from transaction %s")
	MsgStateDomainContextClosed       = ffe("PD010122", "Domain context has been closed")
	MsgStateDomainContextNotActive    = ffe("PD010123", "There is no domain context with UUID %s active")
	MsgStateLockNoTransaction         = ffe("PD010124", "Transaction missing from state lock")
	MsgStateLockNoState               = ffe("PD010125", "State missing from state lock")
	MsgStateNullifierStateNotInCtx    = ffe("PD010126", "State %s referred to by nullifier %s has not previously been added to the context")
	MsgStateNullifierConflict         = ffe("PD010127", "State %s already has nullifier %s associated in this context")
	MsgStateInvalidCalculatingHash    = ffe("PD010128", "Failed to generate hash as state is invalid")
	MsgStateHashMismatch              = ffe("PD010129", "The supplied state ID '%s' does not match the state hash '%s'")
	MsgStateIDMissing                 = ffe("PD010130", "The state id must be supplied for this domain")

	// Persistence PD0102XX
	MsgPersistenceInvalidType         = ffe("PD010200", "Invalid persistence type: %s")
	MsgPersistenceMissingDSN          = ffe("PD010201", "Missing database connection Data Source Name (DSN)")
	MsgPersistenceInitFailed          = ffe("PD010202", "Database init failed")
	MsgPersistenceMigrationFailed     = ffe("PD010203", "Database migration failed")
	MsgPersistenceMissingMigrationDir = ffe("PD010204", "Missing database migration directory for autoMigrate")
	MsgPersistenceInvalidDSNTemplate  = ffe("PD010205", "dsnParams were provided, but the DSN supplied is not a valid template")
	MsgPersistenceDSNParamLoadFile    = ffe("PD010206", "Failed to load dsnParams[%s] from '%s'")
	MsgPersistenceDSNTemplateFail     = ffe("PD010207", "Templated substitution into database connection DSN failed")

	// Transaction Processor PD0103XX
	MsgTransactionProcessorInvalidStage         = ffe("PD010300", "Invalid stage: %s")
	MsgContextCanceled                          = ffe("PD010301", "Context canceled")
	MsgTransactionProcessorActionFailed         = ffe("PD010302", "Action for transaction with ID: %s at stage %s failed")
	MsgTransactionProcessorBlockedOnDependency  = ffe("PD010303", "Transaction with ID: %s cannot be processed by %s stage due to dependent transactions.")
	MsgTransactionProcessorUndeterminedStage    = ffe("PD010304", "Cannot determine a processing stage for transaction with ID: %s")
	MsgTransactionProcessorEmptyAssembledResult = ffe("PD010305", "No transaction was assembled for transaction with ID: %s")

	// Transaction store PD0104XX
	MsgTransactionMissingField         = ffe("PD010400", "Must provide a payload (one of PayloadJSON or PayloadRLP), from, and contract address.  Mising %v")
	MsgTransactionParseError           = ffe("PD010401", "Failed to parse transaction message.")
	MsgTransactionSerializeError       = ffe("PD010402", "Failed to serialise transaction response.")
	MsgTransactionInvalidTransactionID = ffe("PD010403", "The provided ID %s cannot be parsed into a valid UUID due to %s")

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
	MsgFiltersValueInvalidUUID            = ffe("PD010720", "Failed to parse value as UUID: %v")

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
	MsgBlockIndexerESSourceError            = ffe("PD011302", "Event stream sources must not be changed after creation")
	MsgBlockIndexerESInitFail               = ffe("PD011303", "Event stream initialization failed")
	MsgBlockIndexerESAlreadyInit            = ffe("PD011304", "Event stream already initialized")
	MsgBlockIndexerConfirmedReceiptNotFound = ffe("PD011305", "Receipt for confirmed transaction %s not found")
	MsgBlockIndexerInvalidEventStreamType   = ffe("PD011306", "Unsupported event stream type: %s")
	MsgBlockIndexerNoBlocksIndexed          = ffe("PD011308", "No confirmed blocks have yet been indexed")
	MsgBlockIndexerTransactionReverted      = ffe("PD011309", "Transaction reverted: %s")
	MsgBlockIndexerConfirmedBlockNotFound   = ffe("PD011310", "Block %s (%d) not found on retrieval after detection and requested number of confirmations")

	// EthClient module PD0115XX
	MsgEthClientInvalidInput            = ffe("PD011500", "Unable to convert to ABI function input (func=%s)")
	MsgEthClientMissingFrom             = ffe("PD011501", "Signer (from) missing")
	MsgEthClientMissingTo               = ffe("PD011502", "To missing")
	MsgEthClientMissingInput            = ffe("PD011503", "Input missing")
	MsgEthClientMissingOutput           = ffe("PD011504", "Output missing")
	MsgEthClientInvalidTXVersion        = ffe("PD011505", "Invalid TX Version (%s)")
	MsgEthClientABIJson                 = ffe("PD011506", "JSON ABI parsing failed")
	MsgEthClientFunctionNotFound        = ffe("PD011507", "Function %q not found on ABI")
	MsgEthClientChainIDFailed           = ffe("PD011508", "Failed to query chain ID")
	MsgEthClientKeyMismatch             = ffe("PD011509", "Resolved %q to different key handle expected=%q received=%q")
	MsgEthClientToWithConstructor       = ffe("PD011510", "To address cannot be specified for constructor")
	MsgEthClientHTTPURLMissing          = ffe("PD011511", "HTTP URL missing in configuration")
	MsgEthClientChainIDMismatch         = ffe("PD011512", "ChainID mismatch between HTTP and WebSocket JSON/RPC connections http=%d ws=%d")
	MsgEthClientCallReverted            = ffe("PD011513", "Reverted: %s")
	MsgEthClientReceiptNotAvailable     = ffe("PD011514", "Receipt not available for transaction '%s'")
	MsgEthClientReturnValueNotDecoded   = ffe("PD011515", "Error return value for custom error: %s")
	MsgEthClientReturnValueNotAvailable = ffe("PD011516", "Error return value unavailable")
	MsgEthClientNoConnection            = ffe("PD011517", "No JSON/RPC connection is available to this client")

	// DomainManager module PD0116XX
	MsgDomainNotFound                         = ffe("PD011600", "Domain %q not found")
	MsgDomainNotInitialized                   = ffe("PD011601", "Domain not initialized")
	MsgDomainInvalidSchema                    = ffe("PD011602", "Domain schema %d is invalid")
	MsgDomainFactoryAbiJsonInvalid            = ffe("PD011605", "Factory contract ABI invalid")
	MsgDomainRegistryAddressInvalid           = ffe("PD011606", "Registry address '%s' invalid for domain '%s'")
	MsgDomainPrivateAbiJsonInvalid            = ffe("PD011607", "Private contract ABI invalid")
	MsgDomainInvalidQueryJSON                 = ffe("PD011608", "Invalid query JSON")
	MsgDomainContractNotFoundByAddr           = ffe("PD011609", "A smart contract with address %s has not yet been indexed")
	MsgDomainInvalidPrepareDeployResult       = ffe("PD011611", "Prepare deploy did not result in exactly one of a invoke transaction or a deploy transaction")
	MsgDomainInvalidFunctionParams            = ffe("PD011612", "Invalid function parameters for %s")
	MsgDomainUnknownSchema                    = ffe("PD011613", "Unknown schema %s")
	MsgDomainInvalidStateIDFromDomain         = ffe("PD011614", "Domain returned invalid id '%s' for state %d")
	MsgDomainInputStateNotFound               = ffe("PD011615", "Input state %d [%s] not found")
	MsgDomainMissingStates                    = ffe("PD011616", "Missing in-memory states")
	MsgDomainEndorsementReverted              = ffe("PD011617", "Endorsement from '%s' reverted: %s")
	MsgDomainBaseLedgerSubmitInvalid          = ffe("PD011619", "Base ledger submission config is invalid")
	MsgDomainTXIncompleteInitDeploy           = ffe("PD011620", "Transaction is incomplete for phase InitDeploy")
	MsgDomainTXIncompletePrepareDeploy        = ffe("PD011621", "Transaction is incomplete for phase PrepareDeploy")
	MsgDomainDeployNoSigner                   = ffe("PD011622", "Domain did not provide a signer for base ledger transaction to deploy the private smart contract")
	MsgDomainMultipleEndorsersSubmit          = ffe("PD011623", "Multiple endorsers of the transaction specified a submission constraint")
	MsgDomainNoEndorserSubmit                 = ffe("PD011624", "Domain is configured for endorser submission, and no endorser specified a submission constraint")
	MsgDomainInvalidSubmissionConfig          = ffe("PD011625", "Domain specified an unexpected base ledger submission config: %s")
	MsgDomainTXIncompleteInitTransaction      = ffe("PD011626", "Transaction is incomplete for phase InitTransaction")
	MsgDomainTXIncompleteAssembleTransaction  = ffe("PD011627", "Transaction is incomplete for phase AssembleTransaction")
	MsgDomainTXIncompleteWritePotentialStates = ffe("PD011628", "Transaction is incomplete for phase WritePotentialStates")
	MsgDomainTXIncompleteLockStates           = ffe("PD011629", "Transaction is incomplete for phase LockStates")
	MsgDomainReqIncompleteEndorseTransaction  = ffe("PD011630", "Request is incomplete for phase EndorseTransaction")
	MsgDomainTXIncompleteResolveDispatch      = ffe("PD011631", "Transaction is incomplete for phase ResolveDispatch")
	MsgDomainTXIncompletePrepareTransaction   = ffe("PD011632", "Transaction is incomplete for phase PrepareTransaction")
	MsgDomainABIEncodingRequestEntryInvalid   = ffe("PD011633", "ABI encoding request could not be completed as ABI entry is invalid")
	MsgDomainABIEncodingRequestEncodingFail   = ffe("PD011634", "ABI encoding request failed")
	MsgDomainABIEncodingRequestInvalidType    = ffe("PD011635", "ABI encoding request is of invalid type '%s'")
	MsgDomainABIEncodingRequestInvalidTX      = ffe("PD011636", "Transaction encoding request is invalid")
	MsgDomainABIRecoverRequestAlgorithm       = ffe("PD011637", "Algorithm not supported for recover signer '%s'")
	MsgDomainABIRecoverRequestSignature       = ffe("PD011638", "Invalid signature")
	MsgDomainABIEncodingTypedDataInvalid      = ffe("PD011639", "EIP-712 typed data V4 encoding request invalid")
	MsgDomainABIEncodingTypedDataFail         = ffe("PD011640", "EIP-712 typed data V4 encoding request failed")
	MsgDomainInvalidSchemaID                  = ffe("PD011641", "Invalid schema ID '%s'")
	MsgDomainInvalidEvents                    = ffe("PD011642", "Events ABI is invalid")
	MsgDomainSigningDisabled                  = ffe("PD011643", "Domain signing it not enabled for domain '%s'")
	MsgDomainSigningAlgorithmNotSupported     = ffe("PD011644", "Domain '%s' has not registered algorithm '%s'")
	MsgDomainABIDecodingRequestEntryInvalid   = ffe("PD011645", "ABI decoding request could not be completed as ABI entry is invalid")
	MsgDomainABIDecodingRequestFail           = ffe("PD011646", "ABI decoding request failed")
	MsgDomainABIDecodingRequestInvalidType    = ffe("PD011647", "ABI decoding request is of invalid type '%s'")
	MsgDomainTransactionWasNotADeployment     = ffe("PD011648", "Transaction %s did not result in a deployment")
	MsgDomainRequestNotInFlight               = ffe("PD011649", "State query context '%s' not in-flight")
	MsgDomainInvalidStateID                   = ffe("PD011650", "Invalid state ID '%s'")
	MsgDomainInvalidStates                    = ffe("PD011651", "Invalid states")
	MsgDomainInvalidResponseToValidate        = ffe("PD011652", "Invalid response to validation")

	// Entrypoint PD0117XX
	MsgEntrypointUnknownRunMode = ffe("PD011700", "Unknown run mode '%s'")

	// PrivTxMgr PD0118XX
	MsgDomainNotProvided              = ffe("PD011800", "Domain not found in the transaction input")
	MsgPrivateTxManagerInternalError  = ffe("PD011801", "Unexpected error in engine %s")
	MsgPrivateTxManagerAssembleError  = ffe("PD011802", "Error assembling transaction")
	MsgPrivateTxManagerParseFailed    = ffe("PD011803", "Failed to parse message")
	MsgPrivateTxManagerInvalidMessage = ffe("PD011804", "Invalid message received from transport")
	MsgSequencerInternalError         = ffe("PD011805", "Sequencer internal error %s, ")
	MsgKeyResolutionFailed            = ffe("PD011806", "Key resolution failed for key %s, algorithm %s")
	MsgDeployInitFailed               = ffe("PD011807", "Failed to initialise a deploy transaction")
	MsgDeployPrepareFailed            = ffe("PD011808", "Failed to prepare a deploy transaction")
	MsgDeployPrepareIncomplete        = ffe("PD011809", "Prepare step did not return a transaction to invoke, or a transaction to deploy")
	MsgBaseLedgerTransactionFailed    = ffe("PD011810", "Failed to submit base ledger transaction")
	MsgContractAddressNotProvided     = ffe("PD011811", "Contract address (To) not found in the transaction input")
	MsgPrivTxMgrPublicTxFail          = ffe("PD011812", "Public transaction rejected")
	MsgResolveVerifierRemoteFailed    = ffe("PD011813", "Failed to resolve verifier on remote node with lookup %s algorithm %s: Error %s")

	// Public Transaction Manager PD0119XX
	MsgInsufficientBalance             = ffe("PD011900", "Balance %s of fueling source address %s is below the required amount %s")
	MsgBalanceBelowMinimum             = ffe("PD011901", "Balance %s of fueling source address %s is below the configured minimum balance %s")
	MsgInvalidBigIntString             = ffe("PD011902", "Value of '%s' is not a valid bigInt string")
	MsgMaxBelowMin                     = ffe("PD011903", "Value of '%s' is not a valid max, it is below the min value")
	MsgMaxBelowMinThreshold            = ffe("PD011904", "Value of '%s' is not a valid max, it is below the min threshold")
	MsgSubmitFailedWrongHashReturned   = ffe("PD011905", "Submission of transaction with calculatedHash '%s' returned hash '%s'")
	MsgSubmissionResponseMissingTxHash = ffe("PD011906", "Missing transaction hash from the submission response for transaction with ID: %s")
	MsgPublicTxMgrAlreadyInit          = ffe("PD011907", "Public transaction manager already initialized")
	MsgInvalidGasClientConfig          = ffe("PD011908", "Invalid gas client config: %s")
	MsgInvalidGasPriceIncreaseMax      = ffe("PD011909", "Invalid max gas price increase price string %s")
	MsgMissingTransactionID            = ffe("PD011910", "Transaction ID must be provided")
	MsgPublicTransactionNotFound       = ffe("PD011911", "Public transaction not found with id %s")
	MsgGasPriceError                   = ffe("PD011917", `The gasPrice '%s' could not be parsed. Must be a numeric string, or an object with 'gasPrice' field, or 'maxFeePerGas'/'maxPriorityFeePerGas' fields (EIP-1559), error: %s`)
	MsgPersistError                    = ffe("PD011918", "Unexpected internal error, cannot persist stage.")
	MsgInvalidStageOutput              = ffe("PD011919", "Stage output object is missing %s: %+v")
	MsgInvalidGasLimit                 = ffe("PD011920", "Invalid gas limit, must be a positive number")
	MsgStatusUpdateForbidden           = ffe("PD011921", "Cannot update status of a completed transaction")
	MsgTransactionNotFound             = ffe("PD011924", "Transaction '%s' not found")
	MsgTransactionEngineRequestTimeout = ffe("PD011926", "The transaction handler did not acknowledge the request after %.2fs")
	MsgErrorMissingSignerID            = ffe("PD011928", "Signer Identifier must be provided")
	MsgInvalidTransactionType          = ffe("PD011929", "Transaction type invalid")
	MsgMissingConfirmedTransaction     = ffe("PD011930", "Transaction %s with nonce smaller than the recorded confirmed nonce does not have an indexed transaction.")
	MsgPublicTxHistoryInfo             = ffe("PD011931", "PubTx[INFO] from=%s nonce=%s subStatus=%s action=%s info=%s")
	MsgPublicTxHistoryError            = ffe("PD011932", "PubTx[ERROR] from=%s nonce=%s subStatus=%s action=%s error=%s")
	MsgPublicBatchCompleted            = ffe("PD011933", "Batch already completed")
	MsgInvalidAutoFuelSource           = ffe("PD011934", "Invalid auto-fueling source '%s'")
	MsgInvalidStateMissingTXHash       = ffe("PD011935", "Invalid state - missing transaction hash from previous sign stage")

	// TransportManager module PD0120XX
	MsgTransportInvalidMessage                = ffe("PD012000", "Invalid message")
	MsgTransportNotFound                      = ffe("PD012001", "Transport %q not found")
	MsgTransportNodeNameNotConfigured         = ffe("PD012002", "nodeName must be configured to set the identity of the local node")
	MsgTransportNoTransportsConfiguredForNode = ffe("PD012003", "None of the transports registered by node '%s' are configured locally on this node: %v")
	MsgTransportDetailsNotAvailable           = ffe("PD012004", "Transport '%s' not available for node '%s'")
	MsgTransportInvalidDestinationReceived    = ffe("PD012005", "Message received with invalid destination for local node '%s': '%s'")
	MsgTransportInvalidReplyToReceived        = ffe("PD012006", "Message received with invalid replyTo destination: '%s'")
	MsgTransportInvalidDestinationSend        = ffe("PD012007", "Message has invalid destination for sending from local node '%s': '%s'")
	MsgTransportInvalidReplyToSend            = ffe("PD012008", "Message has invalid replyTo destination: '%s'")
	MsgTransportInvalidLocalNode              = ffe("PD012009", "Node '%s' is the local node")
	MsgTransportClientAlreadyRegistered       = ffe("PD012010", "Client '%s' already registered")
	MsgTransportDestinationNotFound           = ffe("PD012011", "Destination '%s' not found")
	MsgTransportClientRegisterAfterStartup    = ffe("PD012012", "Client '%s' attempted registration after startup")

	// RegistryManager module PD0121XX
	MsgRegistryNodeEntiresNotFound     = ffe("PD012100", "No entries found for node '%s'")
	MsgRegistryNotFound                = ffe("PD012101", "Registry %q not found")
	MsgRegistryInvalidEventSource      = ffe("PD012102", "Events source %d is invalid")
	MsgRegistryInvalidEntryID          = ffe("PD012103", "Invalid entry ID '%s'")
	MsgRegistryInvalidEntryName        = ffe("PD012104", "Invalid entry name '%s'")
	MsgRegistryInvalidPropertyName     = ffe("PD012105", "Invalid property name '%s'")
	MsgRegistryInvalidParentID         = ffe("PD012106", "Invalid parent ID '%s'")
	MsgRegistryQueryLimitRequired      = ffe("PD012107", "Limit is required on all queries")
	MsgRegistryTransportPropertyRegexp = ffe("PD012108", "transports.propertyRegexp for registry '%s' is invalid")
	MsgRegistryDollarPrefixReserved    = ffe("PD012109", "Name '%s' is invalid. Dollar ('$') prefix is allowed only for reserved properties, and then is required (pluginReserved=%t)")

	// TxMgr module PD0122XX
	MsgTxMgrQueryLimitRequired           = ffe("PD012200", "limit is required on all queries")
	MsgTxMgrInvalidABI                   = ffe("PD012201", "ABI is invalid")
	MsgTxMgrABIAndDefinition             = ffe("PD012202", "Must supply one of an abi or an abiReference")
	MsgTxMgrABIReferenceLookupFailed     = ffe("PD012203", "Failed to resolve abiReference %s")
	MsgTxMgrFunctionWithoutTo            = ffe("PD012204", "A to contract address must be specified with a function name (leave blank to select constructor)")
	MsgTxMgrFunctionMultiMatch           = ffe("PD012205", "Supplied function selector matched more than one function in the ABI: '%s' and '%s'")
	MsgTxMgrFunctionNoMatch              = ffe("PD012206", "Supplied function selector did not match any function in the ABI")
	MsgTxMgrBytecodeNonPublicConstructor = ffe("PD012207", "Bytecode can only be supplied with a public constructor. Selected %s function %s")
	MsgTxMgrInvalidInputData             = ffe("PD012208", "Invalid input data for function %s")
	MsgTxMgrBytecodeAndHexData           = ffe("PD012210", "When deploying a smart contract the bytecode must be supplied separately to the input data")
	MsgTxMgrInvalidTXType                = ffe("PD012211", "Invalid transaction type")
	MsgTxMgrInvalidInputDataType         = ffe("PD012212", "Invalid input data type: %T")
	MsgTxMgrInvalidReceiptNotification   = ffe("PD012213", "Invalid receipt notification from component: %s")
	MsgTxMgrRevertedNoData               = ffe("PD012214", "Transaction reverted (no revert data)")
	MsgTxMgrRevertedDataNotDecoded       = ffe("PD012215", "Transaction reverted (revert data not decoded)")
	MsgTxMgrRevertedDecodedData          = ffe("PD012216", "Transaction reverted %s")
	MsgTxMgrInvalidStoredData            = ffe("PD012217", "Stored data is invalid")
	MsgTxMgrNoABIOrReference             = ffe("PD012218", "An ABI containing a function/constructor definition or an abiReference to an existing stored ABI must be supplied")

	// FlushWriter module PD0123XX
	MsgFlushWriterQuiescing      = ffe("PD012300", "Writer shutting down")
	MsgFlushWriterInvalidResults = ffe("PD012301", "Error in handler produced invalid write results")
	MsgFlushWriterOpInvalid      = ffe("PD012302", "Write operation missing key")
)
