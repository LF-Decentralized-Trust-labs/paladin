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
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"golang.org/x/text/language"
)

//revive:disable
var ffm = func(key, translation string) i18n.MessageKey {
	return i18n.FFM(language.AmericanEnglish, key, translation)
}

// pldapi/blockindex.go
var (
	IndexedBlockNumber                 = ffm("IndexedBlock.number", "The block number")
	IndexedBlockHash                   = ffm("IndexedBlock.hash", "The unique hash of the block")
	IndexedBlockTimestamp              = ffm("IndexedBlock.timestamp", "The block timestamp")
	IndexedTransactionHash             = ffm("IndexedTransaction.hash", "The unique hash of the transaction")
	IndexedTransactionBlockNumber      = ffm("IndexedTransaction.blockNumber", "The block number containing this transaction")
	IndexedTransactionTransactionIndex = ffm("IndexedTransaction.transactionIndex", "The index of the transaction within the block")
	IndexedTransactionFrom             = ffm("IndexedTransaction.from", "The sender's Ethereum address")
	IndexedTransactionTo               = ffm("IndexedTransaction.to", "The recipient's Ethereum address (optional)")
	IndexedTransactionNonce            = ffm("IndexedTransaction.nonce", "The transaction nonce")
	IndexedTransactionContractAddress  = ffm("IndexedTransaction.contractAddress", "The contract address created by this transaction (optional)")
	IndexedTransactionResult           = ffm("IndexedTransaction.result", "The result of the transaction (optional)")
	IndexedTransactionBlock            = ffm("IndexedTransaction.block", "The block containing this event")
	IndexedEventBlockNumber            = ffm("IndexedEvent.blockNumber", "The block number containing this event")
	IndexedEventTransactionIndex       = ffm("IndexedEvent.transactionIndex", "The index of the transaction within the block")
	IndexedEventLogIndex               = ffm("IndexedEvent.logIndex", "The log index of the event")
	IndexedEventTransactionHash        = ffm("IndexedEvent.transactionHash", "The hash of the transaction that triggered this event")
	IndexedEventSignature              = ffm("IndexedEvent.signature", "The event signature")
	IndexedEventTransaction            = ffm("IndexedEvent.transaction", "The transaction that triggered this event (optional)")
	IndexedEventBlock                  = ffm("IndexedEvent.block", "The block containing this event")
	EventWithDataSoliditySignature     = ffm("EventWithData.soliditySignature", "A Solidity style description of the event and parameters, including parameter names and whether they are indexed")
	EventWithDataAddress               = ffm("EventWithData.address", "The address of the smart contract that emitted this event")
	EventWithDataData                  = ffm("EventWithData.data", "JSON formatted data from the event")
)

// pldapi/keymgr.go
var (
	WalletInfoName                     = ffm("WalletInfo.name", "The name of the wallet")
	WalletInfoKeySelector              = ffm("WalletInfo.keySelector", "The key selector for the wallet")
	KeyMappingIdentifier               = ffm("KeyMapping.identifier", "The full identifier used to look up this key")
	KeyMappingWallet                   = ffm("KeyMapping.wallet", "The name of the wallet containing this key")
	KeyMappingKeyHandle                = ffm("KeyMapping.keyHandle", "The handle within the wallet containing the key")
	KeyMappingWithPathPath             = ffm("KeyMappingWithPath.path", "The full path including the leaf that is the identifier")
	KeyMappingAndVerifierVerifier      = ffm("KeyMappingAndVerifier.verifier", "The verifier associated with this key mapping")
	KeyVerifierWithKeyRefKeyIdentifier = ffm("KeyVerifierWithKeyRef.keyIdentifier", "The identifier of the key associated with this verifier")
	KeyVerifierVerifier                = ffm("KeyVerifier.verifier", "The verifier value")
	KeyVerifierType                    = ffm("KeyVerifier.type", "The type of verifier")
	KeyVerifierAlgorithm               = ffm("KeyVerifier.algorithm", "The algorithm used by the verifier")
	KeyPathSegmentName                 = ffm("KeyPathSegment.name", "The name of the path segment")
	KeyPathSegmentIndex                = ffm("KeyPathSegment.index", "The index of the path segment")
)

