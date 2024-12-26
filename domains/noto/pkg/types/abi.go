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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/solutils"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

//go:embed abis/INotoPrivate.json
var notoPrivateJSON []byte

var NotoABI = solutils.MustParseBuildABI(notoPrivateJSON)

type ConstructorParams struct {
	Notary         string      `json:"notary"`                   // Lookup string for the notary identity
	Implementation string      `json:"implementation,omitempty"` // Use a specific implementation of Noto that was registered to the factory (blank to use default)
	Hooks          *HookParams `json:"hooks,omitempty"`          // Configure hooks for programmable logic around Noto operations
	RestrictMint   *bool       `json:"restrictMint,omitempty"`   // Only allow notary to mint (default: true)
	AllowBurn      *bool       `json:"allowBurn,omitempty"`      // Allow token holders to burn their tokens (default: true)
}

// Currently the only supported hooks are provided via a Pente private smart contract
type HookParams struct {
	PrivateGroup   *PentePrivateGroup  `json:"privateGroup,omitempty"`   // Details on a Pente privacy group
	PublicAddress  *tktypes.EthAddress `json:"publicAddress,omitempty"`  // Public address of the Pente privacy group
	PrivateAddress *tktypes.EthAddress `json:"privateAddress,omitempty"` // Private address of the hook contract deployed within the privacy group
}

type MintParams struct {
	To     string              `json:"to"`
	Amount *tktypes.HexUint256 `json:"amount"`
	Data   tktypes.HexBytes    `json:"data"`
}

type TransferParams struct {
	To     string              `json:"to"`
	Amount *tktypes.HexUint256 `json:"amount"`
	Data   tktypes.HexBytes    `json:"data"`
}

type BurnParams struct {
	Amount *tktypes.HexUint256 `json:"amount"`
	Data   tktypes.HexBytes    `json:"data"`
}

type ApproveParams struct {
	Inputs   []*pldapi.StateEncoded `json:"inputs"`
	Outputs  []*pldapi.StateEncoded `json:"outputs"`
	Data     tktypes.HexBytes       `json:"data"`
	Delegate *tktypes.EthAddress    `json:"delegate"`
}

type LockParams struct {
	LockID tktypes.Bytes32     `json:"lockId"`
	Amount *tktypes.HexUint256 `json:"amount"`
	Data   tktypes.HexBytes    `json:"data"`
}

type UnlockParams struct {
	LockID  tktypes.Bytes32       `json:"lockId"`
	To      []string              `json:"to"`
	Amounts []*tktypes.HexUint256 `json:"amounts"`
	Data    tktypes.HexBytes      `json:"data"`
}

type ApproveExtraParams struct {
	Data tktypes.HexBytes `json:"data"`
}

type NotoPublicTransaction struct {
	FunctionABI *abi.Entry       `json:"functionABI"`
	ParamsJSON  tktypes.RawJSON  `json:"paramsJSON"`
	EncodedCall tktypes.HexBytes `json:"encodedCall"`
}

type NotoTransferMetadata struct {
	ApprovalParams       ApproveExtraParams    `json:"approvalParams"`       // Partial set of params that can be passed to the "approveTransfer" method to approve another party to perform this transfer
	TransferWithApproval NotoPublicTransaction `json:"transferWithApproval"` // The public transaction that would need to be submitted by an approved party to perform this transfer
}
