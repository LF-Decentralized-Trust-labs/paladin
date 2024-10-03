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

package noto

import (
	"context"

	"encoding/json"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

func (n *Noto) GetHandler(method string) types.DomainHandler {
	switch method {
	case "mint":
		return &mintHandler{noto: n}
	case "transfer":
		return &transferHandler{noto: n}
	case "approveTransfer":
		return &approveHandler{noto: n}
	default:
		return nil
	}
}

// Check that a mint has no inputs, and an output matching the requested amount
func (n *Noto) validateMintAmounts(ctx context.Context, params *types.MintParams, coins *gatheredCoins) error {
	if len(coins.inCoins) > 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidInputs, "mint", coins.inCoins)
	}
	if coins.outTotal.Cmp(params.Amount.Int()) != 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidAmount, "mint", params.Amount.Int().Text(10), coins.outTotal.Text(10))
	}
	return nil
}

// Check that the inputs and outputs of a transfer net out to zero
func (n *Noto) validateTransferAmounts(ctx context.Context, coins *gatheredCoins) error {
	if coins.inTotal.Cmp(coins.outTotal) != 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidAmount, "transfer", coins.inTotal, coins.outTotal)
	}
	return nil
}

// Check that the sender of a transfer provided a signature on the input transaction details
func (n *Noto) validateTransferSignature(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest, coins *gatheredCoins) error {
	signature := domain.FindAttestation("sender", req.Signatures)
	if signature == nil {
		return i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}
	if signature.Verifier.Lookup != tx.Transaction.From {
		return i18n.NewError(ctx, msgs.MsgAttestationUnexpected, "sender", tx.Transaction.From, signature.Verifier.Lookup)
	}
	encodedTransfer, err := n.encodeTransferUnmasked(ctx, tx.ContractAddress, coins.inCoins, coins.outCoins)
	if err != nil {
		return err
	}
	recoveredSignature, err := n.recoverSignature(ctx, encodedTransfer, signature.Payload)
	if err != nil {
		return err
	}
	if recoveredSignature.String() != signature.Verifier.Verifier {
		return i18n.NewError(ctx, msgs.MsgSignatureDoesNotMatch, "sender", signature.Verifier.Verifier, recoveredSignature.String())
	}
	return nil
}

// Check that the sender of an approval provided a signature on the input transaction details
func (n *Noto) validateApprovalSignature(ctx context.Context, req *prototk.EndorseTransactionRequest, transferHash []byte) error {
	signature := domain.FindAttestation("sender", req.Signatures)
	if signature == nil {
		return i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}
	recoveredSignature, err := n.recoverSignature(ctx, transferHash, signature.Payload)
	if err != nil {
		return err
	}
	if recoveredSignature.String() != signature.Verifier.Verifier {
		return i18n.NewError(ctx, msgs.MsgSignatureDoesNotMatch, "sender", signature.Verifier.Verifier, recoveredSignature.String())
	}
	return nil
}

// Check that all input coins are owned by the transaction sender
func (n *Noto) validateOwners(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest, coins *gatheredCoins) error {
	from := domain.FindVerifier(tx.Transaction.From, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if from == nil {
		return i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "from")
	}
	fromAddress, err := tktypes.ParseEthAddress(from.Verifier)
	if err != nil {
		return err
	}

	for i, coin := range coins.inCoins {
		if !coin.Owner.Equals(fromAddress) {
			return i18n.NewError(ctx, msgs.MsgStateWrongOwner, coins.inStates[i].Id, tx.Transaction.From)
		}
	}
	return nil
}

type TransactionWrapper struct {
	functionABI     *abi.Entry
	paramsJSON      []byte
	contractAddress *string
}

func (tw *TransactionWrapper) prepare() (*prototk.PrepareTransactionResponse, error) {
	functionJSON, err := json.Marshal(tw.functionABI)
	if err != nil {
		return nil, err
	}
	return &prototk.PrepareTransactionResponse{
		Transaction: &prototk.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(tw.paramsJSON),
			ContractAddress: tw.contractAddress,
		},
	}, nil
}

func (tw *TransactionWrapper) encode(ctx context.Context) ([]byte, error) {
	return tw.functionABI.EncodeCallDataJSONCtx(ctx, tw.paramsJSON)
}
