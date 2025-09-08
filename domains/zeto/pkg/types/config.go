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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
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
	Name     string                  `yaml:"name"`
	Circuits *zetosignerapi.Circuits `yaml:"circuits"`
}

func (d *DomainFactoryConfig) GetCircuits(ctx context.Context, tokenName string) (*zetosignerapi.Circuits, error) {
	for _, contract := range d.DomainContracts.Implementations {
		if contract.Name == tokenName {
			return contract.Circuits, nil
		}
	}
	return nil, i18n.NewError(ctx, msgs.MsgContractNotFound, tokenName)
}

func (d *DomainFactoryConfig) GetCircuit(ctx context.Context, tokenName, method string) (*zetosignerapi.Circuit, error) {
	for _, contract := range d.DomainContracts.Implementations {
		if contract.Name == tokenName {
			return (*contract.Circuits)[method], nil
		}
	}
	return nil, i18n.NewError(ctx, msgs.MsgContractNotFound, tokenName)
}

// DomainInstanceConfig is the domain instance config, which are
// sent to the domain contract deployment request to be published
// on-chain. This must include sufficient information for a Paladin
// node to fully initialize the domain instance, based on only
// on-chain information.
type DomainInstanceConfig struct {
	TokenName string                  `json:"tokenName"`
	Circuits  *zetosignerapi.Circuits `json:"circuits"`
}

// DomainInstanceConfigABI is the ABI for the DomainInstanceConfig,
// used to encode and decode the on-chain data for the domain config
var DomainInstanceConfigABI = &abi.ParameterArray{
	{
		Type: "string",
		Name: "tokenName",
	},
	{
		Type: "tuple",
		Name: "circuits",
		Components: []*abi.Parameter{
			{Type: "tuple", Name: "deposit", Components: []*abi.Parameter{{Type: "string", Name: "name"}, {Type: "string", Name: "type"}, {Type: "bool", Name: "usesEncryption"}, {Type: "bool", Name: "usesNullifiers"}, {Type: "bool", Name: "usesKyc"}}},
			{Type: "tuple", Name: "withdraw", Components: []*abi.Parameter{{Type: "string", Name: "name"}, {Type: "string", Name: "type"}, {Type: "bool", Name: "usesEncryption"}, {Type: "bool", Name: "usesNullifiers"}, {Type: "bool", Name: "usesKyc"}}},
			{Type: "tuple", Name: "transfer", Components: []*abi.Parameter{{Type: "string", Name: "name"}, {Type: "string", Name: "type"}, {Type: "bool", Name: "usesEncryption"}, {Type: "bool", Name: "usesNullifiers"}, {Type: "bool", Name: "usesKyc"}}},
			{Type: "tuple", Name: "transferLocked", Components: []*abi.Parameter{{Type: "string", Name: "name"}, {Type: "string", Name: "type"}, {Type: "bool", Name: "usesEncryption"}, {Type: "bool", Name: "usesNullifiers"}, {Type: "bool", Name: "usesKyc"}}},
		},
	},
}

// marks the version of the Zeto transaction data schema
var ZetoTransactionDataID_V0 = ethtypes.MustNewHexBytes0xPrefix("0x00010000")

type ZetoTransactionData_V0 struct {
	TransactionID pldtypes.Bytes32   `json:"transactionId"`
	InfoStates    []pldtypes.Bytes32 `json:"infoStates"`
}

var ZetoTransactionDataABI_V0 = &abi.ParameterArray{
	{Name: "transactionId", Type: "bytes32"},
	{Name: "infoStates", Type: "bytes32[]"},
}

type DomainHandler = domain.DomainHandler[DomainInstanceConfig]
type DomainCallHandler = domain.DomainCallHandler[DomainInstanceConfig]
type ParsedTransaction = domain.ParsedTransaction[DomainInstanceConfig]
