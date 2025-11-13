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
	_ "embed"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
)

//go:embed abis/INotoPrivate.json
var notoPrivateJSON []byte

var NotoABI = solutils.MustParseBuildABI(notoPrivateJSON)

type ConstructorParams struct {
	Name           string      `json:"name,omitempty"`           // Name of the token
	Symbol         string      `json:"symbol,omitempty"`         // Symbol of the token
	Notary         string      `json:"notary"`                   // Lookup string for the notary identity
	NotaryMode     NotaryMode  `json:"notaryMode"`               // Notary mode (basic or hooks)
	Implementation string      `json:"implementation,omitempty"` // Use a specific implementation of Noto that was registered to the factory (blank to use default)
	Options        NotoOptions `json:"options"`                  // Configure options for the chosen notary mode
}

type NotaryMode string

const (
	NotaryModeBasic NotaryMode = "basic"
	NotaryModeHooks NotaryMode = "hooks"
)

func (tt NotaryMode) Enum() pldtypes.Enum[NotaryMode] {
	return pldtypes.Enum[NotaryMode](tt)
}

func (tt NotaryMode) Options() []string {
	return []string{
		string(NotaryModeBasic),
		string(NotaryModeHooks),
	}
}

type MintParams struct {
	To     string               `json:"to"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type BurnParams struct {
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type BurnFromParams struct {
	From   string               `json:"from"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type TransferParams struct {
	To     string               `json:"to"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type TransferFromParams struct {
	From   string               `json:"from"`
	To     string               `json:"to"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type ApproveParams struct {
	Inputs   []*pldapi.StateEncoded `json:"inputs"`
	Outputs  []*pldapi.StateEncoded `json:"outputs"`
	Data     pldtypes.HexBytes      `json:"data"`
	Delegate *pldtypes.EthAddress   `json:"delegate"`
}

type LockParams struct {
	Amount *pldtypes.HexUint256 `json:"amount"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type UnlockParams struct {
	LockID     pldtypes.Bytes32   `json:"lockId"`
	From       string             `json:"from"`
	Recipients []*UnlockRecipient `json:"recipients"`
	Data       pldtypes.HexBytes  `json:"data"`
}

type DelegateLockParams struct {
	LockID   pldtypes.Bytes32     `json:"lockId"`
	Unlock   *UnlockPublicParams  `json:"unlock"`
	Delegate *pldtypes.EthAddress `json:"delegate"`
	Data     pldtypes.HexBytes    `json:"data"`
}

type UnlockRecipient struct {
	To     string               `json:"to"`
	Amount *pldtypes.HexUint256 `json:"amount"`
}

type UnlockPublicParams struct {
	TxId          string            `json:"txId"`
	LockedInputs  []string          `json:"lockedInputs"`
	LockedOutputs []string          `json:"lockedOutputs"`
	Outputs       []string          `json:"outputs"`
	Signature     pldtypes.HexBytes `json:"signature"`
	Data          pldtypes.HexBytes `json:"data"`
}

type BalanceOfParam struct {
	Account string `json:"account"`
}

type BalanceOfResult struct {
	TotalBalance *pldtypes.HexUint256 `json:"totalBalance"`
	TotalStates  *pldtypes.HexUint256 `json:"totalStates"`
	Overflow     bool                 `json:"overflow"`
}
