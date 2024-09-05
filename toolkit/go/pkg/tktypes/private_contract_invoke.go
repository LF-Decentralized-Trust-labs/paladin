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

package tktypes

import (
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// Specification for the invocation of a private smart contract
type PrivateContractInvoke struct {
	From     string     `json:"from"`               // the authorizing identity for this transaction
	To       EthAddress `json:"to"`                 // the private smart contract to invoke (must already have been deployed and indexed)
	Function abi.Entry  `json:"function,omitempty"` // ABI definition of the function to invoke
	Inputs   RawJSON    `json:"inputs,omitempty"`   // JSON encoded inputs - which will be validated against the function spec
}
