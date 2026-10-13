// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package zeto

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	coinSchemaID     = "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"
	nftSchemaID      = "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"
	smtRootSchemaID  = "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ee"
	smtNodeSchemaID  = "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ef"
	dataSchemaID     = "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95f0"
	unknownSchemaID  = "0x0999999999999999999999999999999999999999999999999999999999999999"
	stateID1         = "0x1234567890123456789012345678901234567890123456789012345678901231"
	stateID2         = "0x1234567890123456789012345678901234567890123456789012345678901232"
	stateID3         = "0x1234567890123456789012345678901234567890123456789012345678901233"
	stateID4         = "0x1234567890123456789012345678901234567890123456789012345678901234"
	infoStateID      = "0x1a2b3c4d5e6f7081928374655647382910a1b2c3d4e5f6071827364556677889"
	otherInfoStateID = "0x1a2b3c4d5e6f7081928374655647382910a1b2c3d4e5f607182736455667788a"
)

var (
	owner1 = pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	owner2 = pldtypes.MustParseHexBytes("0x7ddd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8026")
	owner3 = pldtypes.MustParseHexBytes("0x7edd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8027")
)

func newReceiptTestZeto() *Zeto {
	return &Zeto{
		coinSchema:           &prototk.StateSchema{Id: coinSchemaID},
		nftSchema:            &prototk.StateSchema{Id: nftSchemaID},
		merkleTreeRootSchema: &prototk.StateSchema{Id: smtRootSchemaID},
		merkleTreeNodeSchema: &prototk.StateSchema{Id: smtNodeSchemaID},
		dataSchema:           &prototk.StateSchema{Id: dataSchemaID},
	}
}

func coinState(id string, owner pldtypes.HexBytes, amount int64, locked bool) *prototk.EndorsableState {
	return &prototk.EndorsableState{
		Id:            id,
		SchemaId:      coinSchemaID,
		StateDataJson: fmt.Sprintf(`{"salt":"0x1234","owner":"%s","amount":%d,"locked":%t}`, owner, amount, locked),
	}
}

func nftState(id string, owner pldtypes.HexBytes, tokenID string) *prototk.EndorsableState {
	return &prototk.EndorsableState{
		Id:            id,
		SchemaId:      nftSchemaID,
		StateDataJson: fmt.Sprintf(`{"salt":"0x1234","uri":"https://example.com/1","owner":"%s","tokenID":"%s"}`, owner, tokenID),
	}
}

func infoState(id string, data string) *prototk.EndorsableState {
	return &prototk.EndorsableState{
		Id:            id,
		SchemaId:      dataSchemaID,
		StateDataJson: fmt.Sprintf(`{"salt":"0xabcdef","data":"%s"}`, data),
	}
}

func buildTestReceipt(t *testing.T, z *Zeto, req *prototk.BuildReceiptRequest) *types.ZetoDomainReceipt {
	res, err := z.buildReceipt(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, res.ReceiptJson)
	receipt := &types.ZetoDomainReceipt{}
	require.NoError(t, json.Unmarshal([]byte(res.ReceiptJson), receipt))
	return receipt
}

