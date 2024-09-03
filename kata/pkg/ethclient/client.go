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
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"golang.org/x/crypto/sha3"
)

// Higher level client interface to the base Ethereum ledger for TX submission.
// See blockindexer package for the events side, including WaitForTransaction()
type EthClient interface {
	Close()
	ABI(ctx context.Context, a abi.ABI) (ABIClient, error)
	ABIJSON(ctx context.Context, abiJson []byte) (ABIClient, error)
	ABIFunction(ctx context.Context, functionABI *abi.Entry) (_ ABIFunctionClient, err error)
	ABIConstructor(ctx context.Context, constructorABI *abi.Entry, bytecode types.HexBytes) (_ ABIFunctionClient, err error)
	MustABIJSON(abiJson []byte) ABIClient
	ChainID() int64

	// Below are raw functions that the ABI() above provides wrappers for
	GasPrice(ctx context.Context) (gasPrice *ethtypes.HexInteger, err error)
	GetBalance(ctx context.Context, address string, block string) (balance *ethtypes.HexInteger, err error)
	GasEstimate(ctx context.Context, tx *ethsigner.Transaction) (gasLimit *ethtypes.HexInteger, err error)
	GetTransactionCount(ctx context.Context, fromAddr string) (transactionCount *ethtypes.HexUint64, err error)
	GetTransactionReceipt(ctx context.Context, txHash string) (*TransactionReceiptResponse, error)
	CallContract(ctx context.Context, from *string, tx *ethsigner.Transaction, block string) (data types.HexBytes, err error)
	BuildRawTransaction(ctx context.Context, txVersion EthTXVersion, from string, tx *ethsigner.Transaction) (types.HexBytes, error)
	SendRawTransaction(ctx context.Context, rawTX types.HexBytes) (*types.Bytes32, error)
}

type KeyManager interface {
	ResolveKey(ctx context.Context, identifier string, algorithm string) (keyHandle, verifier string, err error)
	Sign(ctx context.Context, req *proto.SignRequest) (*proto.SignResponse, error)
	Close()
}

type ethClient struct {
	chainID           int64
	gasEstimateFactor float64
	rpc               rpcbackend.RPC
	keymgr            KeyManager
}

// A direct creation of a dedicated RPC client for things like unit tests outside of Paladin.
// Within Paladin, use the EthClientFactory instead as passed to your component/manager/engine via the initialization
func WrapRPCClient(ctx context.Context, keymgr KeyManager, rpc rpcbackend.RPC, conf *Config) (EthClient, error) {
	ec := &ethClient{
		keymgr:            keymgr,
		rpc:               rpc,
		gasEstimateFactor: confutil.Float64Min(conf.GasEstimateFactor, 1.0, *Defaults.GasEstimateFactor),
	}
	if err := ec.setupChainID(ctx); err != nil {
		return nil, err
	}
	return ec, nil
}

func (ec *ethClient) Close() {
	wsRPC, isWS := ec.rpc.(rpcbackend.WebSocketRPCClient)
	if isWS {
		wsRPC.Close()
	}
}

func (ec *ethClient) ChainID() int64 {
	return ec.chainID
}

func (ec *ethClient) setupChainID(ctx context.Context) error {
	var chainID ethtypes.HexUint64
	if rpcErr := ec.rpc.CallRPC(ctx, &chainID, "eth_chainId"); rpcErr != nil {
		log.L(ctx).Errorf("eth_chainId failed: %+v", rpcErr)
		return i18n.WrapError(ctx, rpcErr.Error(), msgs.MsgEthClientChainIDFailed)
	}
	ec.chainID = int64(chainID.Uint64())
	return nil
}

func (ec *ethClient) CallContract(ctx context.Context, from *string, tx *ethsigner.Transaction, block string) (data types.HexBytes, err error) {

	if from != nil {
		_, fromAddr, err := ec.keymgr.ResolveKey(ctx, *from, algorithms.ECDSA_SECP256K1_PLAINBYTES)
		if err != nil {
			return nil, err
		}
		tx.From = json.RawMessage(types.JSONString(fromAddr))
	}

	if rpcErr := ec.rpc.CallRPC(ctx, &data, "eth_call", tx, block); rpcErr != nil {
		log.L(ctx).Errorf("eth_call failed: %+v", rpcErr)
		return nil, rpcErr.Error()
	}

	return data, err

}

func (ec *ethClient) GetBalance(ctx context.Context, address string, block string) (*ethtypes.HexInteger, error) {
	var addressBalance ethtypes.HexInteger

	if rpcErr := ec.rpc.CallRPC(ctx, &addressBalance, "eth_getBalance", address, block); rpcErr != nil {
		log.L(ctx).Errorf("eth_getBalance failed: %+v", rpcErr)
		return nil, rpcErr.Error()
	}
	return &addressBalance, nil
}

