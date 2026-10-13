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

func TestLock(t *testing.T) {
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
	fn := types.NotoABI.Functions()["lock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	inputCoin := &types.NotoCoinState{
		ID: pldtypes.RandBytes32(),
		Data: types.NotoCoin{
			Owner:  (*pldtypes.EthAddress)(&senderKey.Address),
			Amount: pldtypes.Uint64ToUint256(100),
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
			"amount": 100,
			"data": "0x1234"
		}`,
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
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 2) // coin + lock
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 2)
	assert.Equal(t, inputCoin.ID.String(), assembleRes.AssembledTransaction.InputStates[0].Id)

	dataState := assembleRes.AssembledTransaction.InfoStates[1]
	outCoin1State := assembleRes.AssembledTransaction.OutputStates[0]

	outputCoin, err := n.unmarshalLockedCoin(outCoin1State.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), outputCoin.Owner.String())
	assert.Equal(t, "100", outputCoin.Amount.Int().String())
	assert.Equal(t, []string{"notary@node1", "sender@node1"}, assembleRes.AssembledTransaction.OutputStates[0].DistributionList)

	outputInfo, err := n.unmarshalInfo(assembleRes.AssembledTransaction.InfoStates[1].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())
	assert.Equal(t, []string{"notary@node1", "sender@node1"}, assembleRes.AssembledTransaction.InfoStates[0].DistributionList)

	lockState := assembleRes.AssembledTransaction.OutputStates[1]
	lockInfo, err := n.unmarshalLockV1(lockState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), lockInfo.Owner.String())
	assert.Equal(t, lockInfo.LockID, outputCoin.LockID)
	assert.Equal(t, []string{"notary@node1", "sender@node1"}, assembleRes.AssembledTransaction.InfoStates[1].DistributionList)

	encodedLock, err := n.encodeLock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoCoin{&inputCoin.Data}, []*types.NotoCoin{}, []*types.NotoLockedCoin{outputCoin})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedLock)
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
			SchemaId:      assembleRes.AssembledTransaction.OutputStates[0].SchemaId,
			Id:            *assembleRes.AssembledTransaction.OutputStates[0].Id,
			StateDataJson: assembleRes.AssembledTransaction.OutputStates[0].StateDataJson,
		},
		{
			SchemaId:      assembleRes.AssembledTransaction.OutputStates[1].SchemaId,
			Id:            *assembleRes.AssembledTransaction.OutputStates[1].Id,
			StateDataJson: assembleRes.AssembledTransaction.OutputStates[1].StateDataJson,
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      assembleRes.AssembledTransaction.InfoStates[1].SchemaId,
			Id:            *assembleRes.AssembledTransaction.InfoStates[1].Id,
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[1].StateDataJson,
		},
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		Inputs:            inputStates,
		Outputs:           outputStates,
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

	// Prepare once to test base invoke
	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		InputStates:       inputStates,
		OutputStates:      outputStates,
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

	// Decode the parameters
	createLockABI := interfaceV2Build.ABI.Functions()["createLock"]
	expectedFunction := mustParseJSON(createLockABI)
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Validate the parameters
	params := decodeFnParams[CreateLockParams](t, createLockABI, prepareRes.Transaction.ParamsJson)
	require.Zero(t, params.SpendCommitment)
	require.Zero(t, params.CancelCommitment)
	notoParams := decodeSingleABITuple[types.NotoCreateLockArgs](t, types.NotoCreateLockArgsABI, params.CreateArgs)
	require.Equal(t, &types.NotoLockOptions{}, notoParams.Options)
	require.Equal(t, &types.NotoCreateLockArgs{
		TxId:         "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Inputs:       []string{inputCoin.ID.String()},
		Outputs:      []string{},
		Contents:     []string{*outCoin1State.Id},
		NewLockState: pldtypes.MustParseBytes32(*lockState.Id),
		Options:      &types.NotoLockOptions{},
		Proof:        signatureBytes,
	}, notoParams)
	data, err := n.decodeTransactionDataV1(ctx, params.Data)
	require.NoError(t, err)
	require.Equal(t, &types.NotoTransactionData_V1{
		InfoStates: []pldtypes.Bytes32{
			pldtypes.MustParseBytes32(*dataState.Id),
		},
	}, data)

	// Prepare again with V1 variant to exercise compatibility parameter shape
	tx.ContractInfo.ContractConfigJson = mustParseJSON(&types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		NotaryMode:   types.NotaryModeBasic.Enum(),
		Variant:      types.NotoVariantV1,
	})
	prepareResV1, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		InputStates:       inputStates,
		OutputStates:      outputStates,
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

	// Decode the parameters for the V1 variant
	createLockV1ABI := interfaceV1Build.ABI.Functions()["createLock"]
	assert.JSONEq(t, mustParseJSON(createLockV1ABI), prepareResV1.Transaction.FunctionAbiJson)
	paramsV1 := decodeFnParams[CreateLockParams_V1](t, createLockV1ABI, prepareResV1.Transaction.ParamsJson)
	require.Equal(t, params.SpendCommitment, paramsV1.Params.SpendHash)
	require.Equal(t, params.CancelCommitment, paramsV1.Params.CancelHash)
	require.Equal(t, params.Data.String(), paramsV1.Data.String())

	// Validate the encoded noto parameters passed in for the V1 variant
	notoParamsV1 := decodeSingleABITuple[types.NotoCreateLockArgs_V1](t, types.NotoCreateLockArgsABI_V1, paramsV1.CreateArgs)
	require.Equal(t, &types.NotoCreateLockArgs_V1{
		TxId:         "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Inputs:       []string{inputCoin.ID.String()},
		Outputs:      []string{},
		Contents:     []string{*outCoin1State.Id},
		NewLockState: pldtypes.MustParseBytes32(*lockState.Id),
		Proof:        signatureBytes,
	}, notoParamsV1)

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
		InputStates:       inputStates,
		OutputStates:      outputStates,
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
	expectedFunctionABI := hooksBuild.ABI.Functions()["onLock"]
	assert.JSONEq(t, mustParseJSON(expectedFunctionABI), prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)
	_, err = expectedFunctionABI.EncodeCallDataJSON([]byte(prepareRes.Transaction.ParamsJson))
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{
		"sender": "%s",
		"lockId": "%s",
		"from": "%s",
		"amount": "0x64",
		"data": "0x1234",
		"prepared": {
			"contractAddress": "%s",
			"encodedCall": "%s"
		}
	}`, senderKey.Address, lockInfo.LockID, senderKey.Address, contractAddress, pldtypes.HexBytes(encodedCall)), prepareRes.Transaction.ParamsJson)

	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	manifestState.Id = confutil.P(pldtypes.RandBytes32().String()) // manifest is odd one out that  doesn't get ID allocated during assemble
	mt := newManifestTester(t, ctx, n, mockCallbacks, tx.TransactionId, assembleRes.AssembledTransaction)
	mt.withMissingStates( /* no missing states */ ).
		completeForIdentity(notaryAddress).
		completeForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(manifestState, dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(outCoin1State).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(lockState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())

	receipt := testGetDomainReceipt(t, n, &prototk.BuildReceiptRequest{
		TransactionId:     tx.TransactionId,
		UnavailableStates: false,
		InputStates:       inputStates,
		OutputStates:      outputStates,
		InfoStates:        infoStates,
	})
	require.Equal(t, lockInfo.LockID, receipt.LockInfo.LockID)
	require.Empty(t, receipt.LockInfo.UnlockFunction) // not prepared
	require.Nil(t, receipt.LockInfo.UnlockParams)     // not prepared
	require.Equal(t, (*pldtypes.EthAddress)(&senderKey.Address), receipt.Sender)
}

