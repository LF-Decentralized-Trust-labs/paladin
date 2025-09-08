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

package fungible

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/smt"
	corepb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/node"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"google.golang.org/protobuf/proto"
)

// due to the ZKP circuit needing to check if the amount is positive,
// the maximum transfer amount is (2^100 - 1)
// Reference: https://github.com/hyperledger-labs/zeto/blob/main/zkp/circuits/lib/check-positive.circom
var MAX_TRANSFER_AMOUNT = big.NewInt(0).Exp(big.NewInt(2), big.NewInt(100), nil)

type baseHandler struct {
	name         string
	stateSchemas *common.StateSchemas
}

func (h *baseHandler) getAlgoZetoSnarkBJJ() string {
	return getAlgoZetoSnarkBJJ(h.name)
}

func validateTransferParams(ctx context.Context, params []*types.FungibleTransferParamEntry) error {
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

func validateBalanceOfParams(ctx context.Context, param *types.FungibleBalanceOfParam) error {
	if param.Account == "" {
		return i18n.NewError(ctx, msgs.MsgNoParamAccount)
	}
	return nil
}

func validateAmountParam(ctx context.Context, amount *pldtypes.HexUint256, i int) error {
	if amount == nil {
		return i18n.NewError(ctx, msgs.MsgNoParamAmount, i)
	}
	if amount.Int().Sign() != 1 {
		return i18n.NewError(ctx, msgs.MsgParamAmountInRange, i)
	}
	return nil
}

func utxosFromInputStates(ctx context.Context, states []*prototk.EndorsableState, desiredSize int) ([]string, error) {
	return utxosFromStates(ctx, states, desiredSize, true)
}

func utxosFromOutputStates(ctx context.Context, states []*prototk.EndorsableState, desiredSize int) ([]string, error) {
	return utxosFromStates(ctx, states, desiredSize, false)
}

func utxosFromStates(ctx context.Context, states []*prototk.EndorsableState, desiredSize int, isInputs bool) ([]string, error) {
	utxos := make([]string, desiredSize)
	for i := 0; i < desiredSize; i++ {
		if i < len(states) {
			msgTemplate := msgs.MsgErrorParseInputStates
			if !isInputs {
				msgTemplate = msgs.MsgErrorParseOutputStates
			}
			state := states[i]
			coin, err := makeCoin(state.StateDataJson)
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

func generateMerkleProofs(ctx context.Context, smtSpec *common.MerkleTreeSpec, indexes []*big.Int, targetSize int) (*corepb.MerkleProofObject, error) {
	// verify that the input UTXOs have been indexed by the Merkle tree DB
	// and generate a merkle proof for each
	mtRoot := smtSpec.Tree.Root()
	proofs, _, err := smtSpec.Tree.GenerateProofs(indexes, mtRoot)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
	}
	var mps []*corepb.MerkleProof
	var enabled []bool
	for i, proof := range proofs {
		cp, err := proof.ToCircomVerifierProof(indexes[i], indexes[i], mtRoot, smtSpec.Levels)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorConvertToCircomProof, err)
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
	// if the proofs are less than the target size, we need to fill the rest with empty proofs
	size := len(mps)
	for i := size; i < targetSize; i++ {
		mps = append(mps, smtSpec.EmptyProof)
		enabled = append(enabled, false)
	}
	smtProof := &corepb.MerkleProofObject{
		Root:         mtRoot.BigInt().Text(16),
		MerkleProofs: mps,
		Enabled:      enabled,
	}

	return smtProof, nil
}

func makeLeafIndexesFromCoins(ctx context.Context, inputCoins []*types.ZetoCoin, mt core.SparseMerkleTree) ([]*big.Int, error) {
	var indexes []*big.Int
	for _, coin := range inputCoins {
		pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(coin.Owner.String())
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}
		// Create a new fungible node for the coin, to check existence
		// in the Merkle tree. The index is calculated from the coin's
		// amount, owner and salt.
		idx := node.NewFungible(coin.Amount.Int(), pubKey, coin.Salt.Int())
		leaf, err := node.NewLeafNode(idx)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorNewLeafNode, err)
		}
		// Check if the leaf exists in the Merkle tree
		n, err := mt.GetNode(leaf.Ref())
		if err != nil {
			// TODO: deal with when the node is not found in the DB tables for the tree
			// e.g because the transaction event hasn't been processed yet
			return nil, i18n.NewError(ctx, msgs.MsgErrorQueryLeafNode, leaf.Ref().Hex(), err)
		}
		// Check if the index of the node returned from the merkle tree
		// matches the expected index calculated from the coin's amount, owner and salt.
		hash, err := coin.Hash(ctx)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
		}
		if n.Index().BigInt().Cmp(hash.Int()) != 0 {
			expectedIndex, err := node.NewNodeIndexFromBigInt(hash.Int())
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorNewNodeIndex, err)
			}
			// we have found a node in the tree based on its primary key (ref),
			// but the node's index doesn't match the expected index based on
			// the coin's amount, owner and salt. This is an error situation
			return nil, i18n.NewError(ctx, msgs.MsgErrorHashMismatch, leaf.Ref().Hex(), n.Index().BigInt().Text(16), n.Index().Hex(), hash.HexString0xPrefix(), expectedIndex.Hex())
		}
		indexes = append(indexes, n.Index().BigInt())
	}
	return indexes, nil
}