func TestBuildReceipt(t *testing.T) {
	res, err := newReceiptTestZeto().BuildReceipt(context.Background(), &prototk.BuildReceiptRequest{
		TransactionId: "0x1a2b3c4d5e6f7081928374655647382910a1b2c3d4e5f6071827364556677889",
		OutputStates:  []*prototk.EndorsableState{coinState(stateID1, owner1, 10, false)},
	})
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{
		"states": {
			"outputs": [{
				"id": "%s",
				"schema": "%s",
				"data": {"salt":"0x1234","owner":"%s","amount":10,"locked":false}
			}]
		},
		"transfers": [{"to": "%s", "amount": "0x0a"}]
	}`, stateID1, coinSchemaID, owner1, owner1), res.ReceiptJson)
}

func TestBuildReceiptNoStates(t *testing.T) {
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{})
	assert.Empty(t, receipt.States.Inputs)
	assert.Empty(t, receipt.States.LockedInputs)
	assert.Empty(t, receipt.States.Outputs)
	assert.Empty(t, receipt.States.LockedOutputs)
	assert.Empty(t, receipt.Transfers)
}

func TestBuildReceiptMint(t *testing.T) {
	// Each mint entry is reported as its own transfer, carrying that entry's data, even where the
	// entries are to the same owner
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InfoStates: []*prototk.EndorsableState{
			infoState(infoStateID, "0xdeadbeef"),
			infoState(otherInfoStateID, "0xfeedface"),
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 10, false),
			coinState(stateID2, owner1, 20, false),
		},
	})
	assert.Empty(t, receipt.States.Inputs)
	require.Len(t, receipt.States.Outputs, 2)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID1), receipt.States.Outputs[0].ID)
	assert.Equal(t, pldtypes.MustParseBytes32(coinSchemaID), receipt.States.Outputs[0].Schema)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID2), receipt.States.Outputs[1].ID)
	assert.Equal(t, []*types.ReceiptTransfer{
		{To: owner1, Amount: pldtypes.Uint64ToUint256(10), Data: pldtypes.MustParseHexBytes("0xdeadbeef")},
		{To: owner1, Amount: pldtypes.Uint64ToUint256(20), Data: pldtypes.MustParseHexBytes("0xfeedface")},
	}, receipt.Transfers)
}

func TestBuildReceiptTransferDataPerRecipient(t *testing.T) {
	// A recipient is only distributed the info state for their own entry, so from their point of view
	// there is one info state and one transfer, and the data is theirs
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InfoStates: []*prototk.EndorsableState{infoState(infoStateID, "0xfeedface")},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner2, 25, false),
		},
	})
	require.Len(t, receipt.Transfers, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0xfeedface"), receipt.Transfers[0].Data)
}

func TestBuildReceiptTransferDataPerEntry(t *testing.T) {
	// The sender sees every info state. Entries are matched to their coin by position, so each
	// recipient gets the data that was supplied on their own entry.
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InfoStates: []*prototk.EndorsableState{
			infoState(infoStateID, "0xdeadbeef"),
			infoState(otherInfoStateID, "0xfeedface"),
		},
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 100, false),
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID2, owner2, 40, false),
			coinState(stateID3, owner3, 60, false),
		},
	})
	assert.Equal(t, []*types.ReceiptTransfer{
		{From: owner1, To: owner2, Amount: pldtypes.Uint64ToUint256(40), Data: pldtypes.MustParseHexBytes("0xdeadbeef")},
		{From: owner1, To: owner3, Amount: pldtypes.Uint64ToUint256(60), Data: pldtypes.MustParseHexBytes("0xfeedface")},
	}, receipt.Transfers)
}

func TestBuildReceiptChangeCarriesNoData(t *testing.T) {
	// Change is appended after the entries, so it falls beyond the info states and is netted off
	// rather than reported - the entry's data must not leak onto it
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InfoStates: []*prototk.EndorsableState{infoState(infoStateID, "0xdeadbeef")},
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 100, false),
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID2, owner2, 40, false),
			coinState(stateID3, owner1, 60, false), // change back to the sender
		},
	})
	assert.Equal(t, []*types.ReceiptTransfer{
		{From: owner1, To: owner2, Amount: pldtypes.Uint64ToUint256(40), Data: pldtypes.MustParseHexBytes("0xdeadbeef")},
	}, receipt.Transfers)
}

func TestBuildReceiptNoTopLevelData(t *testing.T) {
	// None of Zeto's methods take a top-level data parameter, so the receipt must not report one
	res, err := newReceiptTestZeto().buildReceipt(context.Background(), &prototk.BuildReceiptRequest{
		InfoStates:   []*prototk.EndorsableState{infoState(infoStateID, "0xdeadbeef")},
		OutputStates: []*prototk.EndorsableState{coinState(stateID1, owner1, 10, false)},
	})
	require.NoError(t, err)

	var raw struct {
		Data      *pldtypes.HexBytes `json:"data"`
		Transfers []struct {
			Data *pldtypes.HexBytes `json:"data"`
		} `json:"transfers"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.ReceiptJson), &raw))
	assert.Nil(t, raw.Data, "no top-level data on the receipt")
	require.Len(t, raw.Transfers, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0xdeadbeef"), *raw.Transfers[0].Data)
}