func TestLock_V0(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       testSchema("coin"),
		lockedCoinSchema: testSchema("lockedCoin"),
		lockInfoSchemaV0: testSchema("lockInfo"),
		lockInfoSchemaV1: testSchema("UNUSED"), // needs to be there for coin filtering
		dataSchemaV0:     testSchema("data"),
	}
	ctx := t.Context()
	fn := types.NotoABI.Functions()["lock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	inputCoin := &types.NotoCoinState{
		ID: pldtypes.RandBytes32(),
		Data: types.NotoCoin{
			Owner:  (*pldtypes.EthAddress)(&senderKey.Address),
			Amount: pldtypes.Uint64ToUint256(100),
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
			ContractConfigJson: mustParseJSON(notoBasicConfigV0),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: `{
			"amount": 100,
			"data": "0x1234"
		}`,
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
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 2)
	assert.Equal(t, inputCoin.ID.String(), assembleRes.AssembledTransaction.InputStates[0].Id)

	outputCoin, err := n.unmarshalLockedCoin(assembleRes.AssembledTransaction.OutputStates[0].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), outputCoin.Owner.String())
	assert.Equal(t, "100", outputCoin.Amount.Int().String())
	assert.Equal(t, []string{"notary@node1", "sender@node1"}, assembleRes.AssembledTransaction.OutputStates[0].DistributionList)

	outputInfo, err := n.unmarshalInfo(assembleRes.AssembledTransaction.InfoStates[0].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())
	assert.Equal(t, []string{"notary@node1", "sender@node1"}, assembleRes.AssembledTransaction.InfoStates[0].DistributionList)

	lockInfo, err := n.unmarshalLockV0(assembleRes.AssembledTransaction.InfoStates[1].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), lockInfo.Owner.String())
	assert.Equal(t, lockInfo.LockID, outputCoin.LockID)
	assert.Equal(t, []string{"notary@node1", "sender@node1"}, assembleRes.AssembledTransaction.InfoStates[1].DistributionList)

	encodedLock, err := n.encodeLock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoCoin{&inputCoin.Data}, []*types.NotoCoin{}, []*types.NotoLockedCoin{outputCoin})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedLock)
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
			SchemaId:      hashName("lockedCoin"),
			Id:            "0x26b394af655bdc794a6d7cd7f8004eec20bffb374e4ddd24cdaefe554878d945",
			StateDataJson: assembleRes.AssembledTransaction.OutputStates[0].StateDataJson,
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
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		Inputs:            inputStates,
		Outputs:           outputStates,
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

	// Prepare once to test base invoke
	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		InputStates:       inputStates,
		OutputStates:      outputStates,
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
	expectedFunction := mustParseJSON(interfaceV0Build.ABI.Functions()["lock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)
	assert.JSONEq(t, fmt.Sprintf(`{
		"inputs": ["%s"],
		"outputs": [],
		"lockedOutputs": ["0x26b394af655bdc794a6d7cd7f8004eec20bffb374e4ddd24cdaefe554878d945"],
		"signature": "%s",
		"txId": "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		"data": "0x00010000015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000024cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d69101a0740ec8096b83653600fa7553d676fc92bcc6e203c3572d2cac4f1db2f"
	}`, inputCoin.ID, signatureBytes), prepareRes.Transaction.ParamsJson)

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
		InputStates:       inputStates,
		OutputStates:      outputStates,
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
	expectedFunction = mustParseJSON(hooksBuild.ABI.Functions()["onLock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)
	assert.JSONEq(t, fmt.Sprintf(`{
		"sender": "%s",
		"lockId": "%s",
		"from": "%s",
		"amount": "0x64",
		"data": "0x1234",
		"prepared": {
			"contractAddress": "%s",
			"encodedCall": "%s"
		}
	}`, senderKey.Address, lockInfo.LockID, senderKey.Address, contractAddress, pldtypes.HexBytes(encodedCall)), prepareRes.Transaction.ParamsJson)
}

func TestLockEmpty(t *testing.T) {
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
	fn := types.NotoABI.Functions()["lock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

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
			"amount": 0,
			"data": "0x1234"
		}`,
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
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 1) // lock
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 2)

	dataState := assembleRes.AssembledTransaction.InfoStates[1]

	outputInfo, err := n.unmarshalInfo(assembleRes.AssembledTransaction.InfoStates[1].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())
	assert.Equal(t, []string{"notary@node1", "sender@node1"}, assembleRes.AssembledTransaction.InfoStates[0].DistributionList)

	lockState := assembleRes.AssembledTransaction.OutputStates[0]
	lockInfo, err := n.unmarshalLockV1(lockState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), lockInfo.Owner.String())
	assert.Equal(t, []string{"notary@node1", "sender@node1"}, assembleRes.AssembledTransaction.InfoStates[1].DistributionList)

	encodedLock, err := n.encodeLock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoCoin{}, []*types.NotoCoin{}, []*types.NotoLockedCoin{})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedLock)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	inputStates := []*prototk.EndorsableState{}
	outputStates := []*prototk.EndorsableState{
		{
			SchemaId:      assembleRes.AssembledTransaction.OutputStates[0].SchemaId,
			Id:            *assembleRes.AssembledTransaction.OutputStates[0].Id,
			StateDataJson: assembleRes.AssembledTransaction.OutputStates[0].StateDataJson,
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      assembleRes.AssembledTransaction.InfoStates[1].SchemaId,
			Id:            *assembleRes.AssembledTransaction.InfoStates[1].Id,
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[1].StateDataJson,
		},
	}

	endorseRes, err := n.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		Inputs:            inputStates,
		Outputs:           outputStates,
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

	// Prepare once to test base invoke
	prepareRes, err := n.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		Transaction:       tx,
		ResolvedVerifiers: verifiers,
		InputStates:       inputStates,
		OutputStates:      outputStates,
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

	// Decode the parameters
	createLockABI := interfaceV2Build.ABI.Functions()["createLock"]
	expectedFunction := mustParseJSON(createLockABI)
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Validate the parameters
	params := decodeFnParams[CreateLockParams](t, createLockABI, prepareRes.Transaction.ParamsJson)
	require.Zero(t, params.SpendCommitment)
	require.Zero(t, params.CancelCommitment)
	notoParams := decodeSingleABITuple[types.NotoCreateLockArgs](t, types.NotoCreateLockArgsABI, params.CreateArgs)
	require.Equal(t, &types.NotoLockOptions{}, notoParams.Options)
	require.Equal(t, &types.NotoCreateLockArgs{
		TxId:         "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		Inputs:       []string{},
		Outputs:      []string{},
		Contents:     []string{},
		NewLockState: pldtypes.MustParseBytes32(*lockState.Id),
		Options:      &types.NotoLockOptions{},
		Proof:        signatureBytes,
	}, notoParams)
	data, err := n.decodeTransactionDataV1(ctx, params.Data)
	require.NoError(t, err)
	require.Equal(t, &types.NotoTransactionData_V1{
		InfoStates: []pldtypes.Bytes32{
			pldtypes.MustParseBytes32(*dataState.Id),
		},
	}, data)

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
		InputStates:       inputStates,
		OutputStates:      outputStates,
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
	expectedFunctionABI := hooksBuild.ABI.Functions()["onLock"]
	assert.JSONEq(t, mustParseJSON(expectedFunctionABI), prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)
	_, err = expectedFunctionABI.EncodeCallDataJSON([]byte(prepareRes.Transaction.ParamsJson))
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{
		"sender": "%s",
		"lockId": "%s",
		"from": "%s",
		"amount": "0x00",
		"data": "0x1234",
		"prepared": {
			"contractAddress": "%s",
			"encodedCall": "%s"
		}
	}`, senderKey.Address, lockInfo.LockID, senderKey.Address, contractAddress, pldtypes.HexBytes(encodedCall)), prepareRes.Transaction.ParamsJson)

	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	manifestState.Id = confutil.P(pldtypes.RandBytes32().String()) // manifest is odd one out that  doesn't get ID allocated during assemble
	mt := newManifestTester(t, ctx, n, mockCallbacks, tx.TransactionId, assembleRes.AssembledTransaction)
	mt.withMissingStates( /* no missing states */ ).
		completeForIdentity(notaryAddress).
		completeForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(manifestState, dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
	mt.withMissingNewStates(lockState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String())
}

func TestLockValidateParamsRevert(t *testing.T) {
	h := &lockHandler{}
	for _, tc := range []struct {
		name, params, match string
		config              *types.NotoParsedConfig
	}{
		{"malformed JSON", `{"amount":`, "unexpected end of JSON input", notoBasicConfigV1},
		{"V0 rejects a zero amount", `{"amount": 0}`, "PD200008.*'amount'", notoBasicConfigV0},
		{"V0 rejects an absent amount", `{}`, "PD200008.*'amount'", notoBasicConfigV0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.ValidateParams(t.Context(), tc.config, tc.params)
			assertRevert(t, err, tc.match)
		})
	}
}

func TestLockCheckAllowedRevertsWhenDisabled(t *testing.T) {
	allowLock := false
	h := &lockHandler{}
	tx := &types.ParsedTransaction{
		DomainConfig: &types.NotoParsedConfig{
			NotaryMode: types.NotaryModeBasic.Enum(),
			Options:    types.NotoOptions{Basic: &types.NotoBasicOptions{AllowLock: &allowLock}},
		},
	}
	assertEndorseRevert(t, h.checkAllowed(t.Context(), tx), "PD200030")
}
