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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger-firefly/signer/pkg/ethtypes"
	"github.com/hyperledger-firefly/signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareBurnUnlock(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       testSchema("coin"),
		lockedCoinSchema: testSchema("lockedCoin"),
		lockInfoSchemaV0: testSchema("lockInfo"),
		lockInfoSchemaV1: testSchema("lockInfo_v1"),
		dataSchemaV0:     testSchema("data"),
		dataSchemaV1:     testSchema("data_v1"),
		dataSchemaV2:     testSchema("data_v2"),
		manifestSchema:   testSchema("manifest"),
	}
	ctx := t.Context()
	fn := types.NotoABI.Functions()["prepareBurnUnlock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	lockID := pldtypes.RandBytes32()
	inputCoin := &types.NotoLockedCoinState{
		ID: pldtypes.RandBytes32(),
		Data: types.NotoLockedCoin{
			LockID: lockID,
			Owner:  (*pldtypes.EthAddress)(&senderKey.Address),
			Amount: pldtypes.Uint64ToUint256(100),
		},
	}
	inputLockInfoSalt := pldtypes.RandBytes32()
	inputLockInfo := &prototk.StoredState{
		Id:       "0xa7c7fa6677f6938bb90f9f0ccb3487707fe6a93c527d899f09af497ece2e603b",
		SchemaId: hashName("lockInfo_v1"),
		DataJson: fmt.Sprintf(`{
			"lockId": "%s",
			"salt": "%s",
			"owner": "%s",
			"spender": "%s"
		}`, lockID, inputLockInfoSalt, senderKey.Address, senderKey.Address),
	}
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		switch req.SchemaId {
		case hashName("lockInfo_v1"):
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{inputLockInfo},
			}, nil
		case hashName("lockedCoin"):
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						Id:        inputCoin.ID.String(),
						SchemaId:  hashName("lockedCoin"),
						DataJson:  mustParseJSON(inputCoin.Data),
						CreatedAt: 1000,
					},
				},
			}, nil
		}
		return nil, fmt.Errorf("unmocked query")
	}

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress,
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				NotaryLookup: "notary@node1",
				Variant:      types.NotoVariantV2,
				Options: types.NotoOptions{
					Basic: &types.NotoBasicOptions{
						AllowBurn: confutil.P(true),
					},
				},
			}),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: fmt.Sprintf(`{
		    "lockId": "%s",
			"from": "sender@node1",
			"amount": 100,
			"unlockData": "0x9999",
			"data": "0x1234"
		}`, lockID),
	}

	initRes, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.NoError(t, err)
	require.Len(t, initRes.RequiredVerifiers, 2)
	assert.Equal(t, "notary@node1", initRes.RequiredVerifiers[0].Lookup)
	assert.Equal(t, "sender@node1", initRes.RequiredVerifiers[1].Lookup)

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
	}

	assembleRes, err := n.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
	})
	require.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, assembleRes.AssemblyResult)
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 1)  // old info
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 1) // new info
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 6) // outer-manifest + spend-manifest + cancel-manifest + unlock-data-info + prepare-data-info + cancel-coin

	assert.Equal(t, inputLockInfo.Id, assembleRes.AssembledTransaction.InputStates[0].Id)
	assert.Equal(t, hashName("lockInfo_v1"), assembleRes.AssembledTransaction.OutputStates[0].SchemaId)

	inputCoinState := assembleRes.AssembledTransaction.ReadStates[0]
	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	unlockManifestState := assembleRes.AssembledTransaction.InfoStates[1] // spend manifest
	cancelManifestState := assembleRes.AssembledTransaction.InfoStates[2]
	unlockDataState := assembleRes.AssembledTransaction.InfoStates[3]
	prepareDataState := assembleRes.AssembledTransaction.InfoStates[4]
	cancelCoinState := assembleRes.AssembledTransaction.InfoStates[5]
	newLockInfoState := assembleRes.AssembledTransaction.OutputStates[0]

	assert.Equal(t, inputCoin.ID.String(), inputCoinState.Id)
	cancelCoin, err := n.unmarshalCoin(cancelCoinState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), cancelCoin.Owner.String())
	assert.Equal(t, "100", cancelCoin.Amount.Int().String())
	unlockDataInfo, err := n.unmarshalInfo(unlockDataState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x9999", unlockDataInfo.Data.String())
	prepareDataInfo, err := n.unmarshalInfo(prepareDataState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", prepareDataInfo.Data.String())

	lockInfo, err := n.unmarshalLockV1(newLockInfoState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), lockInfo.Owner.String())
	assert.Equal(t, lockID, lockInfo.LockID)
	require.NotEqual(t, lockInfo.Salt, inputLockInfoSalt)
	require.Equal(t, inputLockInfo.Id, lockInfo.Replaces.String())
	require.Len(t, lockInfo.SpendOutputs, 0)
	require.Len(t, lockInfo.CancelOutputs, 1)
	require.NotEmpty(t, lockInfo.SpendData)
	require.NotEmpty(t, lockInfo.CancelData)
	require.NotEqual(t, lockInfo.SpendData, lockInfo.CancelData) // spend and cancel use distinct manifests

	encodedUnlock, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{&inputCoin.Data}, []*types.NotoLockedCoin{}, []*types.NotoCoin{})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedUnlock)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	readStates := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("lockedCoin"),
			Id:            inputCoin.ID.String(),
			StateDataJson: mustParseJSON(inputCoin.Data),
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      n.dataSchemaV2.Id,
			Id:            *unlockDataState.Id,
			StateDataJson: unlockDataState.StateDataJson,
		},
		{
			SchemaId:      n.coinSchema.Id,
			Id:            *cancelCoinState.Id,
			StateDataJson: cancelCoinState.StateDataJson,
		},
	}
	inputStates := []*prototk.EndorsableState{
		{
			SchemaId:      inputLockInfo.SchemaId,
			Id:            inputLockInfo.Id,
			StateDataJson: inputLockInfo.DataJson,
		},
	}
	outputStates := []*prototk.EndorsableState{
		{
			SchemaId:      n.lockInfoSchemaV1.Id,
			Id:            *newLockInfoState.Id,
			StateDataJson: newLockInfoState.StateDataJson,
		},
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		Reads:             readStates,
		Info:              infoStates,
		Inputs:            inputStates,
		Outputs:           outputStates,
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

	// Prepare once to test base invoke
	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		ReadStates:        readStates,
		InfoStates:        infoStates,
		InputStates:       inputStates,
		OutputStates:      outputStates,
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
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Extract the options from the response to get the generated SpendTxId
	updateLockABI := interfaceV2Build.ABI.Functions()["updateLock"]
	expectedFunction := mustParseJSON(updateLockABI)
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Decode the function parameters
	fnParams := decodeFnParams[UpdateLockParams](t, updateLockABI, prepareRes.Transaction.ParamsJson)
	require.Equal(t, lockID, fnParams.LockID)
	data, err := n.decodeTransactionDataV1(ctx, fnParams.Data) // this is the transaction data for the prepare (not the prepared transaction)
	require.NoError(t, err)
	require.Equal(t, &types.NotoTransactionData_V1{
		InfoStates: []pldtypes.Bytes32{
			pldtypes.MustParseBytes32(*unlockDataState.Id),
			pldtypes.MustParseBytes32(*cancelCoinState.Id),
		},
	}, data)

	// Decode the options we store into the lockInfo
	unlockTxData, err := n.encodeTransactionDataV1(ctx, newStateToEndorsableState([]*prototk.NewState{unlockManifestState, unlockDataState}))
	require.NoError(t, err)
	cancelUnlockTxData, err := n.encodeTransactionDataV1(ctx, newStateToEndorsableState([]*prototk.NewState{cancelManifestState, unlockDataState}))
	require.NoError(t, err)
	notoParams := decodeSingleABITuple[types.NotoUpdateLockArgs](t, types.NotoUpdateLockArgsABI, fnParams.UpdateArgs)
	expectedSpendHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, lockInfo.SpendTxId.HexString(), n.endorsableStateIDs(ctx, nil, readStates, false), []string{}, unlockTxData)
	require.NoError(t, err)
	require.Equal(t, expectedSpendHash, fnParams.SpendCommitment)
	expectedCancelHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, lockInfo.SpendTxId.HexString(), n.endorsableStateIDs(ctx, nil, readStates, false), n.endorsableStateIDs(ctx, nil, infoStates[1:2], false), cancelUnlockTxData)
	require.NoError(t, err)
	require.Equal(t, expectedCancelHash, fnParams.CancelCommitment)

	// Validate the encoded noto parameters passed in
	require.Equal(t, &types.NotoUpdateLockArgs{
		TxId:         "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Contents:     n.endorsableStateIDs(ctx, nil, readStates, false),
		OldLockState: pldtypes.MustParseBytes32(inputLockInfo.Id),
		NewLockState: pldtypes.MustParseBytes32(*newLockInfoState.Id),
		Options:      types.NotoLockOptions{SpendTxId: lockInfo.SpendTxId},
		Proof:        signatureBytes,
	}, notoParams)

	// Prepare again with V1 variant to exercise compatibility parameter shape
	tx.ContractInfo.ContractConfigJson = mustParseJSON(&types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		NotaryMode:   types.NotaryModeBasic.Enum(),
		Variant:      types.NotoVariantV1,
	})
	prepareResV1, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		ReadStates:        readStates,
		InfoStates:        infoStates,
		InputStates:       inputStates,
		OutputStates:      outputStates,
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

	// Decode the parameters for the V1 variant
	updateLockV1ABI := interfaceV1Build.ABI.Functions()["updateLock"]
	assert.JSONEq(t, mustParseJSON(updateLockV1ABI), prepareResV1.Transaction.FunctionAbiJson)
	paramsV1 := decodeFnParams[UpdateLockParams_V1](t, updateLockV1ABI, prepareResV1.Transaction.ParamsJson)
	require.Equal(t, fnParams.LockID, paramsV1.LockID)
	require.Equal(t, fnParams.SpendCommitment, paramsV1.Params.SpendHash)
	require.Equal(t, fnParams.CancelCommitment, paramsV1.Params.CancelHash)
	require.Equal(t, fnParams.Data.String(), paramsV1.Data.String())

	// Validate the encoded noto parameters passed in for the V1 variant
	notoParamsV1 := decodeSingleABITuple[types.NotoUpdateLockArgs_V1](t, types.NotoUpdateLockArgsABI_V1, paramsV1.UpdateArgs)
	require.Equal(t, &types.NotoUpdateLockArgs_V1{
		TxId:         "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		OldLockState: pldtypes.MustParseBytes32(inputLockInfo.Id),
		NewLockState: pldtypes.MustParseBytes32(*newLockInfoState.Id),
		Proof:        signatureBytes,
	}, notoParamsV1)

	// Prepare again to test hook invoke
	hookAddress := "0x515fba7fe1d8b9181be074bd4c7119544426837c"
	tx.ContractInfo.ContractConfigJson = mustParseJSON(&types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		NotaryMode:   types.NotaryModeHooks.Enum(),
		Variant:      types.NotoVariantV2,
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
		ReadStates:        readStates,
		InfoStates:        infoStates,
		InputStates:       inputStates,
		OutputStates:      outputStates,
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
	expectedFunctionABI := hooksBuild.ABI.Functions()["onPrepareBurnUnlock"]
	assert.JSONEq(t, mustParseJSON(expectedFunctionABI), prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)
	_, err = expectedFunctionABI.EncodeCallDataJSON([]byte(prepareRes.Transaction.ParamsJson))
	require.NoError(t, err)

	// Verify hook invoke params
	var hookParams UnlockHookParams
	err = json.Unmarshal([]byte(prepareRes.Transaction.ParamsJson), &hookParams)
	require.NoError(t, err)
	require.NotNil(t, hookParams.Sender)
	assert.Equal(t, senderKey.Address.String(), hookParams.Sender.String())
	assert.Equal(t, lockID, hookParams.LockID)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x1234"), hookParams.Data)

	// Verify recipients
	require.Len(t, hookParams.Recipients, 0)

	// Verify prepared transaction
	assert.Equal(t, pldtypes.MustEthAddress(contractAddress), hookParams.Prepared.ContractAddress)
	assert.NotEmpty(t, hookParams.Prepared.EncodedCall)

	manifestState.Id = confutil.P(pldtypes.RandBytes32().String()) // manifest is odd one out that  doesn't get ID allocated during assemble
	mt := newManifestTester(t, ctx, n, mockCallbacks, tx.TransactionId, assembleRes.AssembledTransaction)
	mt.withMissingStates( /* no missing states */ ).
		completeForIdentity(notaryAddress).
		completeForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(manifestState, unlockDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(unlockManifestState, unlockDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(unlockDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(prepareDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(newLockInfoState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
}

func TestPrepareBurnUnlockOnlyCreator(t *testing.T) {
	ctx := t.Context()
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks: mockCallbacks,
	}
	fn := types.NotoABI.Functions()["prepareBurnUnlock"]

	allowBurn := true
	config := &types.NotoParsedConfig{
		NotaryMode:   types.NotaryModeBasic.Enum(),
		NotaryLookup: "notary@node1",
		Variant:      types.NotoVariantV2,
		Options: types.NotoOptions{
			Basic: &types.NotoBasicOptions{
				AllowBurn: &allowBurn,
			},
		},
	}

	lockID := pldtypes.RandBytes32()
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
			ContractConfigJson: mustParseJSON(config),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: fmt.Sprintf(`{
			"lockId":"%s",
			"from":"other@node1",
			"amount":100,
			"unlockData":"0x9999",
			"data":"0x1234"
		}`, lockID),
	}

	_, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Only the lock creator can perform unlock")
}

func TestPrepareBurnUnlockCheckAllowedForFromLocalNodeNameFailed(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	mockCallbacks.MockLocalNodeName = func() (*prototk.LocalNodeNameResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	h := &prepareBurnUnlockHandler{lockCommon: lockCommon{noto: &Noto{Callbacks: mockCallbacks}}}
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node1"},
		DomainConfig: &types.NotoParsedConfig{NotaryMode: types.NotaryModeBasic.Enum()},
	}
	assertEndorseInternal(t, h.checkAllowedForFrom(t.Context(), tx, "sender"), "pop")
}

func TestPrepareBurnUnlockValidateParamsRevert(t *testing.T) {
	h := &prepareBurnUnlockHandler{}
	lockID := pldtypes.RandBytes32()
	for _, tc := range []struct {
		name, params, match string
		config              *types.NotoParsedConfig
	}{
		{"not supported in V0", `{}`, "PD200014", notoBasicConfigV0},
		{"malformed JSON", `{"lockId":`, "unexpected end of JSON input", notoBasicConfigV1},
		{"missing lockId", `{"from": "sender@node1", "amount": 1}`, "PD200007.*'lockId'", notoBasicConfigV1},
		{"missing from", fmt.Sprintf(`{"lockId": "%s", "amount": 1}`, lockID), "PD200007.*'from'", notoBasicConfigV1},
		{"zero amount", fmt.Sprintf(`{"lockId": "%s", "from": "sender@node1", "amount": 0}`, lockID), "PD200008.*'amount'", notoBasicConfigV1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.ValidateParams(t.Context(), tc.config, tc.params)
			assertRevert(t, err, tc.match)
		})
	}
}

func TestPrepareBurnUnlockCheckAllowedRevertsWhenBurnDisabled(t *testing.T) {
	allowBurn := false
	h := &prepareBurnUnlockHandler{}
	tx := &types.ParsedTransaction{
		DomainConfig: &types.NotoParsedConfig{
			NotaryMode: types.NotaryModeBasic.Enum(),
			Options:    types.NotoOptions{Basic: &types.NotoBasicOptions{AllowBurn: &allowBurn}},
		},
	}
	assertEndorseRevert(t, h.checkAllowed(t.Context(), tx), "PD200025")
}

func TestPrepareBurnUnlockCheckAllowedForFromUnparsableLocator(t *testing.T) {
	h := &prepareBurnUnlockHandler{lockCommon{noto: &Noto{Callbacks: newMockCallbacks()}}}
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node1"},
		DomainConfig: &types.NotoParsedConfig{NotaryMode: types.NotaryModeBasic.Enum()},
	}
	assertEndorseRevert(t, h.checkAllowedForFrom(t.Context(), tx, "sender@node1@node2"), "PD020006")
}

func TestPrepareBurnUnlockCheckAllowedForFromNotTheLockCreator(t *testing.T) {
	h := &prepareBurnUnlockHandler{lockCommon{noto: &Noto{Callbacks: newMockCallbacks()}}}
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "someoneelse@node1"},
		DomainConfig: &types.NotoParsedConfig{NotaryMode: types.NotaryModeBasic.Enum()},
	}
	assertEndorseRevert(t, h.checkAllowedForFrom(t.Context(), tx, "sender"), "PD200031")
}

