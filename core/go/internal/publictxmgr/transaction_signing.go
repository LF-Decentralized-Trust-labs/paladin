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

package publictxmgr

import (
	"context"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"golang.org/x/crypto/sha3"
)

func (it *inFlightTransactionStageController) signTx(ctx context.Context, from pldtypes.EthAddress, ethTx *ethsigner.Transaction) ([]byte, *pldtypes.Bytes32, error) {
	log.L(ctx).Debugf("signTx entry")
	signStart := time.Now()

	// Reverse resolve the key - to get to this point it will be in the key management system
	resolvedKey, err := it.keymgr.ReverseKeyLookup(ctx, it.p.NOTX(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, from.String())
	if err != nil {
		log.L(ctx).Errorf("signing failed to resolve key %s for signing: %s", from.String(), err)
		it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationSign), string(GenericStatusFail), time.Since(signStart).Seconds())
		return nil, nil, err
	}
	// Sign
	sigPayload := ethTx.SignaturePayloadEIP1559(it.ethClient.ChainID())
	sigPayloadHash := sha3.NewLegacyKeccak256()
	_, err = sigPayloadHash.Write(sigPayload.Bytes())
	var signatureRSV []byte
	if err == nil {
		signatureRSV, err = it.keymgr.Sign(ctx, resolvedKey, signpayloads.OPAQUE_TO_RSV, pldtypes.HexBytes(sigPayloadHash.Sum(nil)))
	}
	var sig *secp256k1.SignatureData
	if err == nil {
		sig, err = secp256k1.DecodeCompactRSV(ctx, signatureRSV)
	}
	var signedMessage []byte
	if err == nil {
		signedMessage, err = ethTx.FinalizeEIP1559WithSignature(sigPayload, sig)
	}
	if err != nil {
		log.L(ctx).Errorf("signing failed with keyHandle %s (addr=%s): %s", resolvedKey.KeyHandle, resolvedKey.Verifier.Verifier, err)
		it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationSign), string(GenericStatusFail), time.Since(signStart).Seconds())
		return nil, nil, err
	}
	calculatedHash := calculateTransactionHash(signedMessage)
	log.L(ctx).Debugf("Calculated Hash %s of transaction %s:%d", calculatedHash, ethTx.From, ethTx.Nonce.Uint64())
	it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationSign), string(GenericStatusSuccess), time.Since(signStart).Seconds())
	return signedMessage, calculatedHash, err
}
