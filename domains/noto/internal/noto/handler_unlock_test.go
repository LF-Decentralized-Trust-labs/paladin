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

func TestUnlock(t *testing.T) {
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
	fn := types.NotoABI.Functions()["unlock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	receiverAddress := "0x2000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	lockID := pldtypes.RandBytes32()
	inputCoin := &types.NotoLockedCoinState{
		ID: pldtypes.MustParseBytes32("0xe532ee16774660fceb6c941725d6045939d34263ce81cd17266e910ac0ec5277"),
		Data: types.NotoLockedCoin{
			Salt:   pldtypes.RandBytes32(),
			LockID: lockID,
			Owner:  (*pldtypes.EthAddress)(&senderKey.Address),
			Amount: pldtypes.Uint64ToUint256(100),
		},
	}
	inputLockInfo := &prototk.StoredState{
		Id:       pldtypes.RandBytes32().String(),
		SchemaId: hashName("lockInfo_v1"),
		DataJson: fmt.Sprintf(`{
			"lockId": "%s",
			"salt": "%s",
			"owner": "%s",
			"spender": "%s"
		}`, lockID, pldtypes.RandBytes32(), senderKey.Address, senderKey.Address),
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
						CreatedAt: 1,
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
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 2) // locked coin and lock info
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 3) // both manifests + output-info
	assert.Equal(t, inputCoin.ID.String(), assembleRes.AssembledTransaction.InputStates[0].Id)

	outputCoinState := assembleRes.AssembledTransaction.OutputStates[0]
	outputCoin, err := n.unmarshalCoin(outputCoinState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, receiverAddress, outputCoin.Owner.String())
	assert.Equal(t, "100", outputCoin.Amount.Int().String())

	dataState := assembleRes.AssembledTransaction.InfoStates[2]
	outputInfo, err := n.unmarshalInfo(dataState.StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())

	lockState := assembleRes.AssembledTransaction.InputStates[1]

	encodedTransfer, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{&inputCoin.Data}, []*types.NotoLockedCoin{}, []*types.NotoCoin{outputCoin})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedTransfer)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	inputStates := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("lockedCoin"),
			Id:            inputCoin.ID.String(),
			StateDataJson: mustParseJSON(inputCoin.Data),
		},
		{
			SchemaId:      lockState.SchemaId,
			Id:            lockState.Id,
			StateDataJson: inputLockInfo.DataJson,
		},
	}
	outputStates := []*prototk.EndorsableState{
		{
			SchemaId:      outputCoinState.SchemaId,
			Id:            *outputCoinState.Id,
			StateDataJson: outputCoinState.StateDataJson,
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      dataState.SchemaId,
			Id:            *dataState.Id,
			StateDataJson: dataState.StateDataJson,
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
	spendLockABI := interfaceV2Build.ABI.Functions()["spendLock"]
	expectedFunction := mustParseJSON(spendLockABI)
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Verify base invoke params
	params := decodeFnParams[SpendLockParams](t, spendLockABI, prepareRes.Transaction.ParamsJson)
	require.Equal(t, lockID, params.LockID)
	require.Empty(t, params.Data /* outer data not used */)

	// Validate the parameters
	notoParams := decodeSingleABITuple[types.NotoSpendLockArgs](t, types.NotoSpendLockArgsABI, params.SpendArgs)
	require.Equal(t, "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d", notoParams.TxId)
	require.Equal(t, []string{inputCoin.ID.String(), inputLockInfo.Id}, notoParams.Inputs)
	require.Equal(t, []string{*outputCoinState.Id}, notoParams.Outputs)
	require.Equal(t, signatureBytes, notoParams.Proof)
	data, err := n.decodeTransactionDataV1(ctx, notoParams.Data)
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
	expectedFunctionABI := hooksBuild.ABI.Functions()["onUnlock"]
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
	assert.Equal(t, encodedCall, []byte(hookParams.Prepared.EncodedCall))

	manifestState := assembleRes.AssembledTransaction.InfoStates[0]
	unlockManifestState := assembleRes.AssembledTransaction.InfoStates[1]
	manifestState.Id = confutil.P(pldtypes.RandBytes32().String()) // manifest is odd one out that  doesn't get ID allocated during assemble
	mt := newManifestTester(t, ctx, n, mockCallbacks, tx.TransactionId, assembleRes.AssembledTransaction)
	mt.withMissingStates( /* no missing states */ ).
		completeForIdentity(notaryAddress).
		completeForIdentity(senderKey.Address.String()).
		completeForIdentity(receiverAddress)
	mt.withMissingNewStates(manifestState, dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(unlockManifestState, dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(dataState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
	mt.withMissingNewStates(outputCoinState).
		incompleteForIdentity(notaryAddress).
		incompleteForIdentity(senderKey.Address.String()).
		incompleteForIdentity(receiverAddress)
}

func TestUnlock_V0(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       testSchema("coin"),
		lockedCoinSchema: testSchema("lockedCoin"),
		lockInfoSchemaV0: testSchema("lockInfo"),
		lockInfoSchemaV1: testSchema("UNUSED"), // needs to be there for coin filtering
		dataSchemaV0:     testSchema("data"),
	}
	ctx := context.Background()
	fn := types.NotoABI.Functions()["unlock"]

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
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 2)
	assert.Equal(t, inputCoin.ID.String(), assembleRes.AssembledTransaction.InputStates[0].Id)
	outputCoin, err := n.unmarshalCoin(assembleRes.AssembledTransaction.OutputStates[0].StateDataJson)
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

	encodedTransfer, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{&inputCoin.Data}, []*types.NotoLockedCoin{}, []*types.NotoCoin{outputCoin})
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedTransfer)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	inputStates := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("lockedCoin"),
			Id:            inputCoin.ID.String(),
			StateDataJson: mustParseJSON(inputCoin.Data),
		},
	}
	outputStates := []*prototk.EndorsableState{
		{
			SchemaId:      hashName("coin"),
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
	expectedFunction := mustParseJSON(interfaceV0Build.ABI.Functions()["unlock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)
	assert.JSONEq(t, fmt.Sprintf(`{
		"lockedInputs": ["%s"],
		"lockedOutputs": [],
		"outputs": ["0x26b394af655bdc794a6d7cd7f8004eec20bffb374e4ddd24cdaefe554878d945"],
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
	expectedFunction = mustParseJSON(hooksBuild.ABI.Functions()["onUnlock"])
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
}

func TestLockCommonCheckAllowedLocalNodeNameFailed(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	mockCallbacks.MockLocalNodeName = func() (*prototk.LocalNodeNameResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	h := &lockCommon{noto: &Noto{Callbacks: mockCallbacks}}
	tx := &types.ParsedTransaction{
		Transaction:  &prototk.TransactionSpecification{From: "sender@node1"},
		DomainConfig: &types.NotoParsedConfig{NotaryMode: types.NotaryModeBasic.Enum()},
	}
	assertEndorseInternal(t, h.checkAllowed(t.Context(), tx, "sender"), "pop")
}

func TestUnlockValidateParamsRevert(t *testing.T) {
	h := &unlockHandler{}
	lockID := pldtypes.RandBytes32()
	for _, tc := range []struct{ name, params, match string }{
		{"malformed JSON", `{"lockId":`, "unexpected end of JSON input"},
		{"missing lockId", `{"from": "sender@node1"}`, "PD200007.*'lockId'"},
		{"missing from", fmt.Sprintf(`{"lockId": "%s"}`, lockID), "PD200007.*'from'"},
		{"missing recipients", fmt.Sprintf(`{"lockId": "%s", "from": "sender@node1"}`, lockID), "PD200007.*'recipients'"},
		{"zero recipient amount", fmt.Sprintf(`{"lockId": "%s", "from": "sender@node1", "recipients": [{"to": "receiver@node2", "amount": 0}]}`, lockID), "PD200008.*'recipient amount'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.ValidateParams(t.Context(), notoBasicConfigV1, tc.params)
			assertRevert(t, err, tc.match)
		})
	}
}

func TestLockCommonCheckAllowedRevertsOnInvalidFrom(t *testing.T) {
	h := &lockCommon{noto: &Noto{Callbacks: newMockCallbacks()}}
	for _, tc := range []struct{ name, txFrom, from, match string }{
		{"unparsable locator", "sender@node1", "sender@node1@node2", "PD020006"},
		{"not the lock creator", "someoneelse@node1", "sender", "PD200031"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &types.ParsedTransaction{
				Transaction:  &prototk.TransactionSpecification{From: tc.txFrom},
				DomainConfig: &types.NotoParsedConfig{NotaryMode: types.NotaryModeBasic.Enum()},
			}
			assertEndorseRevert(t, h.checkAllowed(t.Context(), tx, tc.from), tc.match)
		})
	}
}