func TestPrepareBurnUnlockEndorseRevertsWhenLockedCoinsDoNotMatchTheBurnAmount(t *testing.T) {
	n, tx, resolved, sender := notoForLockEndorse()
	tx.Params = &types.PrepareBurnUnlockParams{
		LockID: pldtypes.RandBytes32(),
		From:   "sender@node1",
		Amount: pldtypes.Int64ToInt256(100),
	}
	h := &prepareBurnUnlockHandler{lockCommon{noto: n}}

	_, err := h.Endorse(t.Context(), tx, &prototk.EndorseTransactionRequest{
		Transaction:       tx.Transaction,
		ResolvedVerifiers: resolved,
		Reads:             []*prototk.EndorsableState{newLockedCoinState(sender, 40)},
	})
	assertEndorseRevert(t, err, "PD200013.*prepareBurnUnlock")
}

func TestPrepareBurnUnlockAssembleRevertsWhenTheLockHoldsMoreThanTheBurnAmount(t *testing.T) {
	ctx, mockCallbacks, n := newNotoFullSchemaSet(t)
	sender := pldtypes.RandAddress()
	lockID := pldtypes.RandBytes32()

	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		switch req.SchemaId {
		case n.lockInfoSchemaV1.Id:
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{
				Id:       pldtypes.RandBytes32().String(),
				SchemaId: req.SchemaId,
				DataJson: mustParseJSON(&types.NotoLockInfo_V1{
					Salt: pldtypes.RandBytes32(), LockID: lockID, Owner: sender, Spender: sender,
				}),
			}}}, nil
		case n.lockedCoinSchema.Id:
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{
				Id:       pldtypes.RandBytes32().String(),
				SchemaId: req.SchemaId,
				DataJson: mustParseJSON(&types.NotoLockedCoin{
					Salt: pldtypes.RandBytes32(), LockID: lockID, Owner: sender, Amount: pldtypes.Int64ToInt256(150),
				}),
			}}}, nil
		}
		return &prototk.FindAvailableStatesResponse{}, nil
	}

	h := &prepareBurnUnlockHandler{lockCommon{noto: n}}
	tx := &types.ParsedTransaction{
		Transaction:     &prototk.TransactionSpecification{From: "sender@node1"},
		ContractAddress: ethtypes.MustNewAddress(pldtypes.RandAddress().String()),
		DomainConfig:    notoBasicConfigV1,
		Params: &types.PrepareBurnUnlockParams{
			LockID: lockID,
			From:   "sender@node1",
			Amount: pldtypes.Int64ToInt256(100),
		},
	}
	ethAddressVerifier := func(lookup string) *prototk.ResolvedVerifier {
		return &prototk.ResolvedVerifier{
			Lookup:       lookup,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     sender.String(),
		}
	}

	_, err := h.Assemble(ctx, tx, &prototk.AssembleTransactionRequest{
		Transaction:       tx.Transaction,
		StateQueryContext: "query-context",
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			ethAddressVerifier("notary@node1"),
			ethAddressVerifier("sender@node1"),
		},
	})
	assertAssembleRevert(t, err, "PD200013.*prepareBurnUnlock")
}