func TestBuildReceiptBurn(t *testing.T) {
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 1, false),
		},
	})
	require.Len(t, receipt.States.Inputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID1), receipt.States.Inputs[0].ID)
	assert.Equal(t, []*types.ReceiptTransfer{{
		From:   owner1,
		Amount: pldtypes.Uint64ToUint256(1),
	}}, receipt.Transfers)
}

func TestBuildReceiptBurnWithRemainder(t *testing.T) {
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 10, false),
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID2, owner1, 8, false),
		},
	})
	require.Len(t, receipt.States.Inputs, 1)
	require.Len(t, receipt.States.Outputs, 1)
	assert.Equal(t, []*types.ReceiptTransfer{{
		From:   owner1,
		Amount: pldtypes.Uint64ToUint256(2),
	}}, receipt.Transfers)
}

func TestBuildReceiptTransferWithChange(t *testing.T) {
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 10, false),
			coinState(stateID2, owner1, 20, false),
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID3, owner2, 25, false),
			coinState(stateID4, owner1, 5, false),
		},
	})
	require.Len(t, receipt.States.Inputs, 2)
	require.Len(t, receipt.States.Outputs, 2)
	assert.Equal(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     owner2,
		Amount: pldtypes.Uint64ToUint256(25),
	}}, receipt.Transfers)
}

func TestBuildReceiptTransferReportsEachCoinSeparately(t *testing.T) {
	// A recipient named by more than one entry gets a transfer per entry rather than one combined
	// transfer, so that each entry's own data can be reported against it
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InfoStates: []*prototk.EndorsableState{
			infoState(stateID1, "0x01"),
			infoState(infoStateID, "0x02"),
			infoState(otherInfoStateID, "0x03"),
		},
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 100, false),
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID2, owner2, 30, false),
			coinState(stateID3, owner2, 20, false),
			coinState(stateID4, owner3, 50, false),
		},
	})
	assert.Equal(t, []*types.ReceiptTransfer{
		{From: owner1, To: owner2, Amount: pldtypes.Uint64ToUint256(30), Data: pldtypes.MustParseHexBytes("0x01")},
		{From: owner1, To: owner2, Amount: pldtypes.Uint64ToUint256(20), Data: pldtypes.MustParseHexBytes("0x02")},
		{From: owner1, To: owner3, Amount: pldtypes.Uint64ToUint256(50), Data: pldtypes.MustParseHexBytes("0x03")},
	}, receipt.Transfers)
}

func TestBuildReceiptLock(t *testing.T) {
	// Locking replaces unlocked coins with a locked coin of the same value owned by the same party,
	// plus unlocked change - so no value moves and the locked coin is reported separately
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 10, false),
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID2, owner1, 9, false),
			coinState(stateID3, owner1, 1, true),
		},
	})
	require.Len(t, receipt.States.Inputs, 1)
	assert.Empty(t, receipt.States.LockedInputs)
	require.Len(t, receipt.States.Outputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID2), receipt.States.Outputs[0].ID)
	require.Len(t, receipt.States.LockedOutputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID3), receipt.States.LockedOutputs[0].ID)
	assert.Empty(t, receipt.Transfers)
}

func TestBuildReceiptTransferLocked(t *testing.T) {
	// Spending a locked coin to another party is a transfer, with the locked coin reported as a
	// locked input
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 5, true),
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID2, owner2, 5, false),
		},
	})
	assert.Empty(t, receipt.States.Inputs)
	require.Len(t, receipt.States.LockedInputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID1), receipt.States.LockedInputs[0].ID)
	require.Len(t, receipt.States.Outputs, 1)
	assert.Empty(t, receipt.States.LockedOutputs)
	assert.Equal(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     owner2,
		Amount: pldtypes.Uint64ToUint256(5),
	}}, receipt.Transfers)
}

func TestBuildReceiptIgnoresMerkleTreeStates(t *testing.T) {
	// Nullifier tokens record their sparse merkle tree updates against the transaction
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			coinState(stateID1, owner1, 5, false),
			{Id: stateID2, SchemaId: smtRootSchemaID, StateDataJson: `{"smtName":"smt_1"}`},
		},
		OutputStates: []*prototk.EndorsableState{
			coinState(stateID3, owner2, 5, false),
			{Id: stateID4, SchemaId: smtNodeSchemaID, StateDataJson: `{"index":"0x01"}`},
		},
	})
	require.Len(t, receipt.States.Inputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID1), receipt.States.Inputs[0].ID)
	require.Len(t, receipt.States.Outputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID3), receipt.States.Outputs[0].ID)
	require.Len(t, receipt.Transfers, 1)
}