// pldapi/public_tx.go
var (
	PublicTxOptionsGas                     = ffm("PublicTxOptions.gas", "The gas limit for the transaction (optional)")
	PublicTxOptionsValue                   = ffm("PublicTxOptions.value", "The value transferred in the transaction (optional)")
	PublicCallOptionsBlock                 = ffm("PublicCallOptions.block", "The block number or 'latest' when calling a public smart contract (optional)")
	PublicTxGasPricingMaxPriorityFeePerGas = ffm("PublicTxGasPricing.maxPriorityFeePerGas", "The maximum priority fee per gas (optional)")
	PublicTxGasPricingMaxFeePerGas         = ffm("PublicTxGasPricing.maxFeePerGas", "The maximum fee per gas (optional)")
	PublicTxGasPricingGasPrice             = ffm("PublicTxGasPricing.gasPrice", "The gas price (optional)")
	PublicTxInputFrom                      = ffm("PublicTxInput.from", "The resolved signing account")
	PublicTxInputTo                        = ffm("PublicTxInput.to", "The target contract address (optional)")
	PublicTxInputData                      = ffm("PublicTxInput.data", "The pre-encoded calldata (optional)")
	PublicTxSubmissionFrom                 = ffm("PublicTxSubmission.from", "The sender's Ethereum address")
	PublicTxSubmissionNonce                = ffm("PublicTxSubmission.nonce", "The transaction nonce")
	PublicTxSubmissionDataTime             = ffm("PublicTxSubmissionData.time", "The submission time")
	PublicTxSubmissionDataTransactionHash  = ffm("PublicTxSubmissionData.transactionHash", "The transaction hash")
	PublicTxLocalID                        = ffm("PublicTx.localId", "A locally generated numeric ID for the public transaction. Unique within the node")
	PublicTxTo                             = ffm("PublicTx.to", "The target contract address (optional)")
	PublicTxData                           = ffm("PublicTx.data", "The pre-encoded calldata (optional)")
	PublicTxFrom                           = ffm("PublicTx.from", "The sender's Ethereum address")
	PublicTxNonce                          = ffm("PublicTx.nonce", "The transaction nonce")
	PublicTxCreated                        = ffm("PublicTx.created", "The creation time")
	PublicTxCompletedAt                    = ffm("PublicTx.completedAt", "The completion time (optional)")
	PublicTxTransactionHash                = ffm("PublicTx.transactionHash", "The transaction hash (optional)")
	PublicTxSuccess                        = ffm("PublicTx.success", "The transaction success status (optional)")
	PublicTxRevertData                     = ffm("PublicTx.revertData", "The revert data (optional)")
	PublicTxSubmissions                    = ffm("PublicTx.submissions", "The submission data (optional)")
	PublicTxActivity                       = ffm("PublicTx.activity", "The transaction activity records (optional)")
	PublicTxBindingTransaction             = ffm("PublicTxBinding.transaction", "The transaction ID")
	PublicTxBindingTransactionType         = ffm("PublicTxBinding.transactionType", "The transaction type")
)

// pldapi/stored_abi.go
var (
	StoredABIHash = ffm("StoredABI.hash", "The unique hash of the ABI")
	StoredABIAPI  = ffm("StoredABI.abi", "The Application Binary Interface (ABI) definition")
)

