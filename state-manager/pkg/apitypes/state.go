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

package apitypes

import (
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
)

type StateState = fftypes.FFEnum

var (
	StateProposed  = fftypes.FFEnumValue("statestate", "proposed")
	StateConfirmed = fftypes.FFEnumValue("statestate", "confirmed")
	StateReserved  = fftypes.FFEnumValue("statestate", "reserved")
	StateConsumed  = fftypes.FFEnumValue("statestate", "consumed")
)

type State struct {
	ID      *string         `ffstruct:"ResourceBase" json:"id"`
	Created *fftypes.FFTime `ffstruct:"ResourceBase" json:"created"`
	Updated *fftypes.FFTime `ffstruct:"ResourceBase" json:"updated"`
	State   *StateState     `ffstruct:"State" json:"state"`
}

func (r *State) GetID() string {
	if r.ID == nil {
		return ""
	}
	return *r.ID
}

func (r *State) SetCreated(t *fftypes.FFTime) {
	r.Created = t
}

func (r *State) SetUpdated(t *fftypes.FFTime) {
	r.Updated = t
}

var StateFilters = &ffapi.QueryFields{}
