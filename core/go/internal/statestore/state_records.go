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
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// State record can be updated before, during and after confirm records are written
// For example the confirmation of the existence of states will be coming all the time
// from the base ledger, for which we will never receive the private state itself.
// Immutable once written
type StateConfirm struct {
	DomainName  string           `json:"domain"       gorm:"primaryKey"`
	State       tktypes.HexBytes `json:"-"            gorm:"primaryKey"`
	Transaction uuid.UUID        `json:"transaction"`
}

// State record can be updated before, during and after spend records are written
// Immutable once written
type StateSpend struct {
	DomainName  string           `json:"domain"       gorm:"primaryKey"`
	State       tktypes.HexBytes `json:"-"            gorm:"primaryKey"`
	Transaction uuid.UUID        `json:"transaction"`
}

// State locks record which transaction a state is being locked to, either
// spending a previously confirmed state, or an optimistic record of creating
// (and maybe later spending) a state that is yet to be confirmed.
type StateLock struct {
	DomainName  string           `json:"domain"       gorm:"primaryKey"`
	State       tktypes.HexBytes `json:"-"            gorm:"primaryKey"`
	Transaction uuid.UUID        `json:"transaction"`
	Creating    bool             `json:"creating"`
	Spending    bool             `json:"spending"`
}

// State nullifiers are used when a domain chooses to use a separate identifier
// specifically for spending states (i.e. not the state ID).
// Domains that choose to leverage this architecture will create nullifier
// entries for all unspent states, and create a StateSpend entry for the
// nullifier (not for the state) when it is spent.
// Immutable once written
type StateNullifier struct {
	DomainName string           `json:"domain"          gorm:"primaryKey"`
	Nullifier  tktypes.HexBytes `json:"nullifier"       gorm:"primaryKey"`
	State      tktypes.HexBytes `json:"-"`
	Spent      *StateSpend      `json:"spent,omitempty" gorm:"foreignKey:state;references:nullifier;"`
}
