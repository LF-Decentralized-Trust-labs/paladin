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

package zeto

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func useNullifiers(circuitId string) bool {
	return circuitId == constants.CIRCUIT_ANON_NULLIFIER || circuitId == constants.CIRCUIT_ANON_NULLIFIER_BATCH
}

// the Zeto implementations support two input/output sizes for the circuits: 2 and 10,
// if the input or output size is larger than 2, then the batch circuit is used with
// input/output size 10
func getInputSize(sizeOfEndorsableStates int) int {
	if sizeOfEndorsableStates <= 2 {
		return 2
	}
	return 10
}

func loadBabyJubKey(payload []byte) (*babyjub.PublicKey, error) {
	var keyCompressed babyjub.PublicKeyComp
	if err := keyCompressed.UnmarshalText(payload); err != nil {
		return nil, err
	}
	return keyCompressed.Decompress()
}

func validateTransferParams(ctx context.Context, params []*types.TransferParamEntry) error {
	if len(params) == 0 {
		return i18n.NewError(ctx, msgs.MsgNoTransferParams)
	}
	for _, param := range params {
		if param.To == "" {
			return i18n.NewError(ctx, msgs.MsgNoParamTo)
		}
		if param.Amount == nil {
			return i18n.NewError(ctx, msgs.MsgNoParamAmount)
		}
		if param.Amount.Int().Sign() != 1 {
			return i18n.NewError(ctx, msgs.MsgParamAmountGtZero)
		}
	}
	return nil
}

func encodeTransactionData(ctx context.Context, transaction *prototk.TransactionSpecification) (tktypes.HexBytes, error) {
	txID, err := tktypes.ParseHexBytes(ctx, transaction.TransactionId)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseTxId, err)
	}
	var data []byte
	data = append(data, types.ZetoTransactionData_V0...)
	data = append(data, txID...)
	return data, nil
}

func decodeTransactionData(data tktypes.HexBytes) (txID tktypes.HexBytes) {
	if len(data) < 4 {
		return nil
	}
	dataPrefix := data[0:4]
	if dataPrefix.String() != types.ZetoTransactionData_V0.String() {
		return nil
	}
	return data[4:]
}
