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
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/node"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/smt"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
)

// due to the ZKP circuit needing to check if the amount is positive,
// the maximum transfer amount is (2^100 - 1)
// Reference: https://github.com/hyperledger-labs/zeto/blob/main/zkp/circuits/lib/check-positive.circom
var MAX_TRANSFER_AMOUNT = big.NewInt(0).Exp(big.NewInt(2), big.NewInt(100), nil)

// the Zeto implementations support two input/output sizes for the circuits: 2 and 10,
// if the input or output size is larger than 2, then the batch circuit is used with
// input/output size 10
func getInputSize(sizeOfEndorsableStates int) int {
	if sizeOfEndorsableStates <= 2 {
		return 2
	}
	return 10
}

func validateTransferParams(ctx context.Context, params []*types.TransferParamEntry) error {
	if len(params) == 0 {
		return i18n.NewError(ctx, msgs.MsgNoTransferParams)
	}
	total := big.NewInt(0)
	for i, param := range params {
		if param.To == "" {
			return i18n.NewError(ctx, msgs.MsgNoParamTo, i)
		}
		if err := validateAmountParam(ctx, param.Amount, i); err != nil {
			return err
		}
		total.Add(total, param.Amount.Int())
	}
	if total.Cmp(MAX_TRANSFER_AMOUNT) >= 0 {
		return i18n.NewError(ctx, msgs.MsgParamTotalAmountInRange)
	}

	return nil
}

func validateAmountParam(ctx context.Context, amount *tktypes.HexUint256, i int) error {
	if amount == nil {
		return i18n.NewError(ctx, msgs.MsgNoParamAmount, i)
	}
	if amount.Int().Sign() != 1 {
		return i18n.NewError(ctx, msgs.MsgParamAmountInRange, i)
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

func encodeProof(proof *corepb.SnarkProof) map[string]interface{} {
	// Convert the proof json to the format that the Solidity verifier expects
	return map[string]interface{}{
		"pA": []string{proof.A[0], proof.A[1]},
		"pB": [][]string{
			{proof.B[0].Items[1], proof.B[0].Items[0]},
			{proof.B[1].Items[1], proof.B[1].Items[0]},
		},
		"pC": []string{proof.C[0], proof.C[1]},
	}
}

func loadBabyJubKey(payload []byte) (*babyjub.PublicKey, error) {
	var keyCompressed babyjub.PublicKeyComp
	if err := keyCompressed.UnmarshalText(payload); err != nil {
		return nil, err
	}
	return keyCompressed.Decompress()
}

func utxosFromInputStates(ctx context.Context, zeto *Zeto, states []*prototk.EndorsableState, desiredSize int) ([]string, error) {
	return _utxosFromStates(ctx, zeto, states, desiredSize, true)
}

func utxosFromOutputStates(ctx context.Context, zeto *Zeto, states []*prototk.EndorsableState, desiredSize int) ([]string, error) {
	return _utxosFromStates(ctx, zeto, states, desiredSize, false)
}

func _utxosFromStates(ctx context.Context, zeto *Zeto, states []*prototk.EndorsableState, desiredSize int, isInputs bool) ([]string, error) {
	utxos := make([]string, desiredSize)
	for i := 0; i < desiredSize; i++ {
		if i < len(states) {
			msgTemplate := msgs.MsgErrorParseInputStates
			if !isInputs {
				msgTemplate = msgs.MsgErrorParseOutputStates
			}
			state := states[i]
			coin, err := zeto.makeCoin(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgTemplate, err)
			}
			hash, err := coin.Hash(ctx)
			if err != nil {
				return nil, i18n.NewError(ctx, msgTemplate, err)
			}
			utxos[i] = hash.String()
		} else {
			utxos[i] = "0"
		}
	}
	return utxos, nil
}

func generateMerkleProofs(ctx context.Context, zeto *Zeto, tokenName string, stateQueryContext string, contractAddress *tktypes.EthAddress, inputCoins []*types.ZetoCoin, locked bool) ([]core.Proof, *corepb.ProvingRequestExtras_Nullifiers, error) {
	smtName := smt.MerkleTreeName(tokenName, contractAddress)
	if locked {
		smtName = smt.MerkleTreeNameForLockedStates(tokenName, contractAddress)
	}
	storage := smt.NewStatesStorage(zeto.Callbacks, smtName, stateQueryContext, zeto.merkleTreeRootSchema.Id, zeto.merkleTreeNodeSchema.Id)
	mt, err := smt.NewSmt(storage)
	if err != nil {
		return nil, nil, i18n.NewError(ctx, msgs.MsgErrorNewSmt, smtName, err)
	}
	// verify that the input UTXOs have been indexed by the Merkle tree DB
	// and generate a merkle proof for each
	var indexes []*big.Int
	for _, coin := range inputCoins {
		pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(coin.Owner.String())
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}
		idx := node.NewFungible(coin.Amount.Int(), pubKey, coin.Salt.Int())
		leaf, err := node.NewLeafNode(idx)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorNewLeafNode, err)
		}
		n, err := mt.GetNode(leaf.Ref())
		if err != nil {
			// TODO: deal with when the node is not found in the DB tables for the tree
			// e.g because the transaction event hasn't been processed yet
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorQueryLeafNode, leaf.Ref().Hex(), err)
		}
		hash, err := coin.Hash(ctx)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
		}
		if n.Index().BigInt().Cmp(hash.Int()) != 0 {
			expectedIndex, err := node.NewNodeIndexFromBigInt(hash.Int())
			if err != nil {
				return nil, nil, i18n.NewError(ctx, msgs.MsgErrorNewNodeIndex, err)
			}
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorHashMismatch, leaf.Ref().Hex(), n.Index().BigInt().Text(16), n.Index().Hex(), hash.HexString0xPrefix(), expectedIndex.Hex())
		}
		indexes = append(indexes, n.Index().BigInt())
	}
	mtRoot := mt.Root()
	proofs, _, err := mt.GenerateProofs(indexes, mtRoot)
	if err != nil {
		return nil, nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
	}
	var mps []*corepb.MerkleProof
	var enabled []bool
	for i, proof := range proofs {
		cp, err := proof.ToCircomVerifierProof(indexes[i], indexes[i], mtRoot, smt.SMT_HEIGHT_UTXO)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorConvertToCircomProof, err)
		}
		proofSiblings := make([]string, len(cp.Siblings)-1)
		for i, s := range cp.Siblings[0 : len(cp.Siblings)-1] {
			proofSiblings[i] = s.BigInt().Text(16)
		}
		p := corepb.MerkleProof{
			Nodes: proofSiblings,
		}
		mps = append(mps, &p)
		enabled = append(enabled, true)
	}
	extrasObj := corepb.ProvingRequestExtras_Nullifiers{
		Root:         mt.Root().BigInt().Text(16),
		MerkleProofs: mps,
		Enabled:      enabled,
	}

	return proofs, &extrasObj, nil
}