// pldclient/transaction.go
var (
	TransactionID                                 = ffm("Transaction.id", "Server-generated UUID for this transaction (query only)")
	TransactionCreated                            = ffm("Transaction.created", "Server-generated creation timestamp for this transaction (query only)")
	TransactionSubmitMode                         = ffm("Transaction.submitMode", "Whether the submission of the transaction to the base ledger is to be performed automatically by the node or coordinated externally (query only)")
	TransactionIdempotencyKey                     = ffm("Transaction.idempotencyKey", "Externally supplied unique identifier for this transaction. 409 Conflict will be returned on attempt to re-submit")
	TransactionType                               = ffm("Transaction.type", "Type of transaction (public or private)")
	TransactionDomain                             = ffm("Transaction.domain", "Name of a domain - only required on input for private deploy transactions")
	TransactionFunction                           = ffm("Transaction.function", "Function signature - inferred from definition if not supplied")
	TransactionABIReference                       = ffm("Transaction.abiReference", "Calculated ABI reference - required with ABI on input if not constructor")
	TransactionFrom                               = ffm("Transaction.from", "Locator for a local signing identity to use for submission of this transaction")
	TransactionTo                                 = ffm("Transaction.to", "Target contract address, or null for a deploy")
	TransactionData                               = ffm("Transaction.data", "Pre-encoded array with/without function selector, array, or object input")
	TransactionInputDependsOn                     = ffm("TransactionInput.dependsOn", "Transactions that must be mined on the blockchain successfully before this transaction submits")
	TransactionInputABI                           = ffm("TransactionInput.abi", "Application Binary Interface (ABI) definition - required if abiReference not supplied")
	TransactionInputBytecode                      = ffm("TransactionInput.bytecode", "Bytecode prepended to encoded data inputs for deploy transactions")
	TransactionCallDataFormat                     = ffm("TransactionCall.dataFormat", "How call data should be serialized into JSON once decoded using the ABI function definition")
	TransactionFullDependsOn                      = ffm("TransactionFull.dependsOn", "Transactions registered as dependencies when the transaction was created")
	TransactionFullReceipt                        = ffm("TransactionFull.receipt", "Transaction receipt data - available if the transaction has reached a final state")
	TransactionFullPublic                         = ffm("TransactionFull.public", "List of public transactions associated with this transaction")
	TransactionReceiptID                          = ffm("TransactionReceipt.id", "Transaction ID")
	TransactionReceiptDataOnchainTransactionHash  = ffm("TransactionReceiptDataOnchain.transactionHash", "Transaction hash")
	TransactionReceiptDataOnchainBlockNumber      = ffm("TransactionReceiptDataOnchain.blockNumber", "Block number")
	TransactionReceiptDataOnchainTransactionIndex = ffm("TransactionReceiptDataOnchain.transactionIndex", "Transaction index")
	TransactionReceiptDataOnchainEventLogIndex    = ffm("TransactionReceiptDataOnchainEvent.logIndex", "Log index")
	TransactionReceiptDataOnchainEventSource      = ffm("TransactionReceiptDataOnchainEvent.source", "Event source")
	TransactionReceiptDataIndexed                 = ffm("TransactionReceiptData.indexed", "The time when this receipt was indexed by the node, providing a relative order of transaction receipts within this node (might be significantly after the timestamp of the block)")
	TransactionReceiptDataDomain                  = ffm("TransactionReceiptData.domain", "The domain that executed the transaction, for private transactions only")
	TransactionReceiptDataSuccess                 = ffm("TransactionReceiptData.success", "Transaction success status")
	TransactionReceiptDataFailureMessage          = ffm("TransactionReceiptData.failureMessage", "Failure message - set if transaction reverted")
	TransactionReceiptDataRevertData              = ffm("TransactionReceiptData.revertData", "Encoded revert data - if available")
	TransactionReceiptDataContractAddress         = ffm("TransactionReceiptData.contractAddress", "New contract address - to be used in the 'To' field for subsequent invoke transactions")
	TransactionReceiptFullStates                  = ffm("TransactionReceiptFull.states", "The state receipt for the transaction (private transactions only)")
	TransactionReceiptFullDomainReceipt           = ffm("TransactionReceiptFull.domainReceipt", "The domain receipt for the transaction (private transaction only)")
	TransactionReceiptFullDomainReceiptError      = ffm("TransactionReceiptFull.domainReceiptError", "Contains the error if it was not possible to obtain the domain receipt for a private transaction")
	TransactionActivityRecordTime                 = ffm("TransactionActivityRecord.time", "Time the record occurred")
	TransactionActivityRecordMessage              = ffm("TransactionActivityRecord.message", "Activity message")
	TransactionDependenciesDependsOn              = ffm("TransactionDependencies.dependsOn", "Transactions that this transaction depends on")
	TransactionDependenciesPrereqOf               = ffm("TransactionDependencies.prereqOf", "Transactions that require this transaction as a prerequisite")
	PreparedTransactionID                         = ffm("PreparedTransaction.id", "The ID of the original transaction that prepared this transaction, and will be confirmed by its submission to the blockchain")
	PreparedTransactionDomain                     = ffm("PreparedTransaction.domain", "The domain of the original transaction that prepared this transaction submission")
	PreparedTransactionTo                         = ffm("PreparedTransaction.to", "The to address or the original transaction that prepared this transaction submission")
	PreparedTransactionTransaction                = ffm("PreparedTransaction.transaction", "The Paladin transaction definition that has been prepared for submission, with the ABI and function details resolved")
	PreparedTransactionMetadata                   = ffm("PreparedTransaction.metadata", "Domain specific additional information generated during prepare in addition to the states. Used particularly in atomic multi-party transactions to separate data that can be disclosed, away from the full transaction submission payload")
	PreparedTransactionStates                     = ffm("PreparedTransaction.states", "Details of all states of the original transaction that prepared this transaction submission")
	DecodedErrorData                              = ffm("ABIDecodedData.data", "The decoded JSON data using the matched ABI definition")
	DecodedSummary                                = ffm("ABIDecodedData.summary", "A string formatted summary - errors only")
	DecodedDefinition                             = ffm("ABIDecodedData.definition", "The ABI definition entry matched from the dictionary of ABIs")
	DecodedSignature                              = ffm("ABIDecodedData.signature", "The signature of the matched ABI definition")
)