func TestBuildReceiptNFTMint(t *testing.T) {
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		OutputStates: []*prototk.EndorsableState{
			nftState(stateID1, owner1, "0xdeadbeef"),
			nftState(stateID2, owner1, "0xcafebabe"),
		},
	})
	require.Len(t, receipt.States.Outputs, 2)
	assert.Equal(t, []*types.ReceiptTransfer{
		{To: owner1, TokenID: pldtypes.MustParseHexUint256("0xdeadbeef")},
		{To: owner1, TokenID: pldtypes.MustParseHexUint256("0xcafebabe")},
	}, receipt.Transfers)
}

func TestBuildReceiptNFTTransfer(t *testing.T) {
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			nftState(stateID1, owner1, "0xdeadbeef"),
		},
		OutputStates: []*prototk.EndorsableState{
			nftState(stateID2, owner2, "0xdeadbeef"),
		},
	})
	require.Len(t, receipt.States.Inputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID1), receipt.States.Inputs[0].ID)
	require.Len(t, receipt.States.Outputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID2), receipt.States.Outputs[0].ID)
	assert.Equal(t, []*types.ReceiptTransfer{{
		From:    owner1,
		To:      owner2,
		TokenID: pldtypes.MustParseHexUint256("0xdeadbeef"),
	}}, receipt.Transfers)
}

func TestBuildReceiptNFTBurn(t *testing.T) {
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			nftState(stateID1, owner1, "0xdeadbeef"),
		},
	})
	assert.Equal(t, []*types.ReceiptTransfer{{
		From:    owner1,
		TokenID: pldtypes.MustParseHexUint256("0xdeadbeef"),
	}}, receipt.Transfers)
}

func TestBuildReceiptTransfersOmitUnusedFields(t *testing.T) {
	// A fungible transfer must not carry a tokenId, and a non-fungible one must not carry an amount
	z := newReceiptTestZeto()
	ctx := context.Background()

	res, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		OutputStates: []*prototk.EndorsableState{coinState(stateID1, owner1, 10, false)},
	})
	require.NoError(t, err)
	assert.NotContains(t, res.ReceiptJson, "tokenId")

	res, err = z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		OutputStates: []*prototk.EndorsableState{nftState(stateID1, owner1, "0xdeadbeef")},
	})
	require.NoError(t, err)
	assert.NotContains(t, res.ReceiptJson, "amount")
}

func TestBuildReceiptErrors(t *testing.T) {
	z := newReceiptTestZeto()
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		req  *prototk.BuildReceiptRequest
	}{
		{
			name: "invalid info state",
			req: &prototk.BuildReceiptRequest{
				InfoStates: []*prototk.EndorsableState{{Id: infoStateID, SchemaId: dataSchemaID, StateDataJson: `{invalid}`}},
			},
		},
		{
			name: "invalid input coin",
			req: &prototk.BuildReceiptRequest{
				InputStates: []*prototk.EndorsableState{{Id: stateID1, SchemaId: coinSchemaID, StateDataJson: `{invalid}`}},
			},
		},
		{
			name: "invalid output coin",
			req: &prototk.BuildReceiptRequest{
				OutputStates: []*prototk.EndorsableState{{Id: stateID1, SchemaId: coinSchemaID, StateDataJson: `{invalid}`}},
			},
		},
		{
			name: "invalid input NFT",
			req: &prototk.BuildReceiptRequest{
				InputStates: []*prototk.EndorsableState{{Id: stateID1, SchemaId: nftSchemaID, StateDataJson: `{invalid}`}},
			},
		},
		{
			// Regression: an unparseable input state ID used to be reported as a successful receipt,
			// because the error was overwritten while building the transfers
			name: "unparseable input state ID",
			req: &prototk.BuildReceiptRequest{
				InputStates: []*prototk.EndorsableState{{Id: "not-hex", SchemaId: coinSchemaID, StateDataJson: `{"amount":1}`}},
			},
		},
		{
			name: "unparseable output state ID",
			req: &prototk.BuildReceiptRequest{
				OutputStates: []*prototk.EndorsableState{{Id: "not-hex", SchemaId: coinSchemaID, StateDataJson: `{"amount":1}`}},
			},
		},
		{
			name: "duplicate input state",
			req: &prototk.BuildReceiptRequest{
				InputStates: []*prototk.EndorsableState{
					coinState(stateID1, owner1, 10, false),
					coinState(stateID1, owner1, 20, false),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := z.buildReceipt(ctx, tc.req)
			assert.Error(t, err)
		})
	}
}