// formatTransferProvingRequest formats the proving request for a transfer transaction.
// the same function is used for both the transfer and lock transactions because they
// both require the same proof from the transfer circuit
func formatTransferProvingRequest(ctx context.Context, zeto *Zeto, inputCoins, outputCoins []*types.ZetoCoin, circuit *zetosignerapi.Circuit, tokenName, stateQueryContext string, contractAddress *tktypes.EthAddress, delegate ...string) ([]byte, error) {
	inputSize := common.GetInputSize(len(inputCoins))
	inputCommitments := make([]string, inputSize)
	inputValueInts := make([]uint64, inputSize)
	inputSalts := make([]string, inputSize)
	inputOwner := inputCoins[0].Owner.String()
	for i := 0; i < inputSize; i++ {
		if i < len(inputCoins) {
			coin := inputCoins[i]
			hash, err := coin.Hash(ctx)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
			}
			inputCommitments[i] = hash.Int().Text(16)
			inputValueInts[i] = coin.Amount.Int().Uint64()
			inputSalts[i] = coin.Salt.Int().Text(16)
		} else {
			inputCommitments[i] = "0"
			inputSalts[i] = "0"
		}
	}

	outputValueInts := make([]uint64, inputSize)
	outputSalts := make([]string, inputSize)
	outputOwners := make([]string, inputSize)
	for i := 0; i < inputSize; i++ {
		if i < len(outputCoins) {
			coin := outputCoins[i]
			outputValueInts[i] = coin.Amount.Int().Uint64()
			outputSalts[i] = coin.Salt.Int().Text(16)
			outputOwners[i] = coin.Owner.String()
		} else {
			outputSalts[i] = "0"
		}
	}

	var extras []byte
	if circuit.UsesNullifiers {
		forLockedStates := len(delegate) > 0
		proofs, extrasObj, err := generateMerkleProofs(ctx, zeto, tokenName, stateQueryContext, contractAddress, inputCoins, forLockedStates)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
		}
		for i := len(proofs); i < inputSize; i++ {
			extrasObj.MerkleProofs = append(extrasObj.MerkleProofs, &smt.Empty_Proof)
			extrasObj.Enabled = append(extrasObj.Enabled, false)
		}
		if len(delegate) > 0 {
			extrasObj.Delegate = delegate[0]
		}
		protoExtras, err := proto.Marshal(extrasObj)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalExtraObj, err)
		}
		extras = protoExtras
	}
	payload := &corepb.ProvingRequest{
		Circuit: circuit.ToProto(),
		Common: &corepb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputValues:      inputValueInts,
			InputSalts:       inputSalts,
			InputOwner:       inputOwner,
			OutputValues:     outputValueInts,
			OutputSalts:      outputSalts,
			OutputOwners:     outputOwners,
		},
	}
	if extras != nil {
		payload.Extras = extras
	}
	return proto.Marshal(payload)
}

func trimZeroUtxos(utxos []string) []string {
	var trimmed []string
	for _, utxo := range utxos {
		if utxo != "0" {
			trimmed = append(trimmed, utxo)
		}
	}
	return trimmed
}
