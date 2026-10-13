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

func TestCreateTransferLock(t *testing.T) {
	ctx, mockCallbacks, n := newNotoFullSchemaSet(t)
	fn := types.NotoABI.Functions()["createTransferLock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiverAddress := "0x2000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	inputCoin := &types.NotoCoinState{
		ID: pldtypes.RandBytes32(),
		Data: types.NotoCoin{
			Owner:  (*pldtypes.EthAddress)(&senderKey.Address),
			Amount: pldtypes.Uint64ToUint256(150), // we'll have a remainder
		},
	}
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					Id:       inputCoin.ID.String(),
					SchemaId: hashName("coin"),
					DataJson: mustParseJSON(inputCoin.Data),
				},
			},
		}, nil
	}

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    contractAddress,
			ContractConfigJson: mustParseJSON(notoBasicConfigV1),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: `{
			"from": "sender@node1",
			"recipients": [{
				"to": "receiver@node2",
				"amount": 100
			}],
			"unlockData": "0x9999",
			"data": "0x1234"
		}`,
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
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 1)  // the input coin
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 3) // lock + locked-coin + remainder-coin
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 7) // outer-manifest + spend-manifest + cancel-manifest + unlock-data-info + data-info + spend-coin + cancel-coin

	inputCoinState := assembleRes.AssembledTransaction.InputStates[0]
	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	unlockManifestState := assembleRes.AssembledTransaction.InfoStates[1] // spend manifest
	cancelManifestState := assembleRes.AssembledTransaction.InfoStates[2]
	unlockDataState := assembleRes.AssembledTransaction.InfoStates[3]
	dataState := assembleRes.AssembledTransaction.InfoStates[4]
	spendCoinState := assembleRes.AssembledTransaction.InfoStates[5]
	cancelCoinState := assembleRes.AssembledTransaction.InfoStates[6]
	newLockInfoState := assembleRes.AssembledTransaction.OutputStates[0]
	lockedCoinState := assembleRes.AssembledTransaction.OutputStates[1]
	remainderCoinState := assembleRes.AssembledTransaction.OutputStates[2]

	assert.Equal(t, inputCoin.ID.String(), inputCoinState.Id)
	lockedCoin, err := n.unmarshalLockedCoin(lockedCoinState.StateDataJson)
	require.NoError(t, err)
	remainderCoin, err := n.unmarshalLockedCoin(remainderCoinState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), remainderCoin.Owner.String())
	assert.Equal(t, "50", remainderCoin.Amount.Int().String())
	spendCoin, err := n.unmarshalCoin(spendCoinState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, receiverAddress, spendCoin.Owner.String())
	assert.Equal(t, "100", spendCoin.Amount.Int().String())
	cancelCoin, err := n.unmarshalCoin(cancelCoinState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), cancelCoin.Owner.String())
	assert.Equal(t, "100", cancelCoin.Amount.Int().String())
	unlockDataInfo, err := n.unmarshalInfo(unlockDataState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x9999", unlockDataInfo.Data.String())
	dataInfo, err := n.unmarshalInfo(dataState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", dataInfo.Data.String())

	lockInfo, err := n.unmarshalLockV1(newLockInfoState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), lockInfo.Owner.String())
	lockID, err := n.computeLockId(ctx, pldtypes.MustEthAddress(contractAddress), pldtypes.MustEthAddress(notaryAddress), tx.TransactionId)
	require.NoError(t, err)

	assert.Equal(t, lockID, lockInfo.LockID)
	require.Len(t, lockInfo.SpendOutputs, 1)
	require.Len(t, lockInfo.CancelOutputs, 1)
	require.NotEmpty(t, lockInfo.SpendData)
	require.NotEmpty(t, lockInfo.CancelData)
	require.NotEqual(t, lockInfo.SpendData, lockInfo.CancelData) // spend and cancel use distinct manifests

	encodedUnlock, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{lockedCoin}, []*types.NotoLockedCoin{}, []*types.NotoCoin{spendCoin})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedUnlock)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	inputStates := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("coin"),
			Id:            inputCoin.ID.String(),
			StateDataJson: mustParseJSON(inputCoin.Data),
		},
	}
	outputStates := []*prototk.EndorsableState{
		{
			SchemaId:      n.lockInfoSchemaV1.Id,
			Id:            *newLockInfoState.Id,
			StateDataJson: newLockInfoState.StateDataJson,
		},
		{
			SchemaId:      n.lockedCoinSchema.Id,
			Id:            *lockedCoinState.Id,
			StateDataJson: lockedCoinState.StateDataJson,
		},
		{
			SchemaId:      n.coinSchema.Id,
			Id:            *remainderCoinState.Id,
			StateDataJson: remainderCoinState.StateDataJson,
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      n.dataSchemaV2.Id,
			Id:            *dataState.Id,
			StateDataJson: dataState.StateDataJson,
		},
		{
			SchemaId:      n.coinSchema.Id,
			Id:            *spendCoinState.Id,
			StateDataJson: spendCoinState.StateDataJson,
		},
		{
			SchemaId:      n.coinSchema.Id,
			Id:            *cancelCoinState.Id,
			StateDataJson: cancelCoinState.StateDataJson,
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
	createLockABI := interfaceV2Build.ABI.Functions()["createLock"]
	expectedFunction := mustParseJSON(createLockABI)
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Decode the function parameters
	fnParams := decodeFnParams[CreateLockParams](t, createLockABI, prepareRes.Transaction.ParamsJson)
	data, err := n.decodeTransactionDataV1(ctx, fnParams.Data) // this is the transaction data for the prepare (not the prepared transaction)
	require.NoError(t, err)
	require.Equal(t, &types.NotoTransactionData_V1{
		InfoStates: []pldtypes.Bytes32{
			pldtypes.MustParseBytes32(*dataState.Id),
			pldtypes.MustParseBytes32(*spendCoinState.Id),
			pldtypes.MustParseBytes32(*cancelCoinState.Id),
		},
	}, data)

	// Decode the options we store into the lockInfo
	unlockTxData, err := n.encodeTransactionDataV1(ctx, newStateToEndorsableState([]*prototk.NewState{unlockManifestState, unlockDataState}))
	require.NoError(t, err)
	cancelUnlockTxData, err := n.encodeTransactionDataV1(ctx, newStateToEndorsableState([]*prototk.NewState{cancelManifestState, unlockDataState}))
	require.NoError(t, err)
	createLockArgs := decodeSingleABITuple[types.NotoCreateLockArgs](t, types.NotoCreateLockArgsABI, fnParams.CreateArgs)
	expectedSpendHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, lockInfo.SpendTxId.HexString(), n.endorsableStateIDs(ctx, nil, outputStates[1:2], false), n.endorsableStateIDs(ctx, nil, infoStates[1:2], false), unlockTxData)
	require.NoError(t, err)
	require.Equal(t, expectedSpendHash, fnParams.SpendCommitment)
	expectedCancelHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, lockInfo.SpendTxId.HexString(), n.endorsableStateIDs(ctx, nil, outputStates[1:2], false), n.endorsableStateIDs(ctx, nil, infoStates[2:3], false), cancelUnlockTxData)
	require.NoError(t, err)
	require.Equal(t, expectedCancelHash, fnParams.CancelCommitment)

	expectedInputs := n.endorsableStateIDs(ctx, nil, inputStates, false)
	require.NotNil(t, expectedInputs)

	// Validate the encoded noto parameters passed in
	require.Equal(t, &types.NotoCreateLockArgs{
		TxId:         "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Inputs:       expectedInputs,
		Outputs:      []string{*remainderCoinState.Id},
		Contents:     []string{*lockedCoinState.Id},
		NewLockState: pldtypes.MustParseBytes32(*newLockInfoState.Id),
		Options:      &types.NotoLockOptions{SpendTxId: lockInfo.SpendTxId},
		Proof:        signatureBytes,
	}, createLockArgs)

	// Prepare again with V1 variant to check parameter shape
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
	createLockV1ABI := interfaceV1Build.ABI.Functions()["createLock"]
	assert.JSONEq(t, mustParseJSON(createLockV1ABI), prepareResV1.Transaction.FunctionAbiJson)
	paramsV1 := decodeFnParams[CreateLockParams_V1](t, createLockV1ABI, prepareResV1.Transaction.ParamsJson)
	require.Equal(t, fnParams.SpendCommitment, paramsV1.Params.SpendHash)
	require.Equal(t, fnParams.CancelCommitment, paramsV1.Params.CancelHash)
	require.Equal(t, fnParams.Data.String(), paramsV1.Data.String())

	// Validate the encoded noto parameters passed in for the V1 variant
	createLockArgsV1 := decodeSingleABITuple[types.NotoCreateLockArgs_V1](t, types.NotoCreateLockArgsABI_V1, paramsV1.CreateArgs)
	require.Equal(t, &types.NotoCreateLockArgs_V1{
		TxId:         "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Inputs:       []string{inputCoinState.Id},
		Outputs:      []string{*remainderCoinState.Id},
		Contents:     []string{*lockedCoinState.Id},
		NewLockState: pldtypes.MustParseBytes32(*newLockInfoState.Id),
		Proof:        signatureBytes,
	}, createLockArgsV1)

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
	expectedFunctionABI := hooksBuild.ABI.Functions()["onCreateTransferLock"]
	assert.JSONEq(t, mustParseJSON(expectedFunctionABI), prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)
	_, err = expectedFunctionABI.EncodeCallDataJSON([]byte(prepareRes.Transaction.ParamsJson))
	require.NoError(t, err)

	// Verify hook invoke params
	var hookParams CreateTransferLockHookParams
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
	mt.withMissingNewStates(dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(unlockDataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(newLockInfoState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		completeForIdentity(receiverAddress) // receivers don't get the lock
	mt.withMissingNewStates(spendCoinState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)

	receipt := testGetDomainReceipt(t, n, &prototk.BuildReceiptRequest{
		TransactionId:     tx.TransactionId,
		UnavailableStates: false,
		InputStates:       inputStates,
		OutputStates:      outputStates,
		InfoStates:        infoStates,
	})
	require.Equal(t, lockInfo.LockID, receipt.LockInfo.LockID)
	require.Equal(t, "spendLock", receipt.LockInfo.UnlockFunction)
	require.NotNil(t, receipt.LockInfo.UnlockParams)
}

func TestCreateTransferLockInsufficientFunds(t *testing.T) {
	ctx, mockCallbacks, n := newNotoFullSchemaSet(t)
	fn := types.NotoABI.Functions()["createTransferLock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiverAddress := "0x2000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{},
		}, nil
	}

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    contractAddress,
			ContractConfigJson: mustParseJSON(notoBasicConfigV1),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: `{
			"from": "sender@node1",
			"recipients": [{
				"to": "receiver@node2",
				"amount": 100
			}],
			"data": "0x1234"
		}`,
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
	require.Equal(t, prototk.AssembleTransactionResponse_REVERT, assembleRes.AssemblyResult)
}

func TestCreateTransferLockValidateParamsRevert(t *testing.T) {
	h := &createTransferLockHandler{}
	for _, tc := range []struct {
		name, params, match string
		config              *types.NotoParsedConfig
	}{
		{"not supported in V0", `{}`, "PD200014", notoBasicConfigV0},
		{"malformed JSON", `{"from":`, "unexpected end of JSON input", notoBasicConfigV1},
		{"missing from", `{"recipients": [{"to": "receiver@node2", "amount": 1}]}`, "PD200007.*'from'", notoBasicConfigV1},
		{"missing recipients", `{"from": "sender@node1"}`, "PD200007.*'recipients'", notoBasicConfigV1},
		{"zero recipient amount", `{"from": "sender@node1", "recipients": [{"to": "receiver@node2", "amount": 0}]}`, "PD200008.*'recipient amount'", notoBasicConfigV1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.ValidateParams(t.Context(), tc.config, tc.params)
			assertRevert(t, err, tc.match)
		})
	}
}

