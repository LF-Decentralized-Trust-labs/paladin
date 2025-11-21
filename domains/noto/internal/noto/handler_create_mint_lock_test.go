/*
 * Copyright © 2025 Kaleido, Inc.
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
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateMintLock(t *testing.T) {
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       &prototk.StateSchema{Id: "coin"},
		lockedCoinSchema: &prototk.StateSchema{Id: "lockedCoin"},
		lockInfoSchemaV0: &prototk.StateSchema{Id: "lockInfo"},
		lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfo_v1"},
		dataSchemaV0:     &prototk.StateSchema{Id: "data"},
		dataSchemaV1:     &prototk.StateSchema{Id: "data_v1"},
	}
	ctx := context.Background()
	fn := types.NotoABI.Functions()["createMintLock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiver1Address := "0x2000000000000000000000000000000000000000"
	receiver2Address := "0x3000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress,
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				NotaryLookup: "notary@node1",
				Variant:      types.NotoVariantDefault, // V1 required for createMintLock
			}),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: `{
			"recipients": [{
				"to": "receiver1@node2",
				"amount": 60
			}, {
				"to": "receiver2@node3",
				"amount": 40
			}],
			"data": "0x1234"
		}`,
	}

	initRes, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.NoError(t, err)
	require.Len(t, initRes.RequiredVerifiers, 4)
	assert.Equal(t, "notary@node1", initRes.RequiredVerifiers[0].Lookup)
	assert.Equal(t, "sender@node1", initRes.RequiredVerifiers[1].Lookup)
	assert.Equal(t, "receiver1@node2", initRes.RequiredVerifiers[2].Lookup)
	assert.Equal(t, "receiver2@node3", initRes.RequiredVerifiers[3].Lookup)

	verifiers := []*prototk.ResolvedVerifier{
		{
			Lookup:       "notary@node1",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     notaryAddress,
		},
		{
			Lookup:       "sender@node1",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     senderKey.Address.String(),
		},
		{
			Lookup:       "receiver1@node2",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     receiver1Address,
		},
		{
			Lookup:       "receiver2@node3",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     receiver2Address,
		},
	}

	assembleRes, err := n.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, assembleRes.AssemblyResult)
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 4) // data, lockInfo, and 2 coins

	// Check that we have 2 output coins (one for each recipient)
	coinCount := 0
	var lockInfoState *prototk.NewState
	for _, state := range assembleRes.AssembledTransaction.InfoStates {
		if state.SchemaId == "coin" {
			coinCount++
		}
		if state.SchemaId == "lockInfo_v1" {
			lockInfoState = state
		}
	}
	assert.Equal(t, 2, coinCount)

	// Verify lock info
	require.NotNil(t, lockInfoState)
	lockInfo, err := n.unmarshalLock(lockInfoState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), lockInfo.Owner.String())
	assert.False(t, lockInfo.LockID.IsZero())

	// Verify output coins
	var outputCoin1, outputCoin2 *types.NotoCoin
	for _, state := range assembleRes.AssembledTransaction.InfoStates {
		if state.SchemaId == "coin" {
			coin, err := n.unmarshalCoin(state.StateDataJson)
			require.NoError(t, err)
			if coin.Owner.String() == receiver1Address {
				outputCoin1 = coin
			} else if coin.Owner.String() == receiver2Address {
				outputCoin2 = coin
			}
		}
	}
	require.NotNil(t, outputCoin1)
	require.NotNil(t, outputCoin2)
	assert.Equal(t, "60", outputCoin1.Amount.Int().String())
	assert.Equal(t, "40", outputCoin2.Amount.Int().String())

	// Verify data
	var dataState *prototk.NewState
	for _, state := range assembleRes.AssembledTransaction.InfoStates {
		if state.SchemaId == "data_v1" {
			dataState = state
			break
		}
	}
	require.NotNil(t, dataState)
	outputInfo, err := n.unmarshalInfo(dataState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())

	// Encode unlock for signature
	encodedUnlock, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), nil, nil, []*types.NotoCoin{outputCoin1, outputCoin2})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedUnlock)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	infoStates := []*prototk.EndorsableState{}
	for _, state := range assembleRes.AssembledTransaction.InfoStates {
		infoStates = append(infoStates, &prototk.EndorsableState{
			SchemaId:      state.SchemaId,
			Id:            pldtypes.RandBytes32().String(), // Mock state ID
			StateDataJson: state.StateDataJson,
		})
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		Reads:             []*prototk.EndorsableState{},
		Info:              infoStates,
		EndorsementRequest: &prototk.AttestationRequest{
			Name: "notary",
		},
		Signatures: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: senderKey.Address.String()},
				Payload:  signatureBytes,
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, endorseRes.EndorsementResult)

	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		ReadStates:        []*prototk.EndorsableState{},
		InfoStates:        infoStates,
		AttestationResult: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: senderKey.Address.String()},
				Payload:  signatureBytes,
			},
			{
				Name:     "notary",
				Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"},
			},
		},
	})
	require.NoError(t, err)
	expectedFunction := mustParseJSON(interfaceBuild.ABI.Functions()["prepareUnlock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Verify base invoke params
	var baseParams NotoPrepareUnlockParams
	err = json.Unmarshal([]byte(prepareRes.Transaction.ParamsJson), &baseParams)
	require.NoError(t, err)
	assert.Equal(t, tx.TransactionId, *baseParams.TxId)
	assert.Equal(t, lockInfo.LockID, *baseParams.LockId)
	assert.NotEmpty(t, baseParams.UnlockTxId)
	assert.Equal(t, []string{}, baseParams.LockedInputs)
	assert.NotEmpty(t, baseParams.UnlockHash)
	assert.Equal(t, signatureBytes, baseParams.Signature)
	assert.NotEmpty(t, baseParams.Data)

	// Prepare again to test hook invoke
	hookAddress := "0x515fba7fe1d8b9181be074bd4c7119544426837c"
	tx.ContractInfo.ContractConfigJson = mustParseJSON(&types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		Variant:      types.NotoVariantDefault, // V1 required for createMintLock
		NotaryMode:   types.NotaryModeHooks.Enum(),
		Options: types.NotoOptions{
			Hooks: &types.NotoHooksOptions{
				PublicAddress:     pldtypes.MustEthAddress(hookAddress),
				DevUsePublicHooks: true,
			},
		},
	})
	prepareRes, err = n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		ReadStates:        []*prototk.EndorsableState{},
		InfoStates:        infoStates,
		AttestationResult: []*prototk.AttestationResult{
			{
				Name:     "sender",
				Verifier: &prototk.ResolvedVerifier{Verifier: senderKey.Address.String()},
				Payload:  signatureBytes,
			},
			{
				Name:     "notary",
				Verifier: &prototk.ResolvedVerifier{Lookup: "notary@node1"},
			},
		},
	})
	require.NoError(t, err)
	expectedFunction = mustParseJSON(hooksBuild.ABI.Functions()["onCreateMintLock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)

	// Verify hook invoke params
	var hookParams UnlockHookParams
	err = json.Unmarshal([]byte(prepareRes.Transaction.ParamsJson), &hookParams)
	require.NoError(t, err)
	require.NotNil(t, hookParams.Sender)
	assert.Equal(t, senderKey.Address.String(), hookParams.Sender.String())
	assert.Equal(t, lockInfo.LockID, hookParams.LockID)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x1234"), hookParams.Data)

	// Verify recipients
	require.Len(t, hookParams.Recipients, 2)
	require.NotNil(t, hookParams.Recipients[0].To)
	assert.Equal(t, pldtypes.MustEthAddress("0x2000000000000000000000000000000000000000").String(), hookParams.Recipients[0].To.String())
	require.NotNil(t, hookParams.Recipients[0].Amount)
	assert.Equal(t, pldtypes.Int64ToInt256(60).String(), hookParams.Recipients[0].Amount.String())
	require.NotNil(t, hookParams.Recipients[1].To)
	assert.Equal(t, pldtypes.MustEthAddress("0x3000000000000000000000000000000000000000").String(), hookParams.Recipients[1].To.String())
	require.NotNil(t, hookParams.Recipients[1].Amount)
	assert.Equal(t, pldtypes.Int64ToInt256(40).String(), hookParams.Recipients[1].Amount.String())

	// Verify prepared transaction
	assert.Equal(t, pldtypes.MustEthAddress(contractAddress), hookParams.Prepared.ContractAddress)
	assert.NotEmpty(t, hookParams.Prepared.EncodedCall, "encodedCall should be present and non-empty")
}
