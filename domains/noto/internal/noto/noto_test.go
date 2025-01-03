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

	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var encodedConfig = func() []byte {
	configData := tktypes.HexBytes(`{"notaryLookup":"notary@node1"}`)
	encoded, err := types.NotoConfigABI_V0.EncodeABIDataJSON([]byte(fmt.Sprintf(`{
		"notaryAddress": "0x138baffcdcc3543aad1afd81c71d2182cdf9c8cd",
		"variant": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"data": "%s"
	}`, configData.String())))
	if err != nil {
		panic(err)
	}
	var result []byte
	result = append(result, types.NotoConfigID_V0...)
	result = append(result, encoded...)
	return result
}()

var mockCallbacks = &domain.MockDomainCallbacks{
	MockLocalNodeName: func() (*prototk.LocalNodeNameResponse, error) {
		return &prototk.LocalNodeNameResponse{
			Name: "node1",
		}, nil
	},
}

func TestConfigureDomainBadConfig(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: "!!wrong",
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitDeployBadParams(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestPrepareDeployBadParams(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestPrepareDeployMissingVerifier(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: "{}",
		},
	})
	assert.ErrorContains(t, err, "PD200011")
}

func TestInitTransactionBadAbi(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitTransactionBadFunction(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
			},
			FunctionAbiJson: `{"name": "does-not-exist"}`,
		},
	})
	assert.ErrorContains(t, err, "PD200001")
}

func TestInitContractOk(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	res, err := n.InitContract(context.Background(), &prototk.InitContractRequest{
		ContractAddress: tktypes.RandAddress().String(),
		ContractConfig:  encodedConfig,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{
		"notaryAddress": "0x138baffcdcc3543aad1afd81c71d2182cdf9c8cd",
		"notaryLookup": "notary@node1",
		"isNotary": true,
		"notaryType": "0x0",
		"restrictMint": false,
		"allowBurn": false,
		"variant": "0x0"
	}`, res.ContractConfig.ContractConfigJson)
}

func TestInitTransactionBadParams(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
			},
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitTransactionMissingTo(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
			},
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: "{}",
		},
	})
	assert.ErrorContains(t, err, "PD200007")
}

func TestInitTransactionMissingAmount(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
			},
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: `{"to": "recipient"}`,
		},
	})
	assert.ErrorContains(t, err, "PD200008")
}

func TestInitTransactionBadSignature(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
			},
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: `{"to": "recipient", "amount": 1}`,
		},
	})
	assert.ErrorContains(t, err, "PD200002")
}

func TestAssembleTransactionBadAbi(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.AssembleTransaction(context.Background(), &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestEndorseTransactionBadAbi(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.EndorseTransaction(context.Background(), &prototk.EndorseTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestPrepareTransactionBadAbi(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.PrepareTransaction(context.Background(), &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestHandleEventBatch_NotoTransfer(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	input := tktypes.RandBytes32()
	output := tktypes.RandBytes32()
	event := &NotoTransfer_Event{
		Inputs:    []tktypes.Bytes32{input},
		Outputs:   []tktypes.Bytes32{output},
		Signature: tktypes.MustParseHexBytes("0x1234"),
		Data:      tktypes.MustParseHexBytes("0x"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoTransfer],
				DataJson:          string(notoEventJson),
			},
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, input.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 1)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
}

func TestHandleEventBatch_NotoTransferBadData(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoTransfer],
				DataJson:          "!!wrong",
			}},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatch_NotoTransferBadTransactionData(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	event := &NotoTransfer_Event{
		Data: tktypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoTransfer],
				DataJson:          string(notoEventJson),
			}},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22047")
}

func TestHandleEventBatch_NotoLock(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	input := tktypes.RandBytes32()
	output := tktypes.RandBytes32()
	lockedOutput := tktypes.RandBytes32()
	event := &NotoLock_Event{
		LockID:        tktypes.RandBytes32(),
		Inputs:        []tktypes.Bytes32{input},
		Outputs:       []tktypes.Bytes32{output},
		LockedOutputs: []tktypes.Bytes32{lockedOutput},
		Signature:     tktypes.MustParseHexBytes("0x1234"),
		Data:          tktypes.MustParseHexBytes("0x"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoLock],
				DataJson:          string(notoEventJson),
			},
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, input.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 2)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
	assert.Equal(t, lockedOutput.String(), res.ConfirmedStates[1].Id)
}

func TestHandleEventBatch_NotoLockBadData(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoLock],
				DataJson:          "!!wrong",
			}},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatch_NotoLockBadTransactionData(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	event := &NotoTransfer_Event{
		Data: tktypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoLock],
				DataJson:          string(notoEventJson),
			}},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22047")
}

func TestHandleEventBatch_NotoUnlock(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	lockedInput := tktypes.RandBytes32()
	output := tktypes.RandBytes32()
	lockedOutput := tktypes.RandBytes32()
	event := &NotoUnlock_Event{
		LockID:        tktypes.RandBytes32(),
		LockedInputs:  []tktypes.Bytes32{lockedInput},
		LockedOutputs: []tktypes.Bytes32{lockedOutput},
		Outputs:       []tktypes.Bytes32{output},
		Signature:     tktypes.MustParseHexBytes("0x1234"),
		Data:          tktypes.MustParseHexBytes("0x"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoUnlock],
				DataJson:          string(notoEventJson),
			},
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, lockedInput.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 2)
	assert.Equal(t, lockedOutput.String(), res.ConfirmedStates[0].Id)
	assert.Equal(t, output.String(), res.ConfirmedStates[1].Id)
}

func TestHandleEventBatch_NotoUnlockBadData(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoUnlock],
				DataJson:          "!!wrong",
			}},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}

func TestHandleEventBatch_NotoUnlockBadTransactionData(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	event := &NotoTransfer_Event{
		Data: tktypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: n.eventSignatures[NotoUnlock],
				DataJson:          string(notoEventJson),
			}},
	}

	_, err = n.HandleEventBatch(ctx, req)
	require.ErrorContains(t, err, "FF22047")
}