func makeLeafIndexesFromCoinOwners(ctx context.Context, inputOwner string, outputCoins []*types.ZetoCoin) ([]*big.Int, error) {
	indexes := make([]*big.Int, len(outputCoins)+1) // +1 for the input owner
	// the first index is for the input owner
	pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(inputOwner)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
	}
	hash, err := poseidon.Hash([]*big.Int{pubKey.X, pubKey.Y})
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
	}
	indexes[0] = hash
	// the rest of the indexes are for the output coins
	for i, coin := range outputCoins {
		pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(coin.Owner.String())
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}
		hash, err := poseidon.Hash([]*big.Int{pubKey.X, pubKey.Y})
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorHashOutputState, err)
		}
		indexes[i+1] = hash
	}
	return indexes, nil
}

// formatTransferProvingRequest formats the proving request for a transfer transaction.
// the same function is used for both the transfer and lock transactions because they
// both require the same proof from the transfer circuit
func formatTransferProvingRequest(ctx context.Context, callbacks plugintk.DomainCallbacks, merkleTreeRootSchema *prototk.StateSchema, merkleTreeNodeSchema *prototk.StateSchema, inputCoins, outputCoins []*types.ZetoCoin, circuit *zetosignerapi.Circuit, tokenName, stateQueryContext string, contractAddress *pldtypes.EthAddress, delegate ...string) ([]byte, error) {
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

		smtProof, err := smtProofForInputs(ctx, callbacks, merkleTreeRootSchema, merkleTreeNodeSchema, tokenName, stateQueryContext, contractAddress, inputCoins, forLockedStates, inputSize)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
		}

		var extrasObj proto.Message
		if !circuit.UsesKyc {
			extrasObj = &corepb.ProvingRequestExtras_Nullifiers{
				SmtProof: smtProof,
			}
			if len(delegate) > 0 {
				extrasObj.(*corepb.ProvingRequestExtras_Nullifiers).Delegate = delegate[0]
			}
		} else {
			// for KYC, we need the additional proof for the KYC states
			smtProofKyc, err := smtProofForOwners(ctx, callbacks, merkleTreeRootSchema, merkleTreeNodeSchema, tokenName, stateQueryContext, contractAddress, inputOwner, outputCoins, inputSize+1)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
			}

			extrasObj = &corepb.ProvingRequestExtras_NullifiersKyc{
				SmtUtxoProof: smtProof,
				SmtKycProof:  smtProofKyc,
			}
			if len(delegate) > 0 {
				extrasObj.(*corepb.ProvingRequestExtras_NullifiersKyc).Delegate = delegate[0]
			}
		}
		protoExtras, err := proto.Marshal(extrasObj)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalExtraObj, err)
		}
		extras = protoExtras
	}
	tokenSecrets, err := marshalTokenSecrets(inputValueInts, outputValueInts)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalValuesFungible, err)
	}
	payload := &corepb.ProvingRequest{
		Circuit: circuit.ToProto(),
		Common: &corepb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputSalts:       inputSalts,
			InputOwner:       inputOwner,
			OutputSalts:      outputSalts,
			OutputOwners:     outputOwners,
			TokenSecrets:     tokenSecrets,
			TokenType:        corepb.TokenType_fungible,
		},
	}
	if extras != nil {
		payload.Extras = extras
	}
	return proto.Marshal(payload)
}