func TestCreateTransferLockEndorseRevertsWhenInputsDoNotCoverOutputs(t *testing.T) {
	n, tx, resolved, sender := notoForLockEndorse()
	tx.Params = &types.CreateTransferLockParams{From: "sender@node1"}
	h := &createTransferLockHandler{lockCommon{noto: n}}
	_, err := h.Endorse(t.Context(), tx, &prototk.EndorseTransactionRequest{
		Transaction:       tx.Transaction,
		ResolvedVerifiers: resolved,
		Inputs:            []*prototk.EndorsableState{newCoinState(sender, 100)},
		Outputs:           []*prototk.EndorsableState{newLockState(sender, nil, nil), newLockedCoinState(sender, 50)},
	})
	assertEndorseRevert(t, err, "PD200013.*totalOutputs")
}

func TestCreateTransferLockEndorseRevertsWhenSpendDoesNotMatchWhatWasLocked(t *testing.T) {
	n, tx, resolved, sender := notoForLockEndorse()
	tx.Params = &types.CreateTransferLockParams{From: "sender@node1"}
	h := &createTransferLockHandler{lockCommon{noto: n}}
	spend := newCoinState(sender, 40)
	_, err := h.Endorse(t.Context(), tx, &prototk.EndorseTransactionRequest{
		Transaction:       tx.Transaction,
		ResolvedVerifiers: resolved,
		Inputs:            []*prototk.EndorsableState{newCoinState(sender, 100)},
		Outputs:           []*prototk.EndorsableState{newLockState(sender, stateIDs(spend), nil), newLockedCoinState(sender, 100)},
		Info:              []*prototk.EndorsableState{spend},
	})
	assertEndorseRevert(t, err, "PD200013.*spendOutputs")
}

func TestCreateTransferLockEndorseRevertsWhenCancelDoesNotReturnWhatWasLocked(t *testing.T) {
	n, tx, resolved, sender := notoForLockEndorse()
	tx.Params = &types.CreateTransferLockParams{From: "sender@node1"}
	h := &createTransferLockHandler{lockCommon{noto: n}}
	spend := newCoinState(sender, 100)
	cancel := newCoinState(sender, 40)
	_, err := h.Endorse(t.Context(), tx, &prototk.EndorseTransactionRequest{
		Transaction:       tx.Transaction,
		ResolvedVerifiers: resolved,
		Inputs:            []*prototk.EndorsableState{newCoinState(sender, 100)},
		Outputs:           []*prototk.EndorsableState{newLockState(sender, stateIDs(spend), stateIDs(cancel)), newLockedCoinState(sender, 100)},
		Info:              []*prototk.EndorsableState{spend, cancel},
	})
	assertEndorseRevert(t, err, "PD200013.*cancelOutputs")
}