func TestBuildReceiptToleratesMissingAmount(t *testing.T) {
	// State data comes back from storage, so a coin without an amount must not take down the receipt
	receipt := buildTestReceipt(t, newReceiptTestZeto(), &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{
			{Id: stateID1, SchemaId: coinSchemaID, StateDataJson: fmt.Sprintf(`{"owner":"%s"}`, owner1)},
		},
		OutputStates: []*prototk.EndorsableState{
			{Id: stateID2, SchemaId: coinSchemaID, StateDataJson: fmt.Sprintf(`{"owner":"%s"}`, owner2)},
		},
	})
	require.Len(t, receipt.States.Inputs, 1)
	assert.Empty(t, receipt.Transfers)
}

func TestFilterSchema(t *testing.T) {
	states := []*prototk.EndorsableState{
		{Id: "state1", SchemaId: "schema1", StateDataJson: "{}"},
		{Id: "state2", SchemaId: "schema2", StateDataJson: "{}"},
		{Id: "state3", SchemaId: "schema1", StateDataJson: "{}"},
		{Id: "state4", SchemaId: "schema3", StateDataJson: "{}"},
	}

	filtered := filterSchema(states, []string{"schema1"})
	require.Len(t, filtered, 2)
	assert.Equal(t, "state1", filtered[0].Id)
	assert.Equal(t, "state3", filtered[1].Id)

	filtered = filterSchema(states, []string{"schema1", "schema3"})
	assert.Len(t, filtered, 3)

	assert.Empty(t, filterSchema(states, []string{"schema99"}))
	assert.Empty(t, filterSchema(states, []string{}))
	assert.Empty(t, filterSchema([]*prototk.EndorsableState{}, []string{"schema1"}))
}

func TestUnmarshalInfo(t *testing.T) {
	info, err := unmarshalInfo(`{"salt":"0xabcd","data":"0xdeadbeef"}`)
	require.NoError(t, err)
	assert.Equal(t, pldtypes.MustParseHexUint256("0xabcd"), info.Salt)
	assert.Equal(t, pldtypes.MustParseHexBytes("0xdeadbeef"), info.Data)

	_, err = unmarshalInfo("{invalid json")
	assert.Error(t, err)

	info, err = unmarshalInfo("{}")
	require.NoError(t, err)
	assert.Nil(t, info.Salt)
	assert.Nil(t, info.Data)
}

func TestUnmarshalCoin(t *testing.T) {
	coin, err := unmarshalCoin(fmt.Sprintf(`{"amount":"100","owner":"%s","locked":true}`, owner1))
	require.NoError(t, err)
	assert.Equal(t, pldtypes.Uint64ToUint256(100), coin.Amount)
	assert.Equal(t, owner1, coin.Owner)
	assert.True(t, coin.Locked)

	_, err = unmarshalCoin("{invalid")
	assert.Error(t, err)

	coin, err = unmarshalCoin("{}")
	require.NoError(t, err)
	assert.Nil(t, coin.Amount)
	assert.Nil(t, coin.Owner)
	assert.False(t, coin.Locked)
}

func TestUnmarshalNFT(t *testing.T) {
	tokenID := pldtypes.MustParseHexUint256("0xdeadbeef")
	nft, err := unmarshalNFT(fmt.Sprintf(`{"tokenID":"%s","owner":"%s"}`, tokenID, owner1))
	require.NoError(t, err)
	assert.Equal(t, tokenID, nft.TokenID)
	assert.Equal(t, owner1, nft.Owner)

	_, err = unmarshalNFT("{invalid")
	assert.Error(t, err)

	nft, err = unmarshalNFT("{}")
	require.NoError(t, err)
	assert.Nil(t, nft.TokenID)
	assert.Nil(t, nft.Owner)
}

