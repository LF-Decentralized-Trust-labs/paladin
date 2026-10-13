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

func TestPrepareMintUnlock(t *testing.T) {
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
	fn := types.NotoABI.Functions()["prepareMintUnlock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiverAddress := "0x2000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	lockID := pldtypes.RandBytes32()
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
			}),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: fmt.Sprintf(`{
		    "lockId": "%s",
			"from": "sender@node1",
			"recipients": [{
				"to": "receiver@node2",
				"amount": 100
			}],
			"unlockData": "0x9999",
			"data": "0x1234"
		}`, lockID),
	}

	initRes, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.NoError(t, err)
	require.Len(t, initRes.RequiredVerifiers, 3)
	assert.Equal(t, "notary@node1", initRes.RequiredVerifiers[0].Lookup)
	assert.Equal(t, "sender@node1", initRes.RequiredVerifiers[1].Lookup)
	assert.Equal(t, "receiver@node2", initRes.RequiredVerifiers[2].Lookup)

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
			Lookup:       "receiver@node2",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     receiverAddress,
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
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 6) // outer-manifest + spend-manifest + cancel-manifest + unlock-data-info + prepare-data-info + output-coin

	assert.Equal(t, inputLockInfo.Id, assembleRes.AssembledTransaction.InputStates[0].Id)
	assert.Equal(t, hashName("lockInfo_v1"), assembleRes.AssembledTransaction.OutputStates[0].SchemaId)

	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	unlockManifestState := assembleRes.AssembledTransaction.InfoStates[1] // spend manifest
	cancelManifestState := assembleRes.AssembledTransaction.InfoStates[2]
	unlockDataState := assembleRes.AssembledTransaction.InfoStates[3]
	prepareDataState := assembleRes.AssembledTransaction.InfoStates[4]
	spendCoinState := assembleRes.AssembledTransaction.InfoStates[5]
	newLockInfoState := assembleRes.AssembledTransaction.OutputStates[0]

	spendCoin, err := n.unmarshalCoin(spendCoinState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, receiverAddress, spendCoin.Owner.String())
	assert.Equal(t, "100", spendCoin.Amount.Int().String())
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
	require.Len(t, lockInfo.SpendOutputs, 1)
	require.Len(t, lockInfo.CancelOutputs, 0)
	require.NotEmpty(t, lockInfo.SpendData)
	require.NotEmpty(t, lockInfo.CancelData)
	require.NotEqual(t, lockInfo.SpendData, lockInfo.CancelData) // spend and cancel use distinct manifests

	encodedUnlock, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{}, []*types.NotoLockedCoin{}, []*types.NotoCoin{spendCoin})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedUnlock)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      n.dataSchemaV2.Id,
			Id:            *unlockDataState.Id,
			StateDataJson: unlockDataState.StateDataJson,
		},
		{
			SchemaId:      n.coinSchema.Id,
			Id:            *spendCoinState.Id,
			StateDataJson: spendCoinState.StateDataJson,
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
		Reads:             nil,
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
		ReadStates:        nil,
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
			pldtypes.MustParseBytes32(*spendCoinState.Id),
		},
	}, data)

	// Decode the options we store into the lockInfo
	unlockTxData, err := n.encodeTransactionDataV1(ctx, newStateToEndorsableState([]*prototk.NewState{unlockManifestState, unlockDataState}))
	require.NoError(t, err)
	cancelUnlockTxData, err := n.encodeTransactionDataV1(ctx, newStateToEndorsableState([]*prototk.NewState{cancelManifestState, unlockDataState}))
	require.NoError(t, err)
	notoParams := decodeSingleABITuple[types.NotoUpdateLockArgs](t, types.NotoUpdateLockArgsABI, fnParams.UpdateArgs)
	expectedSpendHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, lockInfo.SpendTxId.HexString(), []string{}, n.endorsableStateIDs(ctx, nil, infoStates[1:2], false), unlockTxData)
	require.NoError(t, err)
	require.Equal(t, expectedSpendHash, fnParams.SpendCommitment)
	expectedCancelHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, lockInfo.SpendTxId.HexString(), []string{}, []string{}, cancelUnlockTxData)
	require.NoError(t, err)
	require.Equal(t, expectedCancelHash, fnParams.CancelCommitment)

	// Validate the encoded noto parameters passed in
	require.Equal(t, &types.NotoUpdateLockArgs{
		TxId:         "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Contents:     []string{},
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
		ReadStates:        nil,
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
		ReadStates:        nil,
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
	expectedFunctionABI := hooksBuild.ABI.Functions()["onPrepareMintUnlock"]
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
	require.Len(t, hookParams.Recipients, 1)
	require.NotNil(t, hookParams.Recipients[0].To)
	assert.Equal(t, pldtypes.MustEthAddress("0x2000000000000000000000000000000000000000").String(), hookParams.Recipients[0].To.String())
	require.NotNil(t, hookParams.Recipients[0].Amount)
	assert.Equal(t, pldtypes.Uint64ToUint256(100).String(), hookParams.Recipients[0].Amount.String())

	// Verify prepared transaction
	assert.Equal(t, pldtypes.MustEthAddress(contractAddress), hookParams.Prepared.ContractAddress)
	assert.NotEmpty(t, hookParams.Prepared.EncodedCall)

	manifestState.Id = confutil.P(pldtypes.RandBytes32().String()) // manifest is odd one out that  doesn't get ID allocated during assemble
	mt := newManifestTester(t, ctx, n, mockCallbacks, tx.TransactionId, assembleRes.AssembledTransaction)
	mt.withMissingStates( /* no missing states */ ).
		completeForIdentity(notaryAddress).
		completeForIdentity(senderKey.Address.String()).
		completeForIdentity(receiverAddress)
	mt.withMissingNewStates(manifestState, unlockDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(unlockManifestState, unlockDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(unlockDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(prepareDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(newLockInfoState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		completeForIdentity(receiverAddress) // receivers don't get the lock
	mt.withMissingNewStates(spendCoinState).
		incompleteForIdentity(notaryAddress).
		completeForIdentity(senderKey.Address.String()). // sender isn't a participant of the minted coin
		incompleteForIdentity(receiverAddress)
}

func TestPrepareMintUnlockBasicModeAllowLockDisabled(t *testing.T) {
	ctx := t.Context()
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks: mockCallbacks,
	}
	fn := types.NotoABI.Functions()["prepareMintUnlock"]

	restrictMint := true
	allowLock := false
	config := &types.NotoParsedConfig{
		NotaryMode:   types.NotaryModeBasic.Enum(),
		NotaryLookup: "notary@node1",
		Variant:      types.NotoVariantV2,
		Options: types.NotoOptions{
			Basic: &types.NotoBasicOptions{
				RestrictMint: &restrictMint,
				AllowLock:    &allowLock,
			},
		},
	}

	lockID := pldtypes.RandBytes32()
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "notary@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
			ContractConfigJson: mustParseJSON(config),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: fmt.Sprintf(`{
			"lockId":"%s",
			"recipients":[{"to":"receiver@node1","amount":100}],
			"unlockData":"0x9999",
			"data":"0x1234"
		}`, lockID),
	}

	_, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Lock is not enabled")
}

func TestPrepareMintUnlockValidateParamsRevert(t *testing.T) {
	h := &prepareMintUnlockHandler{}
	for _, tc := range []struct {
		name, params, match string
		config              *types.NotoParsedConfig
	}{
		{"not supported in V0", `{}`, "PD200014", notoBasicConfigV0},
		{"malformed JSON", `{"recipients":`, "unexpected end of JSON input", notoBasicConfigV1},
		{"missing recipients", `{}`, "PD200007.*'recipients'", notoBasicConfigV1},
		{"zero recipient amount", `{"recipients": [{"to": "receiver@node2", "amount": 0}]}`, "PD200008.*'recipient amount'", notoBasicConfigV1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.ValidateParams(t.Context(), tc.config, tc.params)
			assertRevert(t, err, tc.match)
		})
	}
}

func TestPrepareMintUnlockCheckAllowedRevert(t *testing.T) {
	allowLock := false
	h := &prepareMintUnlockHandler{}
	for _, tc := range []struct {
		name, from, match string
		config            *types.NotoParsedConfig
	}{
		{"minting restricted to the notary", "sender@node1", "PD200009", notoBasicConfigV1},
		{"lock not enabled", "notary@node1", "PD200030", &types.NotoParsedConfig{
			NotaryMode:   types.NotaryModeBasic.Enum(),
			NotaryLookup: "notary@node1",
			Options:      types.NotoOptions{Basic: &types.NotoBasicOptions{RestrictMint: &pTrue, AllowLock: &allowLock}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &types.ParsedTransaction{DomainConfig: tc.config}
			assertEndorseRevert(t, h.checkAllowed(t.Context(), tx, tc.from), tc.match)
		})
	}
}