// query/query_json.go
var (
	QueryJSONStatements         = ffm("QueryJSON.statements", "Query statements")
	QueryJSONLimit              = ffm("QueryJSON.limit", "Query limit")
	QueryJSONSort               = ffm("QueryJSON.sort", "Query sort order")
	FilterResultsWithCountCount = ffm("FilterResultsWithCount.count", "Number of items returned")
	FilterResultsWithCountTotal = ffm("FilterResultsWithCount.total", "Total number of items available")
	FilterResultsWithCountItems = ffm("FilterResultsWithCount.items", "Returned items")
	ItemsResultTypedCount       = ffm("ItemsResultTyped.count", "Number of items returned")
	ItemsResultTypedTotal       = ffm("ItemsResultTyped.total", "Total number of items available")
	ItemsResultTypedItems       = ffm("ItemsResultTyped.items", "Returned items")
	OpNot                       = ffm("Op.not", "Negate the operation")
	OpCaseInsensitive           = ffm("Op.caseInsensitive", "Perform case-insensitive matching")
	OpField                     = ffm("Op.field", "Field to apply the operation to")
	OpSingleValValue            = ffm("OpSingleVal.value", "Value to compare against")
	OpMultiValValues            = ffm("OpMultiVal.values", "Values to compare against")
	StatementsOr                = ffm("Statements.or", "List of alternative statements")
	OpsEqual                    = ffm("Ops.equal", "Equal to")
	OpsEq                       = ffm("Ops.eq", "Equal to (short name)")
	OpsNEq                      = ffm("Ops.neq", "Not equal to")
	OpsLike                     = ffm("Ops.like", "Like")
	OpsLessThan                 = ffm("Ops.lessThan", "Less than")
	OpsLT                       = ffm("Ops.lt", "Less than (short name)")
	OpsLessThanOrEqual          = ffm("Ops.lessThanOrEqual", "Less than or equal to")
	OpsLTE                      = ffm("Ops.lte", "Less than or equal to (short name)")
	OpsGreaterThan              = ffm("Ops.greaterThan", "Greater than")
	OpsGT                       = ffm("Ops.gt", "Greater than (short name)")
	OpsGreaterThanOrEqual       = ffm("Ops.greaterThanOrEqual", "Greater than or equal to")
	OpsGTE                      = ffm("Ops.gte", "Greater than or equal to (short name)")
	OpsIn                       = ffm("Ops.in", "In")
	OpsNIn                      = ffm("Ops.nin", "Not in")
	OpsNull                     = ffm("Ops.null", "Null")
)

