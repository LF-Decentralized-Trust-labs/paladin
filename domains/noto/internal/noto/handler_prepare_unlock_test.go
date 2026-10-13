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
	"github.com/hyperledger-firefly/signer/pkg/abi"
	"github.com/hyperledger-firefly/signer/pkg/ethtypes"
	"github.com/hyperledger-firefly/signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareUnlock(t *testing.T) {
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
	fn := types.NotoABI.Functions()["prepareUnlock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiverAddress := "0x2000000000000000000000000000000000000000"
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
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 7) // prepare-manifest + spend-manifest + cancel-manifest + unlock-data-info + prepare-data-info + output-coin + cancel-coin

	assert.Equal(t, inputLockInfo.Id, assembleRes.AssembledTransaction.InputStates[0].Id)
	assert.Equal(t, hashName("lockInfo_v1"), assembleRes.AssembledTransaction.OutputStates[0].SchemaId)

	inputCoinState := assembleRes.AssembledTransaction.ReadStates[0]
	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	unlockManifestState := assembleRes.AssembledTransaction.InfoStates[1]
	cancelManifestState := assembleRes.AssembledTransaction.InfoStates[2]
	unlockDataState := assembleRes.AssembledTransaction.InfoStates[3]
	prepareDataState := assembleRes.AssembledTransaction.InfoStates[4]
	spendCoinState := assembleRes.AssembledTransaction.InfoStates[5]
	cancelCoinState := assembleRes.AssembledTransaction.InfoStates[6]
	newLockInfoState := assembleRes.AssembledTransaction.OutputStates[0]

	assert.Equal(t, inputCoin.ID.String(), inputCoinState.Id)
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
	require.Len(t, lockInfo.CancelOutputs, 1)
	require.NotEmpty(t, lockInfo.SpendData)
	require.NotEmpty(t, lockInfo.CancelData)
	require.NotEqual(t, lockInfo.SpendData, lockInfo.CancelData) // spend and cancel use distinct manifests

	encodedUnlock, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{&inputCoin.Data}, []*types.NotoLockedCoin{}, []*types.NotoCoin{spendCoin})
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
			Id:            *spendCoinState.Id,
			StateDataJson: spendCoinState.StateDataJson,
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
			pldtypes.MustParseBytes32(*spendCoinState.Id),
			pldtypes.MustParseBytes32(*cancelCoinState.Id),
		},
	}, data)

	// Decode the options we store into the lockInfo
	unlockTxData, err := n.encodeTransactionDataV1(ctx, newStateToEndorsableState([]*prototk.NewState{unlockManifestState, unlockDataState}))
	require.NoError(t, err)
	cancelTxData, err := n.encodeTransactionDataV1(ctx, newStateToEndorsableState([]*prototk.NewState{cancelManifestState, unlockDataState}))
	require.NoError(t, err)
	notoParams := decodeSingleABITuple[types.NotoUpdateLockArgs](t, types.NotoUpdateLockArgsABI, fnParams.UpdateArgs)
	expectedSpendHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, lockInfo.SpendTxId.HexString(), n.endorsableStateIDs(ctx, nil, readStates, false), n.endorsableStateIDs(ctx, nil, infoStates[1:2], false), unlockTxData)
	require.NoError(t, err)
	require.Equal(t, expectedSpendHash, fnParams.SpendCommitment)
	expectedCancelHash, err := n.unlockHashFromIDs_V1(ctx, ethtypes.MustNewAddress(contractAddress), lockID, lockInfo.SpendTxId.HexString(), n.endorsableStateIDs(ctx, nil, readStates, false), n.endorsableStateIDs(ctx, nil, infoStates[2:3], false), cancelTxData)
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
	expectedFunctionABI := hooksBuild.ABI.Functions()["onPrepareUnlock"]
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

