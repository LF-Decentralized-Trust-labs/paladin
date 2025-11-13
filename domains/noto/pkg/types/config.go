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
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type DomainConfig struct {
	FactoryVersion int64 `json:"factoryVersion"`
}

var NotoConfigID_V0 = pldtypes.MustParseHexBytes("0x00010000")
var NotoConfigID_V1 = pldtypes.MustParseHexBytes("0x00020000")

// This is the config we expect to receive from the contract registration event
type NotoConfig_V1 struct {
	Name          string              `json:"name,omitempty"`
	Symbol        string              `json:"symbol,omitempty"`
	Decimals      pldtypes.HexUint64  `json:"decimals,omitempty"`
	NotaryAddress pldtypes.EthAddress `json:"notary"`
	Variant       pldtypes.HexUint64  `json:"variant"`
	Data          pldtypes.HexBytes   `json:"data"`
}

var NotoConfigABI_V0 = &abi.ParameterArray{
	{Name: "notary", Type: "address"},
	{Name: "variant", Type: "bytes32"},
	{Name: "data", Type: "bytes"},
}

var NotoConfigABI_V1 = &abi.ParameterArray{
	{Name: "name", Type: "string"},
	{Name: "symbol", Type: "string"},
	{Name: "decimals", Type: "uint8"},
	{Name: "notary", Type: "address"},
	{Name: "variant", Type: "bytes32"},
	{Name: "data", Type: "bytes"},
}

var NotoTransactionDataID_V0 = pldtypes.MustParseHexBytes("0x00010000")

// This is the structure we expect to unpack from the config data
type NotoConfigData_V0 struct {
	NotaryLookup   string               `json:"notaryLookup"`
	NotaryMode     pldtypes.HexUint64   `json:"notaryMode"`
	PrivateAddress *pldtypes.EthAddress `json:"privateAddress"`
	PrivateGroup   *PentePrivateGroup   `json:"privateGroup"`
	RestrictMint   bool                 `json:"restrictMint"`
	AllowBurn      bool                 `json:"allowBurn"`
	AllowLock      bool                 `json:"allowLock"`
}

type NotoTransactionData_V0 struct {
	TransactionID pldtypes.Bytes32   `json:"transactionId"`
	InfoStates    []pldtypes.Bytes32 `json:"infoStates"`
}

// This is the structure we parse the config into in InitConfig and gets passed back to us on every call
type NotoParsedConfig struct {
	Name         string                    `json:"name"`
	Symbol       string                    `json:"symbol"`
	Decimals     pldtypes.HexUint64        `json:"decimals"`
	NotaryLookup string                    `json:"notaryLookup"`
	NotaryMode   pldtypes.Enum[NotaryMode] `json:"notaryMode"`
	Variant      pldtypes.HexUint64        `json:"variant"`
	IsNotary     bool                      `json:"isNotary"`
	Options      NotoOptions               `json:"options"`
}

type NotoOptions struct {
	Basic *NotoBasicOptions `json:"basic,omitempty"`
	Hooks *NotoHooksOptions `json:"hooks,omitempty"`
}

type NotoBasicOptions struct {
	RestrictMint *bool `json:"restrictMint"` // Only allow notary to mint (default: true)
	AllowBurn    *bool `json:"allowBurn"`    // Allow token holders to burn their tokens (default: true)
	AllowLock    *bool `json:"allowLock"`    // Allow token holders to lock their tokens (default: true)
}

type NotoHooksOptions struct {
	PublicAddress     *pldtypes.EthAddress `json:"publicAddress"`               // Public address of the Pente privacy group
	PrivateGroup      *PentePrivateGroup   `json:"privateGroup,omitempty"`      // Details on the Pente privacy group
	PrivateAddress    *pldtypes.EthAddress `json:"privateAddress,omitempty"`    // Private address of the hook contract deployed within the privacy group
	DevUsePublicHooks bool                 `json:"devUsePublicHooks,omitempty"` // Use a public hooks contract - insecure, for dev purposes only! (privateGroup/privateAddress are ignored)
}

type PentePrivateGroup struct {
	Salt    pldtypes.Bytes32 `json:"salt"`
	Members []string         `json:"members"`
}

var NotoTransactionDataABI_V0 = &abi.ParameterArray{
	{Name: "transactionId", Type: "bytes32"},
	{Name: "infoStates", Type: "bytes32[]"},
}

type DomainHandler = domain.DomainHandler[NotoParsedConfig]
type DomainCallHandler = domain.DomainCallHandler[NotoParsedConfig]
type ParsedTransaction = domain.ParsedTransaction[NotoParsedConfig]

const (
	NotaryModeIntBasic pldtypes.HexUint64 = 0x0000
	NotaryModeIntHooks pldtypes.HexUint64 = 0x0001
)

var NotoVariantDefault pldtypes.HexUint64 = 0x0000