func TestReceiptState(t *testing.T) {
	ctx := context.Background()

	state, err := receiptState(ctx, coinState(stateID1, owner1, 100, false))
	require.NoError(t, err)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID1), state.ID)
	assert.Equal(t, pldtypes.MustParseBytes32(coinSchemaID), state.Schema)
	assert.JSONEq(t, fmt.Sprintf(`{"salt":"0x1234","owner":"%s","amount":100,"locked":false}`, owner1), state.Data.Pretty())

	_, err = receiptState(ctx, &prototk.EndorsableState{Id: "not-a-hex", SchemaId: coinSchemaID})
	assert.Error(t, err)

	_, err = receiptState(ctx, &prototk.EndorsableState{Id: stateID1, SchemaId: "not-a-bytes32"})
	assert.Error(t, err)
}

func TestBuildFungibleTransfersMultipleSenders(t *testing.T) {
	// Zeto cannot spend coins from more than one owner in a transaction, so we make no claim about
	// what moved rather than reporting something misleading
	inputs := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner1, Amount: pldtypes.Uint64ToUint256(100)},
			{Owner: owner2, Amount: pldtypes.Uint64ToUint256(50)},
		},
	}
	assert.Nil(t, buildFungibleTransfers(context.Background(), inputs, &parsedCoins{}, nil))
}

func TestBuildFungibleTransfersZeroAmount(t *testing.T) {
	// The deposit handler pads its outputs with a zero-value coin
	inputs := &parsedCoins{}
	outputs := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner1, Amount: pldtypes.Uint64ToUint256(100)},
			{Owner: owner2, Amount: pldtypes.Uint64ToUint256(0)},
		},
	}
	transfers := buildFungibleTransfers(context.Background(), inputs, outputs, nil)
	assert.Equal(t, []*types.ReceiptTransfer{
		{To: owner1, Amount: pldtypes.Uint64ToUint256(100)},
	}, transfers)
}

func TestBuildFungibleTransfersDoesNotMutateCoins(t *testing.T) {
	outputs := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner1, Amount: pldtypes.Uint64ToUint256(30)},
			{Owner: owner1, Amount: pldtypes.Uint64ToUint256(20)},
		},
	}
	transfers := buildFungibleTransfers(context.Background(), &parsedCoins{}, outputs, nil)
	require.Len(t, transfers, 2)
	assert.Equal(t, pldtypes.Uint64ToUint256(30), transfers[0].Amount)
	assert.Equal(t, pldtypes.Uint64ToUint256(20), transfers[1].Amount)
	assert.Equal(t, pldtypes.Uint64ToUint256(30), outputs.coins[0].Amount)
	assert.Equal(t, pldtypes.Uint64ToUint256(20), outputs.coins[1].Amount)
}

func TestBuildFungibleTransfersLockedCoinsKeepEntryPositions(t *testing.T) {
	// Locked and unlocked coins share one ordered list, so a locked output does not shift the entry
	// positions the data is matched on
	inputs := &parsedCoins{coins: []*types.ZetoCoin{{Owner: owner1, Amount: pldtypes.Uint64ToUint256(100)}}}
	outputs := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner2, Amount: pldtypes.Uint64ToUint256(40), Locked: true},
			{Owner: owner3, Amount: pldtypes.Uint64ToUint256(60)},
		},
	}
	entryData := []pldtypes.HexBytes{
		pldtypes.MustParseHexBytes("0x01"),
		pldtypes.MustParseHexBytes("0x02"),
	}
	assert.Equal(t, []*types.ReceiptTransfer{
		{From: owner1, To: owner2, Amount: pldtypes.Uint64ToUint256(40), Data: pldtypes.MustParseHexBytes("0x01")},
		{From: owner1, To: owner3, Amount: pldtypes.Uint64ToUint256(60), Data: pldtypes.MustParseHexBytes("0x02")},
	}, buildFungibleTransfers(context.Background(), inputs, outputs, entryData))
}

