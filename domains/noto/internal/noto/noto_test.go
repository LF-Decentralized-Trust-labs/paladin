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

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

// ABI encoded config:
// types.NotoConfigInput_V0{NotaryLookup: "notary"})
var encodedConfig = ethtypes.MustNewHexBytes0xPrefix("0x00010000000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000066e6f746172790000000000000000000000000000000000000000000000000000")

func TestConfigureDomainBadConfig(t *testing.T) {
	n := &Noto{}
	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: "!!wrong",
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitDeployBadParams(t *testing.T) {
	n := &Noto{}
	_, err := n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestPrepareDeployBadParams(t *testing.T) {
	n := &Noto{}
	_, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestPrepareDeployMissingVerifier(t *testing.T) {
	n := &Noto{}
	_, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: "{}",
		},
	})
	assert.ErrorContains(t, err, "PD200011")
}

func TestInitTransactionBadAbi(t *testing.T) {
	n := &Noto{}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitTransactionBadFunction(t *testing.T) {
	n := &Noto{}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractConfig:  encodedConfig,
			FunctionAbiJson: `{"name": "does-not-exist"}`,
		},
	})
	assert.ErrorContains(t, err, "PD200001")
}

func TestInitTransactionBadParams(t *testing.T) {
	n := &Noto{}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractConfig:     encodedConfig,
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitTransactionMissingTo(t *testing.T) {
	n := &Noto{}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractConfig:     encodedConfig,
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: "{}",
		},
	})
	assert.ErrorContains(t, err, "PD200007")
}

func TestInitTransactionMissingAmount(t *testing.T) {
	n := &Noto{}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractConfig:     encodedConfig,
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: `{"to": "recipient"}`,
		},
	})
	assert.ErrorContains(t, err, "PD200008")
}

func TestInitTransactionBadSignature(t *testing.T) {
	n := &Noto{}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractConfig:     encodedConfig,
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: `{"to": "recipient", "amount": 1}`,
		},
	})
	assert.ErrorContains(t, err, "PD200002")
}

func TestAssembleTransactionBadAbi(t *testing.T) {
	n := &Noto{}
	_, err := n.AssembleTransaction(context.Background(), &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestEndorseTransactionBadAbi(t *testing.T) {
	n := &Noto{}
	_, err := n.EndorseTransaction(context.Background(), &prototk.EndorseTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestPrepareTransactionBadAbi(t *testing.T) {
	n := &Noto{}
	_, err := n.PrepareTransaction(context.Background(), &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}
