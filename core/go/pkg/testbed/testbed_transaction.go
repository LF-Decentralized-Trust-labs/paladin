// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testbed

import (
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type TransactionInput struct {
	From     string             `json:"from"`
	To       tktypes.EthAddress `json:"to"`
	Function abi.Entry          `json:"function,omitempty"`
	Inputs   tktypes.RawJSON    `json:"inputs,omitempty"`
}

type TransactionResult struct {
	EncodedCall         tktypes.HexBytes         `json:"encodedCall"`
	PreparedTransaction *pldapi.TransactionInput `json:"preparedTransaction"`
	PreparedMetadata    tktypes.RawJSON          `json:"preparedMetadata"`
	InputStates         []*pldapi.StateWithData  `json:"inputStates"`
	OutputStates        []*pldapi.StateWithData  `json:"outputStates"`
	ReadStates          []*pldapi.StateWithData  `json:"readStates"`
	InfoStates          []*pldapi.StateWithData  `json:"infoStates"`
	AssembleExtraData   tktypes.RawJSON          `json:"assembleExtraData"` // TODO: remove (only used for finding Pente contract address)
}
