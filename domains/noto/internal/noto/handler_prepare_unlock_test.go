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

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareUnlock(t *testing.T) {
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       &prototk.StateSchema{Id: "coin"},
		lockedCoinSchema: &prototk.StateSchema{Id: "lockedCoin"},
		lockInfoSchema:   &prototk.StateSchema{Id: "lockInfo"},
		dataSchema:       &prototk.StateSchema{Id: "data"},
	}
	ctx := context.Background()
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
			Amount: pldtypes.Int64ToInt256(100),
		},
	}
	mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					Id:       inputCoin.ID.String(),
					SchemaId: "lockedCoin",
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
	lockInfo, err := n.unmarshalLock(assembleRes.AssembledTransaction.InfoStates[1].StateDataJson)
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
			SchemaId:      "lockedCoin",
			Id:            inputCoin.ID.String(),
			StateDataJson: mustParseJSON(inputCoin.Data),
		},
	}
	infoStates := []*prototk.EndorsableState{
		{
			SchemaId:      "data",
			Id:            "0x4cc7840e186de23c4127b4853c878708d2642f1942959692885e098f1944547d",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[0].StateDataJson,
		},
		{
			SchemaId:      "lockInfo",
			Id:            "0x69101A0740EC8096B83653600FA7553D676FC92BCC6E203C3572D2CAC4F1DB2F",
			StateDataJson: assembleRes.AssembledTransaction.InfoStates[1].StateDataJson,
		},
		{
			SchemaId:      "coin",
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

	unlockHash, err := n.unlockHashFromStates(ctx, ethtypes.MustNewAddress(contractAddress), readStates, nil, n.filterSchema(infoStates, []string{"coin"}), pldtypes.MustParseHexBytes("0x1234"))
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
}
