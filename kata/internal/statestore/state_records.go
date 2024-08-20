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

package statestore

import (
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

// State record can be updated before, during and after confirm records are written
// For example the confirmation of the existence of states will be coming all the time
// from the base ledger, for which we will never receive the private state itself.
// Immutable once written
type StateConfirm struct {
	State       types.Bytes32 `json:"-"            gorm:"primaryKey"`
	Transaction uuid.UUID     `json:"transaction"`
}

// State record can be updated before, during and after spend records are written
// Immutable once written
type StateSpend struct {
	State       types.Bytes32 `json:"-"            gorm:"primaryKey"`
	Transaction uuid.UUID     `json:"transaction"`
}

// State locks record which sequence a state is being locked to, either
// spending a previously confirmed state, or an optimistic record of creating
// (and maybe later spending) a state that is yet to be confirmed.
type StateLock struct {
	State    types.Bytes32 `json:"-"                gorm:"primaryKey"`
	Sequence uuid.UUID     `json:"sequence"`
	Creating bool          `json:"creating"`
	Spending bool          `json:"spending"`
}
