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

type OnChainLocationType int

const (
	// Note numeric order of these is important for sorting
	NotOnChain         OnChainLocationType = 0 // transactions that did not make it on chain
	OnChainTransaction OnChainLocationType = 1 // the transaction is the location
	OnChainEvent       OnChainLocationType = 2 // an individual event within the transaction is the location
)

type OnChainLocation struct {
	Type             OnChainLocationType
	TransactionHash  Bytes32
	BlockNumber      int64
	TransactionIndex int64
	LogIndex         int64       // non-zero only for events
	Source           *EthAddress // non-nil only for events
}

type OnChainLocations []*OnChainLocation

func (l OnChainLocations) Len() int           { return len(l) }
func (l OnChainLocations) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l OnChainLocations) Less(i, j int) bool { return l[i].Compare(l[j]) < 0 }

func (occ *OnChainLocation) Compare(occ2 *OnChainLocation) int {
	if occ.BlockNumber < occ2.BlockNumber {
		return -1
	} else if occ.BlockNumber > occ2.BlockNumber {
		return 1
	}
	// blocks equal
	if occ.TransactionIndex < occ2.TransactionIndex {
		return -1
	} else if occ.TransactionIndex > occ2.TransactionIndex {
		return 1
	}
	// transaction indexes equal
	if occ.Type < occ2.Type {
		return -1
	} else if occ.Type > occ2.Type {
		return 1
	}
	// types equal - events have extra sort dimension
	if occ.Type == OnChainEvent {
		if occ.LogIndex < occ2.LogIndex {
			return -1
		} else if occ.LogIndex > occ2.LogIndex {
			return 1
		}
	}
	// We're equal
	return 0
}
