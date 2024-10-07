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
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortOnChainLocations(t *testing.T) {

	ocls := OnChainLocations{
		{Type: OnChainTransaction, BlockNumber: 3000, TransactionIndex: 10},
		{Type: OnChainEvent, BlockNumber: 2000, TransactionIndex: 10, LogIndex: 10},
		{Type: OnChainTransaction, BlockNumber: 2000, TransactionIndex: 20},
		{Type: OnChainTransaction, BlockNumber: 2000, TransactionIndex: 10},
		{Type: OnChainEvent, BlockNumber: 2000, TransactionIndex: 10, LogIndex: 0},
		{Type: OnChainTransaction, BlockNumber: 2000, TransactionIndex: 0},
		{Type: OnChainTransaction, BlockNumber: 4000, TransactionIndex: 1},
		{Type: OnChainEvent, BlockNumber: 2000, TransactionIndex: 10, LogIndex: 20},
		{Type: OnChainTransaction, BlockNumber: 4000, TransactionIndex: 0},
		{Type: OnChainEvent, BlockNumber: 1000, TransactionIndex: 0, LogIndex: 50},
		{Type: OnChainTransaction, BlockNumber: 2000, TransactionIndex: 0}, // duplicate
	}
	sort.Sort(ocls)

	assert.Equal(t, OnChainLocations{
		{Type: OnChainEvent, BlockNumber: 1000, TransactionIndex: 0, LogIndex: 50},
		{Type: OnChainTransaction, BlockNumber: 2000, TransactionIndex: 0}, // duplicate
		{Type: OnChainTransaction, BlockNumber: 2000, TransactionIndex: 0},
		{Type: OnChainTransaction, BlockNumber: 2000, TransactionIndex: 10},
		{Type: OnChainEvent, BlockNumber: 2000, TransactionIndex: 10, LogIndex: 0},
		{Type: OnChainEvent, BlockNumber: 2000, TransactionIndex: 10, LogIndex: 10},
		{Type: OnChainEvent, BlockNumber: 2000, TransactionIndex: 10, LogIndex: 20},
		{Type: OnChainTransaction, BlockNumber: 2000, TransactionIndex: 20},
		{Type: OnChainTransaction, BlockNumber: 3000, TransactionIndex: 10},
		{Type: OnChainTransaction, BlockNumber: 4000, TransactionIndex: 0},
		{Type: OnChainTransaction, BlockNumber: 4000, TransactionIndex: 1},
	}, ocls)

}