// pldclient/states.go
var (
	StateID                      = ffm("State.id", "The ID of the state, which is generated from the content per the rules of the domain, and is unique within the contract")
	StateCreated                 = ffm("State.created", "Server-generated creation timestamp for this state (query only)")
	StateDomain                  = ffm("State.domain", "The name of the domain this state is managed by")
	StateSchema                  = ffm("State.schema", "The ID of the schema for this state, which defines what fields it has and which are indexed for query")
	StateContractAddress         = ffm("State.contractAddress", "The address of the contract that manages this state within the domain")
	StateData                    = ffm("State.data", "The JSON formatted data for this state")
	StateConfirmed               = ffm("State.confirmed", "The confirmation record, if this an on-chain confirmation has been indexed from the base ledger for this state")
	StateSpent                   = ffm("State.spent", "The spend record, if this an on-chain spend has been indexed from the base ledger for this state")
	StateRead                    = ffm("State.read", "Read record, only returned when querying within an in-memory domain context to represent read-lock on a state from a transaction in that domain context")
	StateLocks                   = ffm("State.locks", "When querying states within a domain context running ahead of the blockchain assembling transactions for submission, this provides detail on locks applied to the state")
	StateNullifier               = ffm("State.nullifier", "Only set if nullifiers are being used in the domain, and a nullifier has been generated that is available for spending this state")
	StateConfirmTransaction      = ffm("StateConfirm.transaction", "The ID of the Paladin transaction where this state was confirmed")
	StateSpendTransaction        = ffm("StateSpend.transaction", "The ID of the Paladin transaction where this state was spent")
	StateLockTransaction         = ffm("StateLock.transaction", "The ID of the Paladin transaction being assembled that is responsible for this lock")
	StateLockType                = ffm("StateLock.type", "Whether this lock is for create, read or spend")
	SchemaID                     = ffm("Schema.id", "The hash derived ID of the schema (query only)")
	SchemaCreated                = ffm("Schema.created", "Server-generated creation timestamp for this schema (query only)")
	SchemaDomain                 = ffm("Schema.domain", "The name of the domain this schema is managed by")
	SchemaSignature              = ffm("Schema.signature", "Human readable signature string for this schema, that is used to generate the hash")
	SchemaType                   = ffm("Schema.type", "The type of the schema, such as if it is an ABI defined schema")
	SchemaDefinition             = ffm("Schema.definition", "The definition of the schema, such as the ABI definition")
	SchemaLabels                 = ffm("Schema.labels", "The list of indexed labels that can be used to filter and sort states using to this schema")
	TransactionStatesNone        = ffm("TransactionStates.none", "No state reference records have been indexed for this transaction. Either the transaction has not been indexed, or it did not reference any states")
	TransactionStatesSpent       = ffm("TransactionStates.spent", "Private state data for input states that were spent in this transaction")
	TransactionStatesRead        = ffm("TransactionStates.read", "Private state data for states that were unspent and used during execution of this transaction, but were not spent by it")
	TransactionStatesConfirmed   = ffm("TransactionStates.confirmed", "Private state data for new states that were confirmed as new unspent states during this transaction")
	TransactionStatesInfo        = ffm("TransactionStates.info", "Private state data for states that were recorded as part of this transaction, and existed only as reference data during its execution. They were not validated as unspent during execution, or recorded as new unspent states")
	TransactionStatesUnavailable = ffm("TransactionStates.unavailable", "If present, this contains information about states recorded as used by this transactions when indexing, but for which the private data is unavailable on this node")
	UnavailableStatesSpent       = ffm("UnavailableStates.spent", "The IDs of spent states consumed by this transaction, for which the private data is unavailable")
	UnavailableStatesRead        = ffm("UnavailableStates.read", "The IDs of read states used by this transaction, for which the private data is unavailable")
	UnavailableStatesConfirmed   = ffm("UnavailableStates.confirmed", "The IDs of confirmed states created by this transaction, for which the private data is unavailable")
	UnavailableStatesInfo        = ffm("UnavailableStates.info", "The IDs of info states referenced in this transaction, for which the private data is unavailable")
)

