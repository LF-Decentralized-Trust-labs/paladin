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
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/node"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/smt"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"google.golang.org/protobuf/proto"
)

type transferHandler struct {
	zeto *Zeto
}

func (h *transferHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var transferParams types.TransferParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
	}
	if transferParams.To == "" {
		return nil, fmt.Errorf("parameter 'to' is required")
	}
	return &transferParams, nil
}

func (h *transferHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.(*types.TransferParams)

	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.zeto.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
			{
				Lookup:       params.To,
				Algorithm:    h.zeto.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}, nil
}

func (h *transferHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.TransferParams)

	resolvedSender := domain.FindVerifier(tx.Transaction.From, h.zeto.getAlgoZetoSnarkBJJ(), zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, fmt.Errorf("failed to resolve: %s", tx.Transaction.From)
	}
	resolvedRecipient := domain.FindVerifier(params.To, h.zeto.getAlgoZetoSnarkBJJ(), zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedRecipient == nil {
		return nil, fmt.Errorf("failed to resolve: %s", params.To)
	}

	senderKey, err := h.loadBabyJubKey([]byte(resolvedSender.Verifier))
	if err != nil {
		return nil, fmt.Errorf("failed to load sender public key. %s", err)
	}
	recipientKey, err := h.loadBabyJubKey([]byte(resolvedRecipient.Verifier))
	if err != nil {
		return nil, fmt.Errorf("failed load receiver public key. %s", err)
	}

	inputCoins, inputStates, total, err := h.zeto.prepareInputs(ctx, req.Transaction.ContractAddress, tx.Transaction.From, params.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare inputs. %s", err)
	}
	outputCoins, outputStates, err := h.zeto.prepareOutputs(params.To, recipientKey, params.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare outputs. %s", err)
	}
	if total.Cmp(params.Amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.Int())
		returnedCoins, returnedStates, err := h.zeto.prepareOutputs(tx.Transaction.From, senderKey, (*tktypes.HexUint256)(remainder))
		if err != nil {
			return nil, fmt.Errorf("failed to prepare outputs for change coins. %s", err)
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	contract, err := ethtypes.NewAddress(req.Transaction.ContractAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse contract address. %s", err)
	}
	payloadBytes, err := h.formatProvingRequest(inputCoins, outputCoins, tx.DomainConfig.CircuitId, tx.DomainConfig.TokenName, contract)
	if err != nil {
		return nil, fmt.Errorf("failed to format proving request. %s", err)
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			InputStates:  inputStates,
			OutputStates: outputStates,
		},
		AttestationPlan: []*pb.AttestationRequest{
			{
				Name:            "sender",
				AttestationType: pb.AttestationType_SIGN,
				Algorithm:       h.zeto.getAlgoZetoSnarkBJJ(),
				VerifierType:    zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
				PayloadType:     zetosigner.PAYLOAD_DOMAIN_ZETO_SNARK,
				Payload:         payloadBytes,
				Parties:         []string{tx.Transaction.From},
			},
			{
				Name:            "submitter",
				AttestationType: pb.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				Parties:         []string{tx.Transaction.From},
			},
		},
	}, nil
}

func (h *transferHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *transferHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	var proofRes corepb.ProvingResponse
	result := domain.FindAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, fmt.Errorf("did not find 'sender' attestation")
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proving response. %s", err)
	}

	inputs := make([]string, INPUT_COUNT)
	for i := 0; i < INPUT_COUNT; i++ {
		if i < len(req.InputStates) {
			state := req.InputStates[i]
			coin, err := h.zeto.makeCoin(state.StateDataJson)
			if err != nil {
				return nil, fmt.Errorf("failed to parse input states. %s", err)
			}
			hash, err := coin.Hash()
			if err != nil {
				return nil, fmt.Errorf("failed to create Poseidon hash for an input coin. %s", err)
			}
			inputs[i] = hash.String()
		} else {
			inputs[i] = "0"
		}
	}
	outputs := make([]string, OUTPUT_COUNT)
	for i := 0; i < OUTPUT_COUNT; i++ {
		if i < len(req.OutputStates) {
			state := req.OutputStates[i]
			coin, err := h.zeto.makeCoin(state.StateDataJson)
			if err != nil {
				return nil, fmt.Errorf("failed to parse output states. %s", err)
			}
			hash, err := coin.Hash()
			if err != nil {
				return nil, fmt.Errorf("failed to create Poseidon hash for an output coin. %s", err)
			}
			outputs[i] = hash.String()
		} else {
			outputs[i] = "0"
		}
	}

	data, err := encodeTransactionData(ctx, req.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction data. %s", err)
	}
	params := map[string]any{
		"inputs":  inputs,
		"outputs": outputs,
		"proof":   h.encodeProof(proofRes.Proof),
		"data":    data,
	}
	if tx.DomainConfig.TokenName == constants.TOKEN_ANON_ENC {
		params["encryptionNonce"] = proofRes.PublicInputs["encryptionNonce"]
		params["encryptedValues"] = strings.Split(proofRes.PublicInputs["encryptedValues"], ",")
	} else if tx.DomainConfig.TokenName == constants.TOKEN_ANON_NULLIFIER {
		delete(params, "inputs")
		params["nullifiers"] = strings.Split(proofRes.PublicInputs["nullifiers"], ",")
		params["root"] = proofRes.PublicInputs["root"]
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal prepared params to JSON. %s", err)
	}
	contractAbi, err := h.zeto.config.GetContractAbi(tx.DomainConfig.TokenName)
	if err != nil {
		return nil, fmt.Errorf("failed to find abi for the token contract %s. %s", tx.DomainConfig.TokenName, err)
	}
	functionJSON, err := json.Marshal(contractAbi.Functions()["transfer"])
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
	}, nil
}

