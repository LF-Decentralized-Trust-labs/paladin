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

package privatetxnmgr

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentionResolver_2TransactionsDeterministicResults(t *testing.T) {
	t.Skip("this test should be run manually when the algorithm is changed. ")
	// see https://github.com/LF-Decentralized-Trust-labs/paladin/pull/145 for background

	// create 2 ids at random (representing bidding transactions),
	// then iterate over 100 random state ids and check that there is a fair distribution of winners
	winner1 := 0
	winner2 := 0
	biddingTransaction1 := uuid.New().String()
	biddingTransaction2 := uuid.New().String()

	resolver := NewContentionResolver()

	for i := 0; i < 1000; i++ {
		stateID := uuid.New().String()
		winner, err := resolver.Resolve(stateID, biddingTransaction1, biddingTransaction2)
		require.NoError(t, err)
		assert.Contains(t, []string{biddingTransaction1, biddingTransaction2}, winner)
		if winner == biddingTransaction1 {
			winner1++
		} else {
			winner2++
		}

	}

	assert.InDelta(t, winner2, winner1, 100)
}

func TestContentionResolver_CommutativeProperty(t *testing.T) {
	// create 2 ids at random (representing bidding transactions),
	// then iterate over 100 random state ids and check that it is always the case that the winner is the same regardless of the order of invocation
	biddingTransaction1 := uuid.New().String()
	biddingTransaction2 := uuid.New().String()
	resolver := NewContentionResolver()

	for i := 0; i < 100; i++ {
		stateID := uuid.New().String()
		winner1, err := resolver.Resolve(stateID, biddingTransaction1, biddingTransaction2)
		require.NoError(t, err)
		winner2, err := resolver.Resolve(stateID, biddingTransaction2, biddingTransaction1)
		require.NoError(t, err)
		assert.Equal(t, winner1, winner2)
	}
}

func TestContentionResolver_AssociativeProperty(t *testing.T) {
	// create 4 ids at random (representing bidding transactions),
	// then iterate over 10 random state ids and for each one, run each permutation of ordering of invocations and check that
	// there are 2 types of permutation a) the knockout tournament format (semi finals -> final) and b) the winner stays on format
	// the final winners are always the same and that there is a fair distribution of winners.
	resolver := NewContentionResolver()

	bidders := make([]string, 4)
	for i := 0; i < 4; i++ {
		bidders[i] = uuid.New().String()
	}
	knockOutDraw1 := []string{bidders[0], bidders[1], bidders[2], bidders[3]} // equvalent to 1,0,3,2 and 1,0,2,3 and 0,1,3,2 as per proven in the TestContentionResolver_CommutativeProperty test
	knockOutDraw2 := []string{bidders[0], bidders[2], bidders[1], bidders[3]} //
	knockOutDraw3 := []string{bidders[0], bidders[3], bidders[1], bidders[2]} //

	// Generate all possible orders of bidders
	orders := [][]string{
		{bidders[0], bidders[1], bidders[2], bidders[3]},
		{bidders[0], bidders[1], bidders[3], bidders[2]},
		{bidders[0], bidders[2], bidders[1], bidders[3]},
		{bidders[0], bidders[2], bidders[3], bidders[1]},
		{bidders[0], bidders[3], bidders[1], bidders[2]},
		{bidders[0], bidders[3], bidders[2], bidders[1]},
		{bidders[1], bidders[0], bidders[2], bidders[3]},
		{bidders[1], bidders[0], bidders[3], bidders[2]},
		{bidders[1], bidders[2], bidders[0], bidders[3]},
		{bidders[1], bidders[2], bidders[3], bidders[0]},
		{bidders[1], bidders[3], bidders[0], bidders[2]},
		{bidders[1], bidders[3], bidders[2], bidders[0]},
		{bidders[2], bidders[0], bidders[1], bidders[3]},
		{bidders[2], bidders[0], bidders[3], bidders[1]},
		{bidders[2], bidders[1], bidders[0], bidders[3]},
		{bidders[2], bidders[1], bidders[3], bidders[0]},
		{bidders[2], bidders[3], bidders[0], bidders[1]},
		{bidders[2], bidders[3], bidders[1], bidders[0]},
		{bidders[3], bidders[0], bidders[1], bidders[2]},
		{bidders[3], bidders[0], bidders[2], bidders[1]},
		{bidders[3], bidders[1], bidders[0], bidders[2]},
		{bidders[3], bidders[1], bidders[2], bidders[0]},
		{bidders[3], bidders[2], bidders[0], bidders[1]},
		{bidders[3], bidders[2], bidders[1], bidders[0]},
	}
	runWinnerStaysOn := func(draw []string, stateID string) string {
		winner1, err := resolver.Resolve(stateID, draw[0], draw[1])
		require.NoError(t, err)

		winner2, err := resolver.Resolve(stateID, winner1, draw[2])
		require.NoError(t, err)

		finalWinner, err := resolver.Resolve(stateID, winner2, draw[3])
		require.NoError(t, err)

		return finalWinner
	}

	runKnockout := func(draw []string, stateID string) string {
		winnerSF1, err := resolver.Resolve(stateID, draw[0], draw[1])
		require.NoError(t, err)
		winnerSF2, err := resolver.Resolve(stateID, draw[2], draw[3])
		require.NoError(t, err)
		finalWinner, err := resolver.Resolve(stateID, winnerSF1, winnerSF2)
		require.NoError(t, err)
		return finalWinner
	}

	for i := 0; i < 10; i++ {
		stateID := uuid.New().String()

		winner1 := runKnockout(knockOutDraw1, stateID)
		winner2 := runKnockout(knockOutDraw2, stateID)
		winner3 := runKnockout(knockOutDraw3, stateID)
		assert.Equal(t, winner1, winner2)
		assert.Equal(t, winner2, winner3)
		for _, order := range orders {
			//for all the different combination of orders, run the winner stays on format and check that the winner is the same as that of the knockout format
			winner := runWinnerStaysOn(order, stateID)
			assert.Equal(t, winner, winner1)
		}
	}
}
