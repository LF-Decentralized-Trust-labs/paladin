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
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReceiptTransfers(t *testing.T) {
	n := &Noto{
		coinSchema:       &prototk.StateSchema{Id: "coin"},
		lockedCoinSchema: &prototk.StateSchema{Id: "lockedCoin"},
	}
	ctx := context.Background()

	transfers, err := n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates:  []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{},
	})
	require.NoError(t, err)
	require.Len(t, transfers, 0)

	owner1 := pldtypes.MustEthAddress("0xbb2b99dde4ca2d4c99f149d13cd55a9edada69eb")
	owner2 := pldtypes.MustEthAddress("0x3008ee73a70cd1cc57647c7d253a48defe86dd9b")
	owner3 := pldtypes.MustEthAddress("0xea82df8cb94b9fa1a0a711d75a7f7dfdec2ef53c")

	// Simple mint
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   nil,
		To:     owner1,
		Amount: pldtypes.Int64ToInt256(1),
	}}, transfers)

	// Simple burn
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     nil,
		Amount: pldtypes.Int64ToInt256(1),
	}}, transfers)

	// Burn with returned remainder
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 10,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "2",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 8,
				"owner": "%s"
			}`, owner1),
		}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     nil,
		Amount: pldtypes.Int64ToInt256(2),
	}}, transfers)

	// Simple transfer
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "2",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner2),
		}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     owner2,
		Amount: pldtypes.Int64ToInt256(1),
	}}, transfers)

	// Unlock to multiple recipients, with locked remainder
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: "lockedCoin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 10,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "2",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner2),
		}, {
			Id:       "3",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner3),
		}, {
			Id:       "4",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner3),
		}, {
			Id:       "5",
			SchemaId: "lockedCoin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 7,
				"owner": "%s"
			}`, owner1),
		}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     owner2,
		Amount: pldtypes.Int64ToInt256(1),
	}, {
		From:   owner1,
		To:     owner3,
		Amount: pldtypes.Int64ToInt256(2),
	}}, transfers)
}
