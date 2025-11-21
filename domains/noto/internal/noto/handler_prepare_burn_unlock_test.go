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
	"fmt"
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

func TestPrepareBurnUnlock(t *testing.T) {
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
	fn := types.NotoABI.Functions()["prepareBurnUnlock"]

	notaryAddress := "0x1000000000000000000000000000000000000000"
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	lockID := pldtypes.RandBytes32()
	lockedCoin := &types.NotoLockedCoinState{
		ID: pldtypes.RandBytes32(),
		Data: types.NotoLockedCoin{
			LockID: lockID,
			Owner:  (*pldtypes.EthAddress)(&senderKey.Address),
			Amount: pldtypes.Int64ToInt256(100),
		},
	}
	mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					Id:       lockedCoin.ID.String(),
					SchemaId: "lockedCoin",
					DataJson: mustParseJSON(lockedCoin.Data),
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
				Variant:      types.NotoVariantDefault, // V1 required for prepareBurnUnlock
				Options: types.NotoOptions{
					Basic: &types.NotoBasicOptions{
						AllowBurn: boolPtr(true),
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
	require.Len(t, assembleRes.AssembledTransaction.InputStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.OutputStates, 0)
	require.Len(t, assembleRes.AssembledTransaction.ReadStates, 1)
	require.Len(t, assembleRes.AssembledTransaction.InfoStates, 2) // data + lockInfo (no outputs for burn)
	assert.Equal(t, lockedCoin.ID.String(), assembleRes.AssembledTransaction.ReadStates[0].Id)
	outputInfo, err := n.unmarshalInfo(assembleRes.AssembledTransaction.InfoStates[0].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, "0x1234", outputInfo.Data.String())
	lockInfo, err := n.unmarshalLock(assembleRes.AssembledTransaction.InfoStates[1].StateDataJson)
	require.NoError(t, err)
	assert.Equal(t, senderKey.Address.String(), lockInfo.Owner.String())
	assert.Equal(t, lockID, lockInfo.LockID)

	// Prepare unlock with no outputs (for burning)
	encodedUnlock, err := n.encodeUnlock(ctx, ethtypes.MustNewAddress(contractAddress), []*types.NotoLockedCoin{&lockedCoin.Data}, nil, nil)
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(encodedUnlock)
	require.NoError(t, err)
	signatureBytes := pldtypes.HexBytes(signature.CompactRSV())

	readStates := []*prototk.EndorsableState{
		{
			SchemaId:      "lockedCoin",
			Id:            lockedCoin.ID.String(),
			StateDataJson: mustParseJSON(lockedCoin.Data),
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      "data_v1",
			Id:            "0x4cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[0].StateDataJson,
		},
		{
			SchemaId:      "lockInfo",
			Id:            "0x69101A0740EC8096B83653600FA7553D676FC92BCC6E203C3572D2CAC4F1DB2F",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[1].StateDataJson,
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

	unlockHash, err := n.unlockHashFromStates(ctx, ethtypes.MustNewAddress(contractAddress), readStates, nil, nil, pldtypes.MustParseHexBytes("0x1234"))
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
	expectedFunction := mustParseJSON(interfaceBuild.ABI.Functions()["prepareUnlock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Nil(t, prepareRes.Transaction.ContractAddress)

	// Verify base invoke params
	var baseParams NotoPrepareUnlockParams
	err = json.Unmarshal([]byte(prepareRes.Transaction.ParamsJson), &baseParams)
	require.NoError(t, err)
	assert.Equal(t, "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d", *baseParams.TxId)
	assert.Equal(t, lockID, *baseParams.LockId)
	assert.NotEmpty(t, baseParams.UnlockTxId)
	assert.Equal(t, []string{lockedCoin.ID.String()}, baseParams.LockedInputs)
	assert.Equal(t, unlockHash.String(), baseParams.UnlockHash.String())
	assert.Equal(t, signatureBytes, baseParams.Signature)
	assert.NotEmpty(t, baseParams.Data)

	// Prepare again to test hook invoke
	hookAddress := "0x515fba7fe1d8b9181be074bd4c7119544426837c"
	tx.ContractInfo.ContractConfigJson = mustParseJSON(&types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		NotaryMode:   types.NotaryModeHooks.Enum(),
		Variant:      types.NotoVariantDefault,
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
	expectedFunction = mustParseJSON(hooksBuild.ABI.Functions()["onPrepareBurnUnlock"])
	assert.JSONEq(t, expectedFunction, prepareRes.Transaction.FunctionAbiJson)
	assert.Equal(t, &hookAddress, prepareRes.Transaction.ContractAddress)

	// Verify hook invoke params
	var hookParams PrepareBurnUnlockHookParams
	err = json.Unmarshal([]byte(prepareRes.Transaction.ParamsJson), &hookParams)
	require.NoError(t, err)
	require.NotNil(t, hookParams.Sender)
	assert.Equal(t, senderKey.Address.String(), hookParams.Sender.String())
	assert.Equal(t, lockID, hookParams.LockId)
	require.NotNil(t, hookParams.From)
	assert.Equal(t, senderKey.Address.String(), hookParams.From.String())
	require.NotNil(t, hookParams.Amount)
	assert.Equal(t, pldtypes.Int64ToInt256(100).String(), hookParams.Amount.String())
	assert.Equal(t, pldtypes.MustParseHexBytes("0x1234"), hookParams.Data)

	// Verify prepared transaction
	assert.Equal(t, pldtypes.MustEthAddress(contractAddress), hookParams.Prepared.ContractAddress)
	assert.NotEmpty(t, hookParams.Prepared.EncodedCall)
}

func TestPrepareBurnUnlock_InvalidParams(t *testing.T) {
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
	fn := types.NotoABI.Functions()["prepareBurnUnlock"]

	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	lockID := pldtypes.RandBytes32()

	// Test missing lockId
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress,
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				NotaryLookup: "notary@node1",
				Variant:      types.NotoVariantDefault,
			}),
		},
		FunctionAbiJson:   mustParseJSON(fn),
		FunctionSignature: fn.SolString(),
		FunctionParamsJson: `{
			"from": "sender@node1",
			"amount": 100,
			"data": "0x1234"
		}`,
	}
	_, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lockId")

	// Test missing from
	tx.FunctionParamsJson = fmt.Sprintf(`{
		"lockId": "%s",
		"amount": 100,
		"data": "0x1234"
	}`, lockID)
	_, err = n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "from")

	// Test invalid amount
	tx.FunctionParamsJson = fmt.Sprintf(`{
		"lockId": "%s",
		"from": "sender@node1",
		"amount": 0,
		"data": "0x1234"
	}`, lockID)
	_, err = n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "amount")

	// Test V0 not supported
	tx.FunctionParamsJson = fmt.Sprintf(`{
		"lockId": "%s",
		"from": "sender@node1",
		"amount": 100,
		"data": "0x1234"
	}`, lockID)
	tx.ContractInfo.ContractConfigJson = mustParseJSON(&types.NotoParsedConfig{
		NotaryLookup: "notary@node1",
		Variant:      types.NotoVariantLegacy, // V0
	})
	_, err = n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported in Noto V0")
}

func TestPrepareBurnUnlock_BurnNotAllowed(t *testing.T) {
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
	fn := types.NotoABI.Functions()["prepareBurnUnlock"]

	lockID := pldtypes.RandBytes32()
	contractAddress := "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3"
	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: contractAddress,
			ContractConfigJson: mustParseJSON(&types.NotoParsedConfig{
				NotaryLookup: "notary@node1",
				NotaryMode:   types.NotaryModeBasic.Enum(),
				Variant:      types.NotoVariantDefault,
				Options: types.NotoOptions{
					Basic: &types.NotoBasicOptions{
						AllowBurn: boolPtr(false), // Burn not allowed
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
			"data": "0x1234"
		}`, lockID),
	}

	_, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Burn is not enabled")
}

func boolPtr(b bool) *bool {
	return &b
}
