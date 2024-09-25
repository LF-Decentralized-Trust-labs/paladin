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

	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (it *InFlightTransactionStageController) signTx(ctx context.Context, signer *ethclient.ResolvedSigner, ethTx *ethsigner.Transaction) ([]byte, *tktypes.Bytes32, error) {
	log.L(ctx).Debugf("signTx entry")
	signStart := time.Now()
	signedMessage, err := it.ethClient.BuildRawTransactionNoResolve(ctx, ethclient.EIP1559, signer, ethTx)

	if err != nil {
		it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationSign), string(GenericStatusFail), time.Since(signStart).Seconds())
		return nil, nil, err
	}
	calculatedHash := calculateTransactionHash(signedMessage)
	log.L(ctx).Debugf("Calculated Hash %s of transaction %s:%d", calculatedHash, ethTx.From, ethTx.Nonce.Uint64())
	it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationSign), string(GenericStatusSuccess), time.Since(signStart).Seconds())
	return signedMessage, calculatedHash, err
}