func TestBuildNonFungibleTransfersSelfTransfer(t *testing.T) {
	tokenID := pldtypes.MustParseHexUint256("0xdeadbeef")
	inputs := &parsedCoins{nfts: []*types.ZetoNFToken{{Owner: owner1, TokenID: tokenID}}}
	outputs := &parsedCoins{nfts: []*types.ZetoNFToken{{Owner: owner1, TokenID: tokenID}}}

	// The token did not change hands, so nothing moved
	assert.Empty(t, buildNonFungibleTransfers(context.Background(), inputs, outputs))
}

func TestBuildNonFungibleTransfersMultipleTokens(t *testing.T) {
	tokenID1 := pldtypes.MustParseHexUint256("0xdeadbeef")
	tokenID2 := pldtypes.MustParseHexUint256("0xcafebabe")
	inputs := &parsedCoins{nfts: []*types.ZetoNFToken{
		{Owner: owner1, TokenID: tokenID1},
		{Owner: owner1, TokenID: tokenID2},
	}}
	// Out of order relative to the inputs, to check the tokens are matched by ID
	outputs := &parsedCoins{nfts: []*types.ZetoNFToken{
		{Owner: owner3, TokenID: tokenID2},
		{Owner: owner2, TokenID: tokenID1},
	}}

	assert.Equal(t, []*types.ReceiptTransfer{
		{From: owner1, To: owner2, TokenID: tokenID1},
		{From: owner1, To: owner3, TokenID: tokenID2},
	}, buildNonFungibleTransfers(context.Background(), inputs, outputs))
}

func TestBuildNonFungibleTransfersMultipleSenders(t *testing.T) {
	inputs := &parsedCoins{nfts: []*types.ZetoNFToken{
		{Owner: owner1, TokenID: pldtypes.MustParseHexUint256("0xdeadbeef")},
		{Owner: owner2, TokenID: pldtypes.MustParseHexUint256("0xcafebabe")},
	}}
	assert.Nil(t, buildNonFungibleTransfers(context.Background(), inputs, &parsedCoins{}))
}

func TestParseCoinList(t *testing.T) {
	z := newReceiptTestZeto()
	ctx := context.Background()

	result, err := z.parseCoinList(ctx, "inputs", []*prototk.EndorsableState{
		coinState(stateID1, owner1, 100, false),
		coinState(stateID2, owner1, 50, true),
		nftState(stateID3, owner1, "0xdeadbeef"),
	})
	require.NoError(t, err)
	// Locked and unlocked coins share one ordered list, so that a coin's position still identifies
	// the transfer entry that produced it
	require.Len(t, result.coins, 2)
	assert.Equal(t, pldtypes.Uint64ToUint256(100), result.coins[0].Amount)
	assert.False(t, result.coins[0].Locked)
	assert.Equal(t, pldtypes.Uint64ToUint256(50), result.coins[1].Amount)
	assert.True(t, result.coins[1].Locked)
	require.Len(t, result.nfts, 1)
	assert.Equal(t, pldtypes.MustParseHexUint256("0xdeadbeef"), result.nfts[0].TokenID)

	// The unlocked coin and the token share the unlocked state list, in request order
	require.Len(t, result.states, 2)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID1), result.states[0].ID)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID3), result.states[1].ID)
	require.Len(t, result.lockedStates, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes(stateID2), result.lockedStates[0].ID)
}

func TestParseCoinListUnexpectedSchema(t *testing.T) {
	z := newReceiptTestZeto()
	_, err := z.parseCoinList(context.Background(), "inputs", []*prototk.EndorsableState{
		{Id: stateID1, SchemaId: unknownSchemaID, StateDataJson: `{}`},
	})
	assert.Regexp(t, "PD210145", err)
}

func TestParseCoinListDuplicateStates(t *testing.T) {
	z := newReceiptTestZeto()
	_, err := z.parseCoinList(context.Background(), "inputs", []*prototk.EndorsableState{
		coinState(stateID1, owner1, 100, false),
		coinState(stateID1, owner1, 50, false),
	})
	assert.Regexp(t, "PD210143", err)
}

func TestParseCoinListInvalidJSON(t *testing.T) {
	z := newReceiptTestZeto()
	_, err := z.parseCoinList(context.Background(), "inputs", []*prototk.EndorsableState{
		{Id: stateID1, SchemaId: coinSchemaID, StateDataJson: `{invalid json}`},
	})
	assert.Regexp(t, "PD210144", err)
}
