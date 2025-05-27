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
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
)

type TransactionResult struct {
	ID                  uuid.UUID                `json:"id"`
	EncodedCall         pldtypes.HexBytes        `json:"encodedCall"`
	PreparedTransaction *pldapi.TransactionInput `json:"preparedTransaction"`
	PreparedMetadata    pldtypes.RawJSON         `json:"preparedMetadata"`
	InputStates         []*pldapi.StateEncoded   `json:"inputStates"`
	OutputStates        []*pldapi.StateEncoded   `json:"outputStates"`
	ReadStates          []*pldapi.StateEncoded   `json:"readStates"`
	InfoStates          []*pldapi.StateEncoded   `json:"infoStates"`
	DomainReceipt       pldtypes.RawJSON         `json:"domainReceipt"`
}