// pldclient/registry.go
var (
	RegistryEntryRegistry                 = ffm("RegistryEntry.registry", "The registry that maintains this record")
	RegistryEntryID                       = ffm("RegistryEntry.id", "The ID of the entry, which is unique within the registry across all records in the hierarchy")
	RegistryEntryName                     = ffm("RegistryEntry.name", "The name of the entry, which is unique across entries with the same parent")
	RegistryParentID                      = ffm("RegistryEntry.parentId", "Unset for a root record, otherwise a reference to another entity in the same registry")
	RegistryEntryWithPropertiesProperties = ffm("RegistryEntryWithProperties.properties", "A name + value pair map of all the active properties for this entry. Only active properties are listed, even if the query on the entries used an activeFilter to return inactive entries")
	RegistryPropertyRegistry              = ffm("RegistryProperty.registry", "The registry that maintains this record")
	RegistryPropertyEntryID               = ffm("RegistryProperty.entryId", "The ID of the entry this property is associated with")
	RegistryPropertyName                  = ffm("RegistryProperty.name", "The name of the property")
	RegistryPropertyValue                 = ffm("RegistryProperty.value", "The value of the property")
	OnChainLocationBlockNumber            = ffm("OnChainLocation.blockNumber", "For Ethereum blockchain backed registries, this is the block number where the registry entry/property was set")
	OnChainLocationTransactionIndex       = ffm("OnChainLocation.transactionIndex", "The transaction index within the block")
	OnChainLocationLogIndex               = ffm("OnChainLocation.logIndex", "The log index within the transaction of the event")
	ActiveFlagActive                      = ffm("ActiveFlag.active", "When querying with an activeFilter of 'any' or 'inactive', this boolean shows if the entry/property is active or not")
)

// pldclient/transport.go
var (
	PeerInfoName              = ffm("PeerInfo.name", "The name of the peer node")
	PeerInfoStats             = ffm("PeerInfo.stats", "Statistics for the outbound and inbound data transfer")
	PeerInfoActivated         = ffm("PeerInfo.activated", "The time when the peer was activated by an attempt to send data, or data arriving on a transport from this peer")
	PeerInfoOutbound          = ffm("PeerInfo.outbound", "Transport specific information about an established outbound connection to the peer. Omitted if the peer does not have an established outbound connection")
	PeerInfoOutboundTransport = ffm("PeerInfo.outboundTransport", "The name of the transport selected for outbound connection to the peer. Omitted if no attempt to send data has occurred for this peer")
	PeerInfoOutboundError     = ffm("PeerInfo.outboundError", "Contains an error if attempting to send data, and the transport connection failed")

	PeerStatsSentMsgs            = ffm("PeerStats.sentMsgs", "Count of messages sent since activation of this peer")
	PeerStatsReceivedMsgs        = ffm("PeerStats.receivedMsgs", "Count of messages received since activation of this peer")
	PeerStatsSentBytes           = ffm("PeerStats.sentBytes", "Count of payload bytes sent since activation of this peer (does not include header data)")
	PeerStatsReceivedBytes       = ffm("PeerStats.receivedBytes", "Count of payload bytes received since activation of this peer (does not include header data)")
	PeerStatsLastSend            = ffm("PeerStats.lastSend", "Timestamp of the last send to this peer")
	PeerStatsLastReceive         = ffm("PeerStats.lastReceive", "Timestamp of the last receive from this peer")
	PeerStatsReliableHighestSent = ffm("PeerStats.reliableHighestSent", "Outbound reliable messages are assigned a sequence. This is the highest sequence sent to the peer since activation")
	PeerStatsReliableAckBase     = ffm("PeerStats.reliableAckBase", "Outbound reliable messages are assigned a sequence. This is the lowest sequence that has not received an acknowledgement from the peer")
)
