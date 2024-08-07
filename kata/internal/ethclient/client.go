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
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/internal/rpcclient"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"golang.org/x/crypto/sha3"
)

// Higher level client interface to the base Ethereum ledger for TX submission.
// See blockindexer package for the events side, including WaitForTransaction()
type EthClient interface {
	Close()
	CallContract(ctx context.Context, from *string, to *ethtypes.Address0xHex, callData ethtypes.HexBytes0xPrefix) (data ethtypes.HexBytes0xPrefix, err error)
	SendTransaction(ctx context.Context, from string, to *ethtypes.Address0xHex, callData ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error)
}

type KeyManager interface {
	ResolveKey(ctx context.Context, identifier string, algorithm string) (keyHandle, verifier string, err error)
	Sign(ctx context.Context, req *proto.SignRequest) (*proto.SignResponse, error)
}

type ethClient struct {
	chainID int64
	rpc     rpcbackend.RPC
	keymgr  KeyManager
}

func NewEthClient(ctx context.Context, keymgr KeyManager, conf *Config) (_ EthClient, err error) {
	var rpc rpcbackend.RPC
	if conf.HTTP.URL != "" {
		// Use HTTP by preference (provides parallelism on performance)
		rpcConf, err := rpcclient.ParseHTTPConfig(ctx, &conf.HTTP)
		if err == nil {
			rpc = rpcbackend.NewRPCClient(rpcConf)
		}
	} else {
		// Otherwise use WS
		var wsRPC rpcbackend.WebSocketRPCClient
		var wsConf *wsclient.WSConfig
		wsConf, err = rpcclient.ParseWSConfig(ctx, &conf.WS)
		if err == nil {
			wsRPC = rpcbackend.NewWSRPCClient(wsConf)
		}
		if err == nil {
			err = wsRPC.Connect(ctx)
		}
		rpc = wsRPC
	}
	if err != nil {
		return nil, err
	}
	return WrapRPCClient(ctx, keymgr, rpc)
}

func WrapRPCClient(ctx context.Context, keymgr KeyManager, rpc rpcbackend.RPC) (EthClient, error) {
	ec := &ethClient{
		keymgr: keymgr,
		rpc:    rpc,
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

func (ec *ethClient) setupChainID(ctx context.Context) error {
	var chainID ethtypes.HexUint64
	if rpcErr := ec.rpc.CallRPC(ctx, &chainID, "eth_chainId"); rpcErr != nil {
		return fmt.Errorf("eth_chainId failed: %s", rpcErr.Error())
	}
	ec.chainID = int64(chainID.Uint64())
	return nil
}

func (ec *ethClient) CallContract(ctx context.Context, from *string, to *ethtypes.Address0xHex, callData ethtypes.HexBytes0xPrefix) (data ethtypes.HexBytes0xPrefix, err error) {

	tx := ethsigner.Transaction{
		To:   to,
		Data: callData,
	}

	if from != nil {
		_, fromAddr, err := ec.keymgr.ResolveKey(ctx, *from, signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
		if err != nil {
			return nil, err
		}
		tx.From = json.RawMessage(fmt.Sprintf(`"%s"`, fromAddr))
	}

	if rpcErr := ec.rpc.CallRPC(ctx, &data, "eth_call", tx); rpcErr != nil {
		log.L(ctx).Errorf("eth_call failed: %+v", rpcErr)
		return nil, rpcErr.Error()
	}

	return data, err

}

func (ec *ethClient) SendTransaction(ctx context.Context, from string, to *ethtypes.Address0xHex, callData ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error) {

	// Resolve the key (directly with the signer - we have no key manager here in the teseced)
	keyHandle, fromAddr, err := ec.keymgr.ResolveKey(ctx, from, signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES)
	if err != nil {
		return nil, err
	}

	// Trivial nonce management in the client - just get the current nonce for this key, from the local node mempool, for each TX
	var nonce ethtypes.HexInteger
	if rpcErr := ec.rpc.CallRPC(ctx, &nonce, "eth_getTransactionCount", fromAddr, "latest"); rpcErr != nil {
		log.L(ctx).Errorf("eth_getTransactionCount(%s) failed: %+v", fromAddr, rpcErr)
		return nil, rpcErr.Error()
	}

	// Construct a simple transaction with the specified data for a permissioned zero-gas-price chain
	tx := ethsigner.Transaction{
		From: json.RawMessage(fmt.Sprintf(`"%s"`, fromAddr)), // for estimation of gas we need the from address
		To:   to,
		Data: callData,
	}

	// Estimate gas before submission
	var gasEstimate ethtypes.HexInteger
	if rpcErr := ec.rpc.CallRPC(ctx, &gasEstimate, "eth_estimateGas", tx); rpcErr != nil {
		log.L(ctx).Errorf("eth_estimateGas failed: %+v", rpcErr)
		return nil, rpcErr.Error()
	}

	// If that went well, so submission with twice that gas as the limit.
	tx.GasLimit = ethtypes.NewHexInteger(new(big.Int).Mul(gasEstimate.BigInt(), big.NewInt(2)))
	tx.Nonce = &nonce

	// Sign
	sigPayload := tx.SignaturePayloadEIP1559(ec.chainID)
	hash := sha3.NewLegacyKeccak256()
	_, _ = hash.Write(sigPayload.Bytes())
	signature, err := ec.keymgr.Sign(ctx, &proto.SignRequest{
		Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		KeyHandle: keyHandle,
		Payload:   ethtypes.HexBytes0xPrefix(hash.Sum(nil)),
	})
	var sig *secp256k1.SignatureData
	if err == nil {
		sig, err = signer.DecodeCompactRSV(ctx, signature.Payload)
	}
	var rawTX []byte
	if err == nil {
		rawTX, err = tx.FinalizeEIP1559WithSignature(sigPayload, sig)
	}
	if err != nil {
		log.L(ctx).Errorf("signing failed with keyHandle %s (addr=%s): %s", keyHandle, fromAddr, err)
		return nil, err
	}

	// Submit
	var txHash ethtypes.HexBytes0xPrefix
	if rpcErr := ec.rpc.CallRPC(ctx, &txHash, "eth_sendRawTransaction", ethtypes.HexBytes0xPrefix(rawTX)); rpcErr != nil {
		addr, decodedTX, err := ethsigner.RecoverRawTransaction(ctx, rawTX, ec.chainID)
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
	return txHash, nil
}

func logJSON(v interface{}) string {
	ret := ""
	b, _ := json.Marshal(v)
	if len(b) > 0 {
		ret = (string)(b)
	}
	return ret
}
