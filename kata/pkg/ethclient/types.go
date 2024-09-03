/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package ethclient

import (
	"strings"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

// ErrorReason are a set of standard error conditions that a blockchain connector can return
// from execution, that affect the action of the transaction manager to the response.
// It is important that error mapping is performed for each of these classification
type ErrorReason string

// *** MUST UPDATE MapSubmissionRejected IF ADDING NEW REASONS THAT ARE POSSIBLE DURING TRANSACTION PREPARE PHASE OF SUBMISSION ***
const (
	// ErrorReasonInvalidInputs transaction inputs could not be parsed by the connector according to the interface (nothing was sent to the blockchain)
	ErrorReasonInvalidInputs ErrorReason = "invalid_inputs"
	// ErrorReasonTransactionReverted on-chain execution (only expected to be returned when the connector is doing gas estimation, or executing a query)
	ErrorReasonTransactionReverted ErrorReason = "transaction_reverted"
	// ErrorReasonNonceTooLow on transaction submission, if the nonce has already been used for a transaction that has made it into a block on the canonical chain known to the local node
	ErrorReasonNonceTooLow ErrorReason = "nonce_too_low"
	// ErrorReasonTransactionUnderpriced if the transaction is rejected due to too low gas price. Either because it was too low according to the minimum configured on the node, or because it's a rescue transaction without a price bump.
	ErrorReasonTransactionUnderpriced ErrorReason = "transaction_underpriced"
	// ErrorReasonInsufficientFunds if the transaction is rejected due to not having enough of the underlying network coin (ether etc.) in your wallet
	ErrorReasonInsufficientFunds ErrorReason = "insufficient_funds"
	// ErrorReasonNotFound if the requested object (block/receipt etc.) was not found
	ErrorReasonNotFound ErrorReason = "not_found"
	// ErrorKnownTransaction if the exact transaction is already known
	ErrorKnownTransaction ErrorReason = "known_transaction"
	// ErrorReasonDownstreamDown if the downstream JSONRPC endpoint is down
	ErrorReasonDownstreamDown ErrorReason = "downstream_down"
)

func MapSubmissionRejected(err error) bool {
	reason := MapError(err)
	switch reason {
	case ErrorReasonInvalidInputs,
		ErrorReasonTransactionReverted,
		ErrorReasonInsufficientFunds:
		// These reason codes are considered as rejections of the transaction - see SubmissionError
		return true
	default:
		// Everything else is eligible for idempotent retry of submission
		return false
	}
}

func MapError(err error) ErrorReason {

	errString := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errString, "filter not found"):
		return ErrorReasonNotFound
	case strings.Contains(errString, "nonce too low"):
		return ErrorReasonNonceTooLow
	case strings.Contains(errString, "insufficient funds"):
		return ErrorReasonInsufficientFunds
	case strings.Contains(errString, "transaction underpriced"):
		return ErrorReasonTransactionUnderpriced
	case strings.Contains(errString, "known transaction"):
		return ErrorKnownTransaction
	case strings.Contains(errString, "already known"):
		return ErrorKnownTransaction
	case strings.Contains(errString, "execution reverted"):
		return ErrorReasonTransactionReverted
	// https://docs.avax.network/quickstart/integrate-exchange-with-avalanche#determining-finality
	case strings.Contains(errString, "cannot query unfinalized data"):
		return ErrorReasonNotFound
	case strings.Contains(errString, "the method net_version does not exist/is not available"):
		return ErrorReasonNotFound
	default:
		// default to no mapping
		return ""
	}
}

// txReceiptJSONRPC is the receipt obtained over JSON/RPC from the ethereum client, with gas used, logs and contract address
type txReceiptJSONRPC struct {
	BlockHash         ethtypes.HexBytes0xPrefix  `json:"blockHash"`
	BlockNumber       *ethtypes.HexInteger       `json:"blockNumber"`
	ContractAddress   *ethtypes.Address0xHex     `json:"contractAddress"`
	CumulativeGasUsed *ethtypes.HexInteger       `json:"cumulativeGasUsed"`
	From              *ethtypes.Address0xHex     `json:"from"`
	GasUsed           *ethtypes.HexInteger       `json:"gasUsed"`
	Logs              []*logJSONRPC              `json:"logs"`
	Status            *ethtypes.HexInteger       `json:"status"`
	To                *ethtypes.Address0xHex     `json:"to"`
	TransactionHash   ethtypes.HexBytes0xPrefix  `json:"transactionHash"`
	TransactionIndex  *ethtypes.HexInteger       `json:"transactionIndex"`
	RevertReason      *ethtypes.HexBytes0xPrefix `json:"revertReason"`
}

type logJSONRPC struct {
	Removed          bool                        `json:"removed"`
	LogIndex         *ethtypes.HexInteger        `json:"logIndex"`
	TransactionIndex *ethtypes.HexInteger        `json:"transactionIndex"`
	BlockNumber      *ethtypes.HexInteger        `json:"blockNumber"`
	TransactionHash  ethtypes.HexBytes0xPrefix   `json:"transactionHash"`
	BlockHash        ethtypes.HexBytes0xPrefix   `json:"blockHash"`
	Address          *ethtypes.Address0xHex      `json:"address"`
	Data             ethtypes.HexBytes0xPrefix   `json:"data"`
	Topics           []ethtypes.HexBytes0xPrefix `json:"topics"`
}

type TransactionReceiptResponse struct {
	BlockNumber      *fftypes.FFBigInt `json:"blockNumber"`
	TransactionIndex *fftypes.FFBigInt `json:"transactionIndex"`
	BlockHash        string            `json:"blockHash"`
	Success          bool              `json:"success"`
	ProtocolID       string            `json:"protocolId"`
	ExtraInfo        *fftypes.JSONAny  `json:"extraInfo,omitempty"`
	ContractLocation *fftypes.JSONAny  `json:"contractLocation,omitempty"`
	Logs             []fftypes.JSONAny `json:"logs,omitempty"` // all raw un-decoded logs should be included if includeLogs=true
}

// receiptExtraInfo is the version of the receipt we store under the TX.
// - We omit the full logs from the JSON/RPC
// - We omit fields already in the standardized cross-blockchain section
// - We format numbers as decimals
type receiptExtraInfo struct {
	ContractAddress   *ethtypes.Address0xHex `json:"contractAddress"`
	CumulativeGasUsed *fftypes.FFBigInt      `json:"cumulativeGasUsed"`
	From              *ethtypes.Address0xHex `json:"from"`
	To                *ethtypes.Address0xHex `json:"to"`
	GasUsed           *fftypes.FFBigInt      `json:"gasUsed"`
	Status            *fftypes.FFBigInt      `json:"status"`
	ErrorMessage      *string                `json:"errorMessage"`
	ReturnValue       *string                `json:"returnValue,omitempty"`
}

type txDebugTrace struct {
	Gas         *fftypes.FFBigInt `json:"gas"`
	Failed      bool              `json:"failed"`
	ReturnValue string            `json:"returnValue"`
	StructLogs  []StructLog       `json:"structLogs"`
}

type StructLog struct {
	PC      *fftypes.FFBigInt `json:"pc"`
	Op      *string           `json:"op"`
	Gas     *fftypes.FFBigInt `json:"gas"`
	GasCost *fftypes.FFBigInt `json:"gasCost"`
	Depth   *fftypes.FFBigInt `json:"depth"`
	Stack   []*string         `json:"stack"`
	Memory  []*string         `json:"memory"`
	Reason  *string           `json:"reason"`
}