func TestPrepareUnlock_V0(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       testSchema("coin"),
		lockedCoinSchema: testSchema("lockedCoin"),
		lockInfoSchemaV0: testSchema("lockInfo"),
		lockInfoSchemaV1: testSchema("UNUSED"), // needs to be there for coin filtering
		dataSchemaV0:     testSchema("data"),
		dataSchemaV1:     testSchema("UNUSED"), // needs to be there for coin filtering
		dataSchemaV2:     testSchema("data_v2"),
	}
	ctx := t.Context()
	fn := types.NotoABI.Functions()["prepareUnlock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiverAddress := "0x2000000000000000000000000000000000000000"
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
	mockCallbacks.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					Id:       inputCoin.ID.String(),
					SchemaId: hashName("lockedCoin"),
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
			ContractAddress: contractAddress,
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				NotaryLookup: "notary@node1",
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
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 3)
	assert.Equal(t, inputCoin.ID.String(), assembleRes.AssembledTransaction.ReadStates[0].Id)
	outputCoin, err := n.unmarshalCoin(assembleRes.AssembledTransaction.InfoStates[2].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, receiverAddress, outputCoin.Owner.String())
	assert.Equal(t, "100", outputCoin.Amount.Int().String())
	outputInfo, err := n.unmarshalInfo(assembleRes.AssembledTransaction.InfoStates[0].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())
	lockInfo, err := n.unmarshalLockV0(assembleRes.AssembledTransaction.InfoStates[1].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), lockInfo.Owner.String())
	assert.Equal(t, lockID, lockInfo.LockID)

	encodedUnlock, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{&inputCoin.Data}, []*types.NotoLockedCoin{}, []*types.NotoCoin{outputCoin})
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
			SchemaId:      hashName("data"),
			Id:            "0x4cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[0].StateDataJson,
		},
		{
			SchemaId:      hashName("lockInfo"),
			Id:            "0x69101A0740EC8096B83653600FA7553D676FC92BCC6E203C3572D2CAC4F1DB2F",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[1].StateDataJson,
		},
		{
			SchemaId:      hashName("coin"),
			Id:            "0x26b394af655bdc794a6d7cd7f8004eec20bffb374e4ddd24cdaefe554878d945",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[2].StateDataJson,
		},
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		Reads:             readStates,
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

	unlockHash, err := unlockHashFromStates_V0(ctx, n, ethtypes.MustNewAddress(contractAddress), readStates, nil, n.filterSchema(infoStates, []string{hashName("coin")}), pldtypes.MustParseHexBytes("0x1234"))
	require.NoError(t, err)

	// Prepare once to test base invoke
	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		ReadStates:        readStates,
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
	expectedFunction := mustParseJSON(interfaceV0Build.ABI.Functions()["prepareUnlock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)
	assert.JSONEq(t, fmt.Sprintf(`{
		"lockedInputs": ["%s"],
		"unlockHash": "%s",
		"signature": "%s",
		"data": "0x00010000015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000034cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d69101a0740ec8096b83653600fa7553d676fc92bcc6e203c3572d2cac4f1db2f26b394af655bdc794a6d7cd7f8004eec20bffb374e4ddd24cdaefe554878d945"
	}`, inputCoin.ID, unlockHash, signatureBytes), prepareRes.Transaction.ParamsJson)

	var invokeFn abi.Entry
	err = json.Unmarshal([]byte(prepareRes.Transaction.FunctionAbiJson), &invokeFn)
	require.NoError(t, err)
	encodedCall, err := invokeFn.EncodeCallDataJSONCtx(ctx, []byte(prepareRes.Transaction.ParamsJson))
	require.NoError(t, err)

	// Prepare again to test hook invoke
	hookAddress := "0x515fba7fe1d8b9181be074bd4c7119544426837c"
	tx.ContractInfo.ContractConfigJson = mustParseJSON(&types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
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
		ReadStates:        readStates,
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
	expectedFunction = mustParseJSON(hooksBuild.ABI.Functions()["onPrepareUnlock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)
	assert.JSONEq(t, fmt.Sprintf(`{
		"sender": "%s",
		"lockId": "%s",
		"recipients": [{
			"to": "0x2000000000000000000000000000000000000000",
			"amount": "0x64"
		}],
		"data": "0x1234",
		"prepared": {
			"contractAddress": "%s",
			"encodedCall": "%s"
		}
	}`, senderKey.Address, lockID, contractAddress, pldtypes.HexBytes(encodedCall)), prepareRes.Transaction.ParamsJson)

	receipt := testGetDomainReceipt(t, n, &prototk.BuildReceiptRequest{
		TransactionId:     tx.TransactionId,
		UnavailableStates: false,
		ReadStates:        readStates,
		InfoStates:        infoStates,
	})
	require.Equal(t, lockInfo.LockID, receipt.LockInfo.LockID)
	require.Equal(t, "unlock", receipt.LockInfo.UnlockFunction)
	require.NotNil(t, receipt.LockInfo.UnlockParams)
}

func unlockHashFromStates_V0(ctx context.Context, n *Noto, contract *ethtypes.Address0xHex, lockedInputs, lockedOutputs, outputs []*prototk.EndorsableState, data pldtypes.HexBytes) (ethtypes.HexBytes0xPrefix, error) {
	return n.unlockHashFromIDs_V0(ctx, contract, n.endorsableStateIDs(ctx, nil, lockedInputs, false), n.endorsableStateIDs(ctx, nil, lockedOutputs, false), n.endorsableStateIDs(ctx, nil, outputs, false), data)
}

func TestPrepareUnlockValidateParamsRevert(t *testing.T) {
	h := &prepareUnlockHandler{}
	_, err := h.ValidateParams(t.Context(), notoBasicConfigV1, `{"lockId":`)
	assertRevert(t, err, "unexpected end of JSON input")
}
