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

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleEventBatch_NotoTransfer(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: `{}`,
	})
	require.NoError(t, err)

	txId := pldtypes.RandBytes32()
	input := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	event := &NotoTransfer_Event{
		TxId:    txId,
		Inputs:  []pldtypes.Bytes32{input},
		Outputs: []pldtypes.Bytes32{output},
		Proof:   pldtypes.MustParseHexBytes("0x1234"),
		Data:    pldtypes.MustParseHexBytes("0x"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventTransfer],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{"variant": "0x0001"}`,
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
				SoliditySignature: eventSignatures[EventTransfer],
				DataJson:          "!!wrong",
			}},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{"variant": "0x0001"}`,
		},
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

	txId := pldtypes.RandBytes32()
	event := &NotoTransfer_Event{
		TxId: txId,
		Data: pldtypes.MustParseHexBytes("0x00010000"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventTransfer],
				DataJson:          string(notoEventJson),
			}},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{"variant": "0x0001"}`,
		},
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

	txId := pldtypes.RandBytes32()
	lockId := pldtypes.RandBytes32()
	input := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	lockedOutput := pldtypes.RandBytes32()
	owner := (*pldtypes.EthAddress)(pldtypes.RandAddress())
	spender := (*pldtypes.EthAddress)(pldtypes.RandAddress())
	event := &NotoLockCreated_Event{
		TxId:          txId,
		LockID:        lockId,
		Owner:         owner,
		Spender:       spender,
		Inputs:        []pldtypes.Bytes32{input},
		Outputs:       []pldtypes.Bytes32{output},
		LockedOutputs: []pldtypes.Bytes32{lockedOutput},
		Proof:         pldtypes.MustParseHexBytes("0x1234"),
		Data:          pldtypes.MustParseHexBytes("0x"),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventLockCreated],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{"variant": "0x0001"}`,
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
				SoliditySignature: eventSignatures[EventLockCreated],
				DataJson:          "!!wrong",
			}},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{"variant": "0x0001"}`,
		},
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

	txId := pldtypes.RandBytes32()
	lockId := pldtypes.RandBytes32()
	owner := (*pldtypes.EthAddress)(pldtypes.RandAddress())
	spender := (*pldtypes.EthAddress)(pldtypes.RandAddress())
	event := &NotoLockCreated_Event{
		TxId:          txId,
		LockID:        lockId,
		Owner:         owner,
		Spender:       spender,
		Inputs:        []pldtypes.Bytes32{},
		Outputs:       []pldtypes.Bytes32{},
		LockedOutputs: []pldtypes.Bytes32{},
		Proof:         pldtypes.MustParseHexBytes("0x"),
		Data:          pldtypes.MustParseHexBytes("0x00010000"), // Invalid transaction data
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventLockCreated],
				DataJson:          string(notoEventJson),
			}},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{"variant": "0x0001"}`,
		},
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

	txId := pldtypes.RandBytes32()
	lockId := pldtypes.RandBytes32()
	lockedInput := pldtypes.RandBytes32()
	output := pldtypes.RandBytes32()
	spender := (*pldtypes.EthAddress)(pldtypes.RandAddress())

	unlockParams := &UnlockData{
		TxId:    txId,
		Inputs:  []pldtypes.Bytes32{lockedInput},
		Outputs: []pldtypes.Bytes32{output},
		Data:    pldtypes.MustParseHexBytes("0x"),
	}
	unlockParamsJSON, err := json.Marshal(unlockParams)
	require.NoError(t, err)
	unlockParamsEncoded, err := UnlockDataABI.EncodeABIDataJSONCtx(ctx, unlockParamsJSON)
	require.NoError(t, err)

	event := &NotoLockSpent_Event{
		LockID:  lockId,
		Spender: spender,
		Data:    pldtypes.HexBytes(unlockParamsEncoded),
	}
	notoEventJson, err := json.Marshal(event)
	require.NoError(t, err)

	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				SoliditySignature: eventSignatures[EventLockSpent],
				DataJson:          string(notoEventJson),
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{"variant": "0x0001"}`,
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 1)
	require.Len(t, res.SpentStates, 1)
	assert.Equal(t, lockedInput.String(), res.SpentStates[0].Id)
	require.Len(t, res.ConfirmedStates, 1)
	assert.Equal(t, output.String(), res.ConfirmedStates[0].Id)
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
				SoliditySignature: eventSignatures[EventLockSpent],
				DataJson:          "!!wrong",
			}},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{"variant": "0x0001"}`,
		},
	}

	res, err := n.HandleEventBatch(ctx, req)
	require.NoError(t, err)
	require.Len(t, res.TransactionsComplete, 0)
	require.Len(t, res.SpentStates, 0)
	require.Len(t, res.ConfirmedStates, 0)
}
