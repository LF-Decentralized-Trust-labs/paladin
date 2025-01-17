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
		"restrictMinting": false,
		"allowBurning": false,
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
