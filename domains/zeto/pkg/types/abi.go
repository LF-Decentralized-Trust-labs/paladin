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

	"github.com/kaleido-io/paladin/toolkit/pkg/solutils"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

//go:embed abis/IZetoFungible.json
var zetoFungibleJSON []byte

//go:embed abis/IZetoNonFungible.json
var zetoNonFungibleJSON []byte

var ZetoFungibleABI = solutils.MustParseBuildABI(zetoFungibleJSON)

var ZetoNonFungibleABI = solutils.MustParseBuildABI(zetoNonFungibleJSON)

const (
	METHOD_MINT            = "mint"
	METHOD_TRANSFER        = "transfer"
	METHOD_TRANSFER_LOCKED = "transferLocked"
	METHOD_LOCK            = "lock"
	METHOD_DEPOSIT         = "deposit"
	METHOD_WITHDRAW        = "withdraw"
)

type InitializerParams struct {
	TokenName string `json:"tokenName"`
	// InitialOwner string `json:"initialOwner"` // TODO: allow the initial owner to be specified by the deploy request
}

type DeployParams struct {
	TransactionID string           `json:"transactionId"`
	Data          tktypes.HexBytes `json:"data"`
	TokenName     string           `json:"tokenName"`
	InitialOwner  string           `json:"initialOwner"`
	IsNonFungible bool             `json:"isNonFungible"`
}

type NonFungibleMintParams struct {
	Mints []*NonFungibleTransferParamEntry `json:"mints"`
}

type FungibleMintParams struct {
	Mints []*FungibleTransferParamEntry `json:"mints"`
}
type FungibleTransferParams struct {
	Transfers []*FungibleTransferParamEntry `json:"transfers"`
}

type FungibleTransferParamEntry struct {
	To     string              `json:"to"`
	Amount *tktypes.HexUint256 `json:"amount"`
}

type FungibleTransferLockedParams struct {
	LockedInputs []*tktypes.HexUint256         `json:"lockedInputs"`
	Delegate     string                        `json:"delegate"`
	Transfers    []*FungibleTransferParamEntry `json:"transfers"`
}

type NonFungibleTransferParams struct {
	Transfers []*NonFungibleTransferParamEntry `json:"transfers"`
}

type NonFungibleTransferParamEntry struct {
	To      string              `json:"to"`
	URI     string              `json:"uri,omitempty"`
	TokenID *tktypes.HexUint256 `json:"tokenID"`
}

type LockParams struct {
	Amount   *tktypes.HexUint256 `json:"amount"`
	Delegate *tktypes.EthAddress `json:"delegate"`
}

type DepositParams struct {
	Amount *tktypes.HexUint256 `json:"amount"`
}

type WithdrawParams struct {
	Amount *tktypes.HexUint256 `json:"amount"`
}
