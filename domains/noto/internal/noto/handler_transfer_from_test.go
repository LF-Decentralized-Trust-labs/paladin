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
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransferFromBasicModeRestriction(t *testing.T) {
	n := &Noto{
		Callbacks:  mockCallbacks,
		coinSchema: &prototk.StateSchema{Id: "coin"},
		dataSchema: &prototk.StateSchema{Id: "data"},
	}
	ctx := context.Background()

	// Test that transferFrom is not allowed in basic mode
	basicConfig := &types.NotoParsedConfig{
		NotaryMode:   types.NotaryModeBasic.Enum(),
		NotaryLookup: "notary@node1",
		Options: types.NotoOptions{
			Basic: &types.NotoBasicOptions{
				AllowBurn: &pTrue,
			},
		},
	}

	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
			ContractConfigJson: mustParseJSON(basicConfig),
		},
		FunctionAbiJson:   mustParseJSON(types.NotoABI.Functions()["transferFrom"]),
		FunctionSignature: types.NotoABI.Functions()["transferFrom"].SolString(),
		FunctionParamsJson: `{
			"from": "from@node1",
			"to": "to@node2",
			"amount": 50,
			"data": "0x1234"
		}`,
	}

	_, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TransferFrom is not enabled")
}

func TestTransferFromHooksModeAllowed(t *testing.T) {
	n := &Noto{
		Callbacks:  mockCallbacks,
		coinSchema: &prototk.StateSchema{Id: "coin"},
		dataSchema: &prototk.StateSchema{Id: "data"},
	}
	ctx := context.Background()

	// Test that transferFrom is allowed in hooks mode
	hooksConfig := &types.NotoParsedConfig{
		NotaryMode:   types.NotaryModeHooks.Enum(),
		NotaryLookup: "notary@node1",
		Options: types.NotoOptions{
			Hooks: &types.NotoHooksOptions{
				PublicAddress: &pldtypes.EthAddress{},
			},
		},
	}

	tx := &prototk.TransactionSpecification{
		TransactionId: "0x015e1881f2ba769c22d05c841f06949ec6e1bd573f5e1e0328885494212f077d",
		From:          "sender@node1",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
			ContractConfigJson: mustParseJSON(hooksConfig),
		},
		FunctionAbiJson:   mustParseJSON(types.NotoABI.Functions()["transferFrom"]),
		FunctionSignature: types.NotoABI.Functions()["transferFrom"].SolString(),
		FunctionParamsJson: `{
			"from": "from@node1",
			"to": "to@node2",
			"amount": 50,
			"data": "0x1234"
		}`,
	}

	initRes, err := n.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: tx,
	})
	require.NoError(t, err)
	require.Len(t, initRes.RequiredVerifiers, 4)
	assert.Equal(t, "notary@node1", initRes.RequiredVerifiers[0].Lookup)
	assert.Equal(t, "sender@node1", initRes.RequiredVerifiers[1].Lookup)
	assert.Equal(t, "from@node1", initRes.RequiredVerifiers[2].Lookup)
	assert.Equal(t, "to@node2", initRes.RequiredVerifiers[3].Lookup)
}