func (ec *ethClient) GasPrice(ctx context.Context) (*ethtypes.HexInteger, error) {
	// currently only support London style gas price
	// For EIP1559, will need to add support for `eth_maxPriorityFeePerGas`
	var gasPrice ethtypes.HexInteger

	if rpcErr := ec.rpc.CallRPC(ctx, &gasPrice, "eth_gasPrice"); rpcErr != nil {
		log.L(ctx).Errorf("eth_gasPrice failed: %+v", rpcErr)
		return nil, rpcErr.Error()
	}
	return &gasPrice, nil
}

func (ec *ethClient) GetTransactionReceipt(ctx context.Context, txHash string) (*TransactionReceiptResponse, error) {

	// Get the receipt in the back-end JSON/RPC format
	var ethReceipt *txReceiptJSONRPC
	rpcErr := ec.rpc.CallRPC(ctx, &ethReceipt, "eth_getTransactionReceipt", txHash)
	if rpcErr != nil {
		return nil, rpcErr.Error()
	}
	if ethReceipt == nil {
		return nil, i18n.NewError(ctx, msgs.MsgReceiptNotAvailable, txHash)
	}
	isSuccess := (ethReceipt.Status != nil && ethReceipt.Status.BigInt().Int64() > 0)

	var returnDataString *string
	var transactionErrorMessage *string

	if !isSuccess {
		returnDataString, transactionErrorMessage = ec.getErrorInfo(ctx, ethReceipt.RevertReason)
	}

	fullReceipt, _ := json.Marshal(&receiptExtraInfo{
		ContractAddress:   ethReceipt.ContractAddress,
		CumulativeGasUsed: (*fftypes.FFBigInt)(ethReceipt.CumulativeGasUsed),
		From:              ethReceipt.From,
		To:                ethReceipt.To,
		GasUsed:           (*fftypes.FFBigInt)(ethReceipt.GasUsed),
		Status:            (*fftypes.FFBigInt)(ethReceipt.Status),
		ReturnValue:       returnDataString,
		ErrorMessage:      transactionErrorMessage,
	})

	var txIndex int64
	if ethReceipt.TransactionIndex != nil {
		txIndex = ethReceipt.TransactionIndex.BigInt().Int64()
	}
	receiptResponse := &TransactionReceiptResponse{
		BlockNumber:      (*fftypes.FFBigInt)(ethReceipt.BlockNumber),
		TransactionIndex: fftypes.NewFFBigInt(txIndex),
		BlockHash:        ethReceipt.BlockHash.String(),
		Success:          isSuccess,
		ProtocolID:       ProtocolIDForReceipt((*fftypes.FFBigInt)(ethReceipt.BlockNumber), fftypes.NewFFBigInt(txIndex)),
		ExtraInfo:        fftypes.JSONAnyPtrBytes(fullReceipt),
	}

	if ethReceipt.ContractAddress != nil {
		location, _ := json.Marshal(map[string]string{
			"address": ethReceipt.ContractAddress.String(),
		})
		receiptResponse.ContractLocation = fftypes.JSONAnyPtrBytes(location)
	}
	return receiptResponse, nil
}

func (ec *ethClient) GasEstimate(ctx context.Context, tx *ethsigner.Transaction) (*ethtypes.HexInteger, error) {
	var gasEstimate ethtypes.HexInteger
	if rpcErr := ec.rpc.CallRPC(ctx, &gasEstimate, "eth_estimateGas", tx); rpcErr != nil {
		log.L(ctx).Errorf("eth_estimateGas failed: %+v", rpcErr)
		return nil, rpcErr.Error()
	}
	return &gasEstimate, nil
}

func (ec *ethClient) GetTransactionCount(ctx context.Context, fromAddr string) (*ethtypes.HexUint64, error) {
	var transactionCount ethtypes.HexUint64
	if rpcErr := ec.rpc.CallRPC(ctx, &transactionCount, "eth_getTransactionCount", fromAddr, "latest"); rpcErr != nil {
		log.L(ctx).Errorf("eth_getTransactionCount(%s) failed: %+v", fromAddr, rpcErr)
		return nil, rpcErr.Error()
	}
	return &transactionCount, nil
}

