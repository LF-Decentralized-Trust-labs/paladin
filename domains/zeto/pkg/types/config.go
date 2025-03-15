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

package types

import (
	"context"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
)

// DomainFactoryConfig is the configuration for a Zeto domain
// to provision new domain instances based on a factory contract
// and avalable implementation contracts
type DomainFactoryConfig struct {
	DomainContracts DomainConfigContracts           `json:"domainContracts"`
	SnarkProver     zetosignerapi.SnarkProverConfig `json:"snarkProver"`
}

type DomainConfigContracts struct {
	Implementations []*DomainContract `yaml:"implementations"`
}

type DomainContract struct {
	Name      string `yaml:"name"`
	CircuitId string `yaml:"circuitId"`
}

// func (d *DomainFactoryConfig) GetContractAbi(ctx context.Context, tokenName string) (abi.ABI, error) {
// 	for _, contract := range d.DomainContracts.Implementations {
// 		if contract.Name == tokenName {
// 			var contractAbi abi.ABI
// 			err := json.Unmarshal([]byte(contract.Abi), &contractAbi)
// 			if err != nil {
// 				return nil, err
// 			}
// 			return contractAbi, nil
// 		}
// 	}
// 	return nil, i18n.NewError(ctx, msgs.MsgContractNotFound, tokenName)
// }

func (d *DomainFactoryConfig) GetCircuitId(ctx context.Context, tokenName string) (string, error) {
	for _, contract := range d.DomainContracts.Implementations {
		if contract.Name == tokenName {
			return contract.CircuitId, nil
		}
	}
	return "", i18n.NewError(ctx, msgs.MsgContractNotFound, tokenName)
}

// DomainInstanceConfig is the domain instance config, which are
// sent to the domain contract deployment request to be published
// on-chain. This must include sufficient information for a Paladin
// node to fully initialize the domain instance, based on only
// on-chain information.
type DomainInstanceConfig struct {
	TokenName string `json:"tokenName"`
	CircuitId string `json:"circuitId"`
}

// DomainInstanceConfigABI is the ABI for the DomainInstanceConfig,
// used to encode and decode the on-chain data for the domain config
var DomainInstanceConfigABI = &abi.ParameterArray{
	{Type: "string", Name: "tokenName"},
	{Type: "string", Name: "circuitId"},
}

// marks the version of the Zeto transaction data schema
var ZetoTransactionData_V0 = ethtypes.MustNewHexBytes0xPrefix("0x00010000")

type DomainHandler = domain.DomainHandler[DomainInstanceConfig]
type ParsedTransaction = domain.ParsedTransaction[DomainInstanceConfig]
