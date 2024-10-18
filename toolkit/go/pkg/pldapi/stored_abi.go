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

package pldapi

import (
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// This is a very compact wrapping structure that is automatically stored for any ABI
// used in a transaction. A deterministic hashing is performed of the ABI that preserves
// all the details including parameter names, indexed flags, errors, event etc.
// (see tktypes.ABISolDefinitionHash()).
//
// In the future a _separate_ metadata object might be added to allow CRUD style storage
// and query of associated ABI details, like devDocs, contract name, times etc.
// However, this record is intended to stay unchanged and deliberately thin
type StoredABI struct {
	Hash tktypes.Bytes32 `json:"hash,omitempty"`
	ABI  abi.ABI         `json:"abi,omitempty"`
}