func (h *transferHandler) loadBabyJubKey(payload []byte) (*babyjub.PublicKey, error) {
	var keyCompressed babyjub.PublicKeyComp
	if err := keyCompressed.UnmarshalText(payload); err != nil {
		return nil, err
	}
	return keyCompressed.Decompress()
}

func (h *transferHandler) formatProvingRequest(inputCoins, outputCoins []*types.ZetoCoin, circuitId, tokenName string, contractAddress *ethtypes.Address0xHex) ([]byte, error) {
	inputCommitments := make([]string, INPUT_COUNT)
	inputValueInts := make([]uint64, INPUT_COUNT)
	inputSalts := make([]string, INPUT_COUNT)
	inputOwner := inputCoins[0].OwnerKey.String()
	for i := 0; i < INPUT_COUNT; i++ {
		if i < len(inputCoins) {
			coin := inputCoins[i]
			hash, err := coin.Hash()
			if err != nil {
				return nil, fmt.Errorf("failed to create Poseidon hash for an input coin. %s", err)
			}
			inputCommitments[i] = hash.Int().Text(16)
			inputValueInts[i] = coin.Amount.Int().Uint64()
			inputSalts[i] = coin.Salt.Int().Text(16)
		} else {
			inputCommitments[i] = "0"
			inputSalts[i] = "0"
		}
	}

	outputValueInts := make([]uint64, OUTPUT_COUNT)
	outputSalts := make([]string, OUTPUT_COUNT)
	outputOwners := make([]string, OUTPUT_COUNT)
	for i := 0; i < OUTPUT_COUNT; i++ {
		if i < len(outputCoins) {
			coin := outputCoins[i]
			outputValueInts[i] = coin.Amount.Int().Uint64()
			outputSalts[i] = coin.Salt.Int().Text(16)
			outputOwners[i] = coin.OwnerKey.String()
		} else {
			outputSalts[i] = "0"
		}
	}

	var extras []byte
	if useNullifiers(circuitId) {
		proofs, extrasObj, err := h.generateMerkleProofs(tokenName, contractAddress, inputCoins)
		if err != nil {
			return nil, fmt.Errorf("failed to generate merkle proofs. %s", err)
		}
		for i := len(proofs); i < INPUT_COUNT; i++ {
			extrasObj.MerkleProofs = append(extrasObj.MerkleProofs, &smt.Empty_Proof)
			extrasObj.Enabled = append(extrasObj.Enabled, false)
		}
		protoExtras, err := proto.Marshal(extrasObj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal the extras object in the proving request. %s", err)
		}
		extras = protoExtras
	}

	payload := &corepb.ProvingRequest{
		CircuitId: circuitId,
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

func (h *transferHandler) encodeProof(proof *corepb.SnarkProof) map[string]interface{} {
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

func (h *transferHandler) generateMerkleProofs(tokenName string, contractAddress *ethtypes.Address0xHex, inputCoins []*types.ZetoCoin) ([]core.Proof, *corepb.ProvingRequestExtras_Nullifiers, error) {
	smtName := smt.MerkleTreeName(tokenName, contractAddress)
	_, mt, err := smt.New(h.zeto.Callbacks, smtName, contractAddress, h.zeto.merkleTreeRootSchema.Id, h.zeto.merkleTreeNodeSchema.Id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create new smt object. %s", err)
	}
	// verify that the input UTXOs have been indexed by the Merkle tree DB
	// and generate a merkle proof for each
	var indexes []*big.Int
	for _, coin := range inputCoins {
		pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(coin.OwnerKey.String())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode owner key. %s", err)
		}
		idx := node.NewFungible(coin.Amount.Int(), pubKey, coin.Salt.Int())
		leaf, err := node.NewLeafNode(idx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create new leaf node. %s", err)
		}
		n, err := mt.GetNode(leaf.Ref())
		if err != nil {
			// TODO: deal with when the node is not found in the DB tables for the tree
			// e.g because the transaction event hasn't been processed yet
			return nil, nil, fmt.Errorf("failed to query the smt DB for leaf node (index=%s). %s", leaf.Index().Hex(), err)
		}
		hash, err := coin.Hash()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Poseidon hash for an input coin. %s", err)
		}
		if n.Index().BigInt().Cmp(hash.Int()) != 0 {
			expectedIndex, err := node.NewNodeIndexFromBigInt(hash.Int())
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create new node index from the local coin hash. %s", err)
			}
			return nil, nil, fmt.Errorf("coin (ref=%s) found in the merkle tree but the persisted hash %s (index=%s) did not match the expected hash %s (index=%s)", leaf.Ref().Hex(), n.Index().BigInt().Text(16), n.Index().Hex(), hash.HexString0xPrefix(), expectedIndex.Hex())
		}
		indexes = append(indexes, n.Index().BigInt())
	}
	mtRoot := mt.Root()
	proofs, _, err := mt.GenerateProofs(indexes, mtRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate merkle proofs. %s", err)
	}
	var mps []*corepb.MerkleProof
	var enabled []bool
	for i, proof := range proofs {
		cp, err := proof.ToCircomVerifierProof(indexes[i], indexes[i], mtRoot, smt.SMT_HEIGHT_UTXO)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to convert to circom verifier proof. %s", err)
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