func (ec *ethClient) BuildRawTransaction(ctx context.Context, txVersion EthTXVersion, from string, tx *ethsigner.Transaction) (types.HexBytes, error) {
	// Resolve the key (directly with the signer - we have no key manager here in the teseced)
	keyHandle, fromAddr, err := ec.keymgr.ResolveKey(ctx, from, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	if err != nil {
		return nil, err
	}
	tx.From = json.RawMessage(types.JSONString(fromAddr))

	// Trivial nonce management in the client - just get the current nonce for this key, from the local node mempool, for each TX
	if tx.Nonce == nil {
		txNonce, err := ec.GetTransactionCount(ctx, fromAddr)
		if err != nil {
			log.L(ctx).Errorf("eth_getTransactionCount(%s) failed: %+v", fromAddr, err)
			return nil, err
		}
		tx.Nonce = ethtypes.NewHexInteger(big.NewInt(int64(txNonce.Uint64())))
	}

	if tx.GasLimit == nil {
		// Estimate gas before submission
		gasEstimate, err := ec.GasEstimate(ctx, tx)
		if err != nil {
			log.L(ctx).Errorf("eth_estimateGas failed: %+v", err)
			return nil, err
		}
		// If that went well, so submission with a bump on the estimation
		gasLimitFactored := new(big.Float).SetInt(gasEstimate.BigInt())
		gasLimitFactored = gasLimitFactored.Mul(gasLimitFactored, big.NewFloat(ec.gasEstimateFactor))
		gasLimit, _ := gasLimitFactored.Int(nil)
		tx.GasLimit = ethtypes.NewHexInteger(gasLimit)
	}

	// Sign
	var sigPayload *ethsigner.TransactionSignaturePayload
	switch txVersion {
	case EIP1559:
		sigPayload = tx.SignaturePayloadEIP1559(ec.chainID)
	case LEGACY_EIP155:
		sigPayload = tx.SignaturePayloadLegacyEIP155(ec.chainID)
	case LEGACY_ORIGINAL:
		sigPayload = tx.SignaturePayloadLegacyOriginal()
	default:
		return nil, i18n.NewError(ctx, msgs.MsgEthClientInvalidTXVersion, txVersion)
	}
	hash := sha3.NewLegacyKeccak256()
	_, _ = hash.Write(sigPayload.Bytes())
	signature, err := ec.keymgr.Sign(ctx, &proto.SignRequest{
		Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
		KeyHandle: keyHandle,
		Payload:   types.HexBytes(hash.Sum(nil)),
	})
	var sig *secp256k1.SignatureData
	if err == nil {
		sig, err = secp256k1.DecodeCompactRSV(ctx, signature.Payload)
	}
	var rawTX []byte
	if err == nil {
		switch txVersion {
		case EIP1559:
			rawTX, err = tx.FinalizeEIP1559WithSignature(sigPayload, sig)
		case LEGACY_EIP155:
			rawTX, err = tx.FinalizeLegacyEIP155WithSignature(sigPayload, sig, ec.chainID)
		case LEGACY_ORIGINAL:
			rawTX, err = tx.FinalizeLegacyOriginalWithSignature(sigPayload, sig)
		}
	}
	if err != nil {
		log.L(ctx).Errorf("signing failed with keyHandle %s (addr=%s): %s", keyHandle, fromAddr, err)
		return nil, err
	}
	return rawTX, nil
}

func (ec *ethClient) SendRawTransaction(ctx context.Context, rawTX types.HexBytes) (*types.Bytes32, error) {

	// Submit
	var txHash types.Bytes32
	if rpcErr := ec.rpc.CallRPC(ctx, &txHash, "eth_sendRawTransaction", types.HexBytes(rawTX)); rpcErr != nil {
		addr, decodedTX, err := ethsigner.RecoverRawTransaction(ctx, ethtypes.HexBytes0xPrefix(rawTX), ec.chainID)
		if err != nil {
			log.L(ctx).Errorf("Invalid transaction build during signing: %s", err)
		} else {
			log.L(ctx).Errorf("Rejected TX (from=%s): %+v", addr, logJSON(decodedTX.Transaction))
		}
		return nil, fmt.Errorf("eth_sendRawTransaction failed: %+v", rpcErr)
	}

	// We just return the hash here - see blockindexer.BlockIndexer
	// - to wait for completion see BlockIndexer.WaitForTransaction()
	// - to query events for that completed transaction see BlockIndexer.ListTransactionEvents()
	// - to stream events in order (whether you submitted them or not) see BlockIndexer.TODO()
	return &txHash, nil
}

func logJSON(v interface{}) string {
	ret := ""
	b, _ := json.Marshal(v)
	if len(b) > 0 {
		ret = (string)(b)
	}
	return ret
}
func (ec *ethClient) getErrorInfo(ctx context.Context, revertFromReceipt *ethtypes.HexBytes0xPrefix) (pReturnValue *string, pErrorMessage *string) {

	var revertReason string
	if revertFromReceipt != nil {
		revertReason = revertFromReceipt.String()
	}

	// See if the return value is using the default error you get from "revert"
	var errorMessage string
	returnDataBytes, _ := hex.DecodeString(padHexData(revertReason))
	if len(returnDataBytes) > 4 && bytes.Equal(returnDataBytes[0:4], defaultErrorID) {
		value, err := defaultError.DecodeCallDataCtx(ctx, returnDataBytes)
		if err == nil {
			errorMessage = value.Children[0].Value.(string)
		}
	}

	// Otherwise we can't decode it, so put it directly in the error
	if errorMessage == "" {
		if len(returnDataBytes) > 0 {
			errorMessage = i18n.NewError(ctx, msgs.MsgReturnValueNotDecoded, revertReason).Error()
		} else {
			errorMessage = i18n.NewError(ctx, msgs.MsgReturnValueNotAvailable).Error()
		}
	}
	return &revertReason, &errorMessage
}
