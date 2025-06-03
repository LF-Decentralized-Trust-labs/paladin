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
	"math/big"

	"encoding/json"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

func (n *Noto) GetHandler(method string) types.DomainHandler {
	switch method {
	case "mint":
		return &mintHandler{noto: n}
	case "transfer":
		return &transferHandler{noto: n}
	case "burn":
		return &burnHandler{noto: n}
	case "approveTransfer":
		return &approveHandler{noto: n}
	case "lock":
		return &lockHandler{noto: n}
	case "unlock":
		return &unlockHandler{
			unlockCommon: unlockCommon{noto: n},
		}
	case "prepareUnlock":
		return &prepareUnlockHandler{
			unlockCommon: unlockCommon{noto: n},
		}
	case "delegateLock":
		return &delegateLockHandler{noto: n}
	default:
		return nil
	}
}

func (n *Noto) GetCallHandler(method string) types.DomainCallHandler {
	switch method {
	case "balanceOf":
		return &balanceOfHandler{noto: n}
	default:
		return nil
	}
}

// Check that a mint has no inputs, and an output matching the requested amount
func (n *Noto) validateMintAmounts(ctx context.Context, params *types.MintParams, inputs, outputs *parsedCoins) error {
	if len(inputs.coins) > 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidInputs, "mint", inputs.coins)
	}
	if outputs.total.Cmp(params.Amount.Int()) != 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidAmount, "mint", params.Amount.Int().Text(10), outputs.total.Text(10))
	}
	return nil
}

// Check that a transfer has at least one input and output, and they net out to zero
func (n *Noto) validateTransferAmounts(ctx context.Context, inputs, outputs *parsedCoins) error {
	if len(inputs.coins) == 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidInputs, "transfer", inputs.coins)
	}
	if inputs.total.Cmp(outputs.total) != 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidAmount, "transfer", inputs.total, outputs.total)
	}
	return nil
}

// Check that a burn has at least one input, and a net output matching the requested amount
func (n *Noto) validateBurnAmounts(ctx context.Context, params *types.BurnParams, inputs, outputs *parsedCoins) error {
	if len(inputs.coins) == 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidInputs, "burn", inputs.coins)
	}
	amount := big.NewInt(0).Sub(inputs.total, outputs.total)
	if amount.Cmp(params.Amount.Int()) != 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidAmount, "burn", params.Amount.Int().Text(10), amount.Text(10))
	}
	return nil
}

// Check that a lock produces locked coins matching the difference between the inputs and outputs
func (n *Noto) validateLockAmounts(ctx context.Context, inputs, outputs *parsedCoins) error {
	if len(inputs.coins) == 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidInputs, "lock", inputs.coins)
	}
	amount := big.NewInt(0).Sub(inputs.total, outputs.total)
	if amount.Cmp(outputs.lockedTotal) != 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidAmount, "lock", outputs.lockedTotal.Text(10), amount.Text(10))
	}
	return nil
}

// Check that an unlock produces unlocked coins matching the difference between the locked inputs and outputs
func (n *Noto) validateUnlockAmounts(ctx context.Context, inputs, outputs *parsedCoins) error {
	if len(inputs.lockedCoins) == 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidInputs, "unlock", inputs.lockedCoins)
	}
	amount := big.NewInt(0).Sub(inputs.lockedTotal, outputs.lockedTotal)
	if amount.Cmp(outputs.total) != 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidAmount, "unlock", outputs.total.Text(10), amount.Text(10))
	}
	return nil
}

// Check that the sender of a transaction provided a signature on the input details
func (n *Noto) validateSignature(ctx context.Context, name string, attestations []*prototk.AttestationResult, encodedMessage []byte) error {
	signature := domain.FindAttestation(name, attestations)
	if signature == nil {
		return i18n.NewError(ctx, msgs.MsgAttestationNotFound, name)
	}
	recoveredSignature, err := n.recoverSignature(ctx, encodedMessage, signature.Payload)
	if err != nil {
		return err
	}
	if recoveredSignature.String() != signature.Verifier.Verifier {
		return i18n.NewError(ctx, msgs.MsgSignatureDoesNotMatch, name, signature.Verifier.Verifier, recoveredSignature.String())
	}
	return nil
}

// Check that all coins are owned by the transaction sender
func (n *Noto) validateOwners(ctx context.Context, owner string, req *prototk.EndorseTransactionRequest, coins []*types.NotoCoin, states []*prototk.StateRef) error {
	fromAddress, err := n.findEthAddressVerifier(ctx, "from", owner, req.ResolvedVerifiers)
	if err != nil {
		return err
	}

	for i, coin := range coins {
		if !coin.Owner.Equals(fromAddress) {
			return i18n.NewError(ctx, msgs.MsgStateWrongOwner, states[i].Id, owner)
		}
	}
	return nil
}

// Check that all locked coins are owned by the transaction sender
func (n *Noto) validateLockOwners(ctx context.Context, owner string, verifiers []*prototk.ResolvedVerifier, coins []*types.NotoLockedCoin, states []*prototk.StateRef) error {
	fromAddress, err := n.findEthAddressVerifier(ctx, "from", owner, verifiers)
	if err != nil {
		return err
	}
	for i, coin := range coins {
		if !coin.Owner.Equals(fromAddress) {
			return i18n.NewError(ctx, msgs.MsgStateWrongOwner, states[i].Id, owner)
		}
	}
	return nil
}

// Parse a resolved verifier as an eth address
func (n *Noto) findEthAddressVerifier(ctx context.Context, label, lookup string, verifierList []*prototk.ResolvedVerifier) (*pldtypes.EthAddress, error) {
	verifier := domain.FindVerifier(lookup, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, verifierList)
	if verifier == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, label)
	}
	return pldtypes.ParseEthAddress(verifier.Verifier)
}

type TransactionWrapper struct {
	transactionType prototk.PreparedTransaction_TransactionType
	functionABI     *abi.Entry
	paramsJSON      []byte
	contractAddress *pldtypes.EthAddress
}

func (tw *TransactionWrapper) prepare(metadata []byte) (*prototk.PrepareTransactionResponse, error) {
	functionJSON, err := json.Marshal(tw.functionABI)
	if err != nil {
		return nil, err
	}
	var contractAddress *string
	if tw.contractAddress != nil {
		addr := tw.contractAddress.String()
		contractAddress = &addr
	}
	res := &prototk.PrepareTransactionResponse{
		Transaction: &prototk.PreparedTransaction{
			Type:            tw.transactionType,
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(tw.paramsJSON),
			ContractAddress: contractAddress,
		},
	}
	if metadata != nil {
		metadataString := string(metadata)
		res.Metadata = &metadataString
	}
	return res, nil
}

func (tw *TransactionWrapper) encode(ctx context.Context) ([]byte, error) {
	return tw.functionABI.EncodeCallDataJSONCtx(ctx, tw.paramsJSON)
}
