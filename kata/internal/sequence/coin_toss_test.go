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

package sequence

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoinToss_FinalMatch100Tosses(t *testing.T) {
	// create 2 ids at random (representing bidding transactions),
	// then itterate over 100 random state ids and check that there is a fair distribution of winners
	winner1 := 0
	winner2 := 0
	biddingTransaction1 := uuid.New().String()
	biddingTransaction2 := uuid.New().String()

	for i := 0; i < 100; i++ {
		stateID := uuid.New().String()
		winner, err := CoinToss(stateID, biddingTransaction1, biddingTransaction2)
		require.NoError(t, err)
		assert.Contains(t, []string{biddingTransaction1, biddingTransaction2}, winner)
		if winner == biddingTransaction1 {
			winner1++
		} else {
			winner2++
		}

	}
	assert.InDelta(t, winner2, winner1, 10)
}

func TestCoinToss_SemiFinal(t *testing.T) {
	// create 4 ids at random (representing bidding transactions),
	// then itterate over 100 random state ids and for each one, run each permutation of semi finals and check that
	// the final winners are always the same and that there is a fair distribution of winners.
	bidders := make([]string, 4)
	for i := 0; i < 4; i++ {
		bidders[i] = uuid.New().String()
	}
	draw1 := []string{bidders[0], bidders[1], bidders[2], bidders[3]}
	draw2 := []string{bidders[0], bidders[2], bidders[1], bidders[3]}
	draw3 := []string{bidders[0], bidders[3], bidders[1], bidders[2]}
	for i := 0; i < 100; i++ {
		stateId := uuid.New().String()
		runDraw := func(draw []string) string {
			winnerSF1, err := CoinToss(stateId, draw[0], draw[1])
			require.NoError(t, err)
			winnerSF2, err := CoinToss(stateId, draw[2], draw[3])
			require.NoError(t, err)
			finalWinner, err := CoinToss(stateId, winnerSF1, winnerSF2)
			require.NoError(t, err)
			return finalWinner
		}
		winner1 := runDraw(draw1)
		winner2 := runDraw(draw2)
		winner3 := runDraw(draw3)
		assert.Equal(t, winner1, winner2)
		assert.Equal(t, winner2, winner3)
	}
}