func smtProofForInputs(ctx context.Context, callbacks plugintk.DomainCallbacks, merkleTreeRootSchema *prototk.StateSchema, merkleTreeNodeSchema *prototk.StateSchema, tokenName, stateQueryContext string, contractAddress *pldtypes.EthAddress, inputCoins []*types.ZetoCoin, forLockedStates bool, targetSize int) (*corepb.MerkleProofObject, error) {
	smtName := smt.MerkleTreeName(tokenName, contractAddress)
	if forLockedStates {
		smtName = smt.MerkleTreeNameForLockedStates(tokenName, contractAddress)
	}
	smtType := common.StatesTree
	if forLockedStates {
		smtType = common.LockedStatesTree
	}

	mt, err := common.NewMerkleTreeSpec(ctx, smtName, smtType, callbacks, merkleTreeRootSchema.Id, merkleTreeNodeSchema.Id, stateQueryContext)
	if err != nil {
		return nil, err
	}

	var indexes []*big.Int
	indexes, err = makeLeafIndexesFromCoins(ctx, inputCoins, mt.Tree)
	if err != nil {
		return nil, err
	}

	smtProof, err := generateMerkleProofs(ctx, mt, indexes, targetSize)
	if err != nil {
		return nil, err
	}
	return smtProof, nil
}

func smtProofForOwners(ctx context.Context, callbacks plugintk.DomainCallbacks, merkleTreeRootSchema *prototk.StateSchema, merkleTreeNodeSchema *prototk.StateSchema, tokenName, stateQueryContext string, contractAddress *pldtypes.EthAddress, inputOwner string, outputCoins []*types.ZetoCoin, targetSize int) (*corepb.MerkleProofObject, error) {
	smtName := smt.MerkleTreeNameForKycStates(tokenName, contractAddress)
	mt, err := common.NewMerkleTreeSpec(ctx, smtName, common.KycStatesTree, callbacks, merkleTreeRootSchema.Id, merkleTreeNodeSchema.Id, stateQueryContext)
	if err != nil {
		return nil, err
	}

	// for KYC, we need to collect the indexes from the coins owners
	indexes, err := makeLeafIndexesFromCoinOwners(ctx, inputOwner, outputCoins)
	if err != nil {
		return nil, err
	}

	smtProof, err := generateMerkleProofs(ctx, mt, indexes, targetSize)
	if err != nil {
		return nil, err
	}
	return smtProof, nil
}

func trimZeroUtxos(utxos []string) []string {
	trimmed := make([]string, 0, len(utxos))
	for _, utxo := range utxos {
		if utxo != "0" {
			trimmed = append(trimmed, utxo)
		}
	}
	return trimmed
}

func getAlgoZetoSnarkBJJ(name string) string {
	return zetosignerapi.AlgoDomainZetoSnarkBJJ(name)
}

func marshalTokenSecrets(input, output []uint64) ([]byte, error) {
	return json.Marshal(corepb.TokenSecrets_Fungible{InputValues: input, OutputValues: output})
}
