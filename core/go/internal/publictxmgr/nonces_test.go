/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package publictxmgr

import (
	"context"
	"database/sql/driver"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"

	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIntentToAssignNonce(t *testing.T) {
	ctx := context.Background()
	callbackHasBeenCalled := false
	nonceCache := newNonceCacheForTesting(func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
		callbackHasBeenCalled = true
		return uint64(42), nil
	})
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.True(t, callbackHasBeenCalled)
	intent.Complete(ctx)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}
}

func TestAssignNonce(t *testing.T) {
	ctx := context.Background()
	callbackHasBeenCalled := false
	nonceCache := newNonceCacheForTesting(func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
		callbackHasBeenCalled = true
		return uint64(42), nil
	})
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.True(t, callbackHasBeenCalled)

	nextNonce, err := intent.AssignNextNonce(ctx)
	require.NoError(t, err)

	assert.Equal(t, uint64(42), nextNonce)
	intent.Complete(ctx)

	callbackHasBeenCalled = false

	intent, err = nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.False(t, callbackHasBeenCalled)

	nextNonce, err = intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(43), nextNonce)
	intent.Complete(ctx)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}

}

func TestIntentToAssignNonceRollbackNoAssign(t *testing.T) {
	ctx := context.Background()
	callbackHasBeenCalled := false
	nonceCache := newNonceCacheForTesting(func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
		callbackHasBeenCalled = true
		return uint64(42), nil
	})
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.True(t, callbackHasBeenCalled)
	intent.Rollback(ctx)

	//check that the nonce is still in memory and if assigned, we get the correct nonce
	callbackHasBeenCalled = false
	intent, err = nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.False(t, callbackHasBeenCalled)

	nextNonce, err := intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(42), nextNonce)
	intent.Complete(ctx)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}
}

func TestIntentToAssignNonceCompleteNoAssign(t *testing.T) {
	ctx := context.Background()
	callbackHasBeenCalled := false
	nonceCache := newNonceCacheForTesting(func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
		callbackHasBeenCalled = true
		return uint64(42), nil
	})
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.True(t, callbackHasBeenCalled)
	intent.Complete(ctx)

	callbackHasBeenCalled = false
	intent, err = nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.False(t, callbackHasBeenCalled)

	nextNonce, err := intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(42), nextNonce)
	intent.Complete(ctx)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}
}

func TestAssignNonceMultipleNonces(t *testing.T) {
	ctx := context.Background()
	nonceCache := newNonceCacheForTesting()
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)

	nextNonce, err := intent.AssignNextNonce(ctx)
	require.NoError(t, err)

	assert.Equal(t, uint64(42), nextNonce)
	nextNonce, err = intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(43), nextNonce)
	nextNonce, err = intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(44), nextNonce)

	intent.Complete(ctx)

	intent, err = nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)

	nextNonce, err = intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(45), nextNonce)
	intent.Complete(ctx)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}

}

func TestAssignNonceRollback(t *testing.T) {
	ctx := context.Background()
	callbackHasBeenCalled := false
	nonceCache := newNonceCacheForTesting(func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
		callbackHasBeenCalled = true
		return uint64(42), nil
	})
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.True(t, callbackHasBeenCalled)

	nextNonce, err := intent.AssignNextNonce(ctx)
	require.NoError(t, err)

	assert.Equal(t, uint64(42), nextNonce)
	intent.Rollback(ctx)

	callbackHasBeenCalled = false

	intent, err = nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)
	assert.False(t, callbackHasBeenCalled)

	nextNonce, err = intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	//should get 42 as before given that the previous assignment got rolled back
	assert.Equal(t, uint64(42), nextNonce)
	intent.Complete(ctx)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}
}

func TestAssignNonceMultipleNoncesRollback(t *testing.T) {
	ctx := context.Background()
	nonceCache := newNonceCacheForTesting()
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)

	nextNonce, err := intent.AssignNextNonce(ctx)
	require.NoError(t, err)

	assert.Equal(t, uint64(42), nextNonce)
	nextNonce, err = intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(43), nextNonce)
	nextNonce, err = intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(44), nextNonce)

	intent.Rollback(ctx)

	intent, err = nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)

	nextNonce, err = intent.AssignNextNonce(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(42), nextNonce)
	intent.Complete(ctx)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}
}

func TestAssignNonceAfterCompleteFail(t *testing.T) {
	ctx := context.Background()

	nonceCache := newNonceCacheForTesting()
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)

	nextNonce, err := intent.AssignNextNonce(ctx)
	require.NoError(t, err)

	assert.Equal(t, uint64(42), nextNonce)
	intent.Complete(ctx)

	_, err = intent.AssignNextNonce(ctx)
	assert.Error(t, err)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}
}

func TestAssignNonceAfterRollbackFail(t *testing.T) {
	ctx := context.Background()
	nonceCache := newNonceCacheForTesting()
	defer nonceCache.Stop()
	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
	require.NoError(t, err)

	nextNonce, err := intent.AssignNextNonce(ctx)
	require.NoError(t, err)

	assert.Equal(t, uint64(42), nextNonce)
	intent.Rollback(ctx)

	_, err = intent.AssignNextNonce(ctx)
	assert.Error(t, err)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}
}

func TestAssignNonceMultiThreaded(t *testing.T) {
	ctx := context.Background()
	callbackCalled := 0
	firstNonce := uint64(42)
	nonceCache := newNonceCacheForTesting(func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
		callbackCalled++
		return firstNonce, nil
	})
	iterations := 100
	threads := 100
	rollbacks := 0
	results := make([][]uint64, threads)
	for i := 0; i < threads; i++ {
		results[i] = make([]uint64, iterations)
	}

	signer := tktypes.EthAddress(tktypes.RandBytes(20))
	doneIt := make(chan struct{}, threads)
	doIt := func(threadNumber int) {
		for iteration := 0; iteration < iterations; iteration++ {

			intent, err := nonceCache.IntentToAssignNonce(ctx, signer)
			require.NoError(t, err)
			nextNonce, err := intent.AssignNextNonce(ctx)
			require.NoError(t, err)
			if rand.Intn(10) == 9 {
				//rollback on average 10% of the time
				results[threadNumber][iteration] = 1 // 1 is a special value meaning rolledback
				rollbacks++
				intent.Rollback(ctx)
			} else {
				results[threadNumber][iteration] = nextNonce
				intent.Complete(ctx)
			}
		}
		doneIt <- struct{}{}
	}

	for thread := 0; thread < threads; thread++ {
		go doIt(thread)
	}
	threadsDone := 0
	for {
		<-doneIt
		threadsDone++
		if threadsDone == threads {
			break
		}
	}

	highestNonce := firstNonce + (uint64(iterations) * uint64(threads)) - 1 - uint64(rollbacks)

	//we should have a non gapless set of results and within a given thread, they should be in order
	haveSeenFirstNonce := false
	haveSeenHighestNonce := false
	seen := make([]bool, iterations*threads)
	for threadNumber, threadResults := range results {
		previousNonce := firstNonce - 1
		for iterationNumber, iterationResult := range threadResults {
			if iterationResult != 1 { //wasn't one of the random rollbacks
				assert.Greater(t, iterationResult, previousNonce, "nonce %d out of order - not greater than %d on thread %d iteration %d ", iterationResult, previousNonce, threadNumber, iterationNumber)
				assert.False(t, seen[iterationResult-firstNonce], "nonce %d used twice on thread %d iteration %d ", iterationResult, threadNumber, iterationNumber)
				seen[iterationResult-firstNonce] = true
				if iterationResult == firstNonce {
					haveSeenFirstNonce = true
				}
				if iterationResult == highestNonce {
					haveSeenHighestNonce = true
				}
			}
		}
	}
	//given that all nonces are unique and are greater or equal to the first nonce,
	// if we have seen the first and highest nonce then it must be gapless
	assert.True(t, haveSeenFirstNonce)
	assert.True(t, haveSeenHighestNonce)
	assert.Equal(t, 1, callbackCalled)

	//Check that the reapear lock is in a state where the reaper can grab it
	gotLock := nonceCache.reaperLock.TryLock()
	assert.True(t, gotLock)
	if gotLock {
		nonceCache.reaperLock.Unlock()
	}
}

func TestAssignNonceMultiThreadedMultiSigningAddresses(t *testing.T) {
	tests := []struct {
		name                        string
		numSigningAddresses         int
		numThreadsPerSigningAddress int
		numItterationsPerThread     int
	}{
		{
			name:                        "10 signing addresses, 10 threads per signing address, 1000 iterations per thread",
			numSigningAddresses:         10,
			numThreadsPerSigningAddress: 10,
			numItterationsPerThread:     1000,
		},
		{
			name:                        "2 signing addresses, 100 threads per signing address, 1000 iterations per thread",
			numSigningAddresses:         2,
			numThreadsPerSigningAddress: 100,
			numItterationsPerThread:     1000,
		},
		{
			name:                        "100 signing addresses, 2 threads per signing address, 1000 iterations per thread",
			numSigningAddresses:         100,
			numThreadsPerSigningAddress: 2,
			numItterationsPerThread:     1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctx := context.Background()
			firstNonce := uint64(42)
			//assert that that callback is only called once per signing address
			callbackCalled := make(map[tktypes.EthAddress]bool)
			nonceCache := newNonceCacheForTesting(func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
				assert.False(t, callbackCalled[signer])
				callbackCalled[signer] = true
				return firstNonce, nil
			})

			numSigningAddresses := tt.numSigningAddresses
			numThreadsPerSigningAddress := tt.numThreadsPerSigningAddress
			numItterationsPerThread := tt.numItterationsPerThread

			//generate a random hex string for each signing address
			signingAddresses := make([]tktypes.EthAddress, numSigningAddresses)
			for i := 0; i < numSigningAddresses; i++ {
				signingAddresses[i] = tktypes.EthAddress(tktypes.RandBytes(20))
			}

			// we are going to keep a count of how many rollbacks there were for each signing address
			//so that we can calculate which nonce number we expect to get up to per signign address
			rollbacks := make([]int32, numSigningAddresses)

			//create a 3 dimensional matrix to store the results
			results := make([][][]uint64, numSigningAddresses)
			for s := 0; s < numSigningAddresses; s++ {
				results[s] = make([][]uint64, numThreadsPerSigningAddress)
				for t := 0; t < numThreadsPerSigningAddress; t++ {
					results[s][t] = make([]uint64, numItterationsPerThread)
				}
			}

			// inner function to run through a number of iterations on a single thread
			runItterationsForThread := func(threadNumber int, signingAddressIndex int, signingAddress tktypes.EthAddress) {
				for iteration := 0; iteration < numItterationsPerThread; iteration++ {
					intent, err := nonceCache.IntentToAssignNonce(ctx, signingAddress)
					defer intent.Rollback(ctx)
					require.NoError(t, err)
					nextNonce, err := intent.AssignNextNonce(ctx)
					require.NoError(t, err)
					if rand.Intn(10) == 9 {
						//rollback on average 10% of the time
						results[signingAddressIndex][threadNumber][iteration] = 1 // 1 is a special value meaning rolledback
						atomic.AddInt32(&rollbacks[signingAddressIndex], 1)
						intent.Rollback(ctx)
					} else {
						results[signingAddressIndex][threadNumber][iteration] = nextNonce
						intent.Complete(ctx)
					}
				}
			}

			//	function to start a number of threads for a given signing address and wait for each of them to complete
			runThreadsForSigningAddress := func(signingAddressIndex int, signingAddress tktypes.EthAddress) {
				var wg sync.WaitGroup
				wg.Add(numThreadsPerSigningAddress)
				for thread := 0; thread < numThreadsPerSigningAddress; thread++ {
					threadIndex := thread
					go func() {
						runItterationsForThread(threadIndex, signingAddressIndex, signingAddress)
						wg.Done()
					}()
				}
				wg.Wait()
			}

			var wg sync.WaitGroup
			wg.Add(numSigningAddresses)
			for signingAddressIndex, signingAddress := range signingAddresses {
				saIndex, sA := signingAddressIndex, signingAddress
				go func() {
					runThreadsForSigningAddress(saIndex, sA)
					wg.Done()
				}()
			}
			wg.Wait()

			//all done, now analyze the results
			for signingAddressNumber, signingAddressResults := range results {
				highestNonce := firstNonce + (uint64(numItterationsPerThread) * uint64(numThreadsPerSigningAddress)) - 1 - uint64(rollbacks[signingAddressNumber])
				//we should have a non gapless set of results and within a given thread, they should be in order
				haveSeenFirstNonce := false
				haveSeenHighestNonce := false
				seen := make([]bool, numItterationsPerThread*numThreadsPerSigningAddress)

				for threadNumber, threadResults := range signingAddressResults {
					previousNonce := firstNonce - 1
					for iterationNumber, iterationResult := range threadResults {
						if iterationResult != 1 { //wasn't one of the random rollbacks
							assert.Greater(t, iterationResult, previousNonce, "nonce %d out of order - not greater than %d on signing address %d thread %d iteration %d ", iterationResult, signingAddressNumber, previousNonce, threadNumber, iterationNumber)
							assert.False(t, seen[iterationResult-firstNonce], "nonce %d used twice on signing address %d thread %d iteration %d ", iterationResult, signingAddressNumber, threadNumber, iterationNumber)
							seen[iterationResult-firstNonce] = true
							if iterationResult == firstNonce {
								haveSeenFirstNonce = true
							}
							if iterationResult == highestNonce {
								haveSeenHighestNonce = true
							}
							previousNonce = iterationResult
						}
					}
				}
				//given that all nonces are unique and are greater or equal to the first nonce,
				// if we have seen the first and highest nonce then it must be gapless
				assert.True(t, haveSeenFirstNonce, "did not see first nonce %d for signing address %d", firstNonce, signingAddressNumber)
				assert.True(t, haveSeenHighestNonce, "did not see highest %d nonce for signing address %d", highestNonce, signingAddressNumber)
				assert.True(t, callbackCalled[signingAddresses[signingAddressNumber]], "callback was not called for signing address %d", signingAddressNumber)
			}
			//Check that the reapear lock is in a state where the reaper can grab it
			gotLock := nonceCache.reaperLock.TryLock()
			assert.True(t, gotLock)
			if gotLock {
				nonceCache.reaperLock.Unlock()
			}
		})
	}
}

func newNonceCacheForTesting(cbFuncs ...NextNonceCallback) *nonceCacheStruct {
	cbFunc := func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
		return uint64(42), nil
	}
	if len(cbFuncs) == 1 {
		cbFunc = cbFuncs[0]
	}
	nonceCache := newNonceCache(10*time.Second, cbFunc)
	return nonceCache.(*nonceCacheStruct)
}

func TestBatchDoubleSubmit(t *testing.T) {
	ctx, ble, mocks, done := newTestPublicTxManager(t, false, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Manager.NonceCacheTimeout = confutil.P("0")
	})
	defer done()

	mocks.ethClient.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(confutil.P(tktypes.HexUint64(1122334455)), nil).Once()

	addr := tktypes.RandAddress()
	batch, err := ble.PrepareSubmissionBatch(ctx, []*components.PublicTxSubmission{
		{
			PublicTxInput: pldapi.PublicTxInput{
				From: addr,
				PublicTxOptions: pldapi.PublicTxOptions{
					Gas:   confutil.P(tktypes.HexUint64(1223451)),
					Value: tktypes.Uint64ToUint256(100),
				},
			},
		},
	})
	require.NoError(t, err)
	assert.Empty(t, batch.Rejected())
	assert.Len(t, batch.Accepted(), 1)

	mocks.db.ExpectBegin()
	mocks.db.ExpectExec("INSERT.*public_txns").WillReturnResult(driver.ResultNoRows)
	mocks.db.ExpectCommit()
	mocks.db.ExpectBegin()
	mocks.db.ExpectRollback()

	err = ble.p.DB().Transaction(func(dbTX *gorm.DB) error {
		return batch.Submit(ctx, dbTX)
	})
	require.NoError(t, err)
	batch.Completed(ctx, true) // would normally be in a defer

	err = ble.p.DB().Transaction(func(dbTX *gorm.DB) error {
		return batch.Submit(ctx, dbTX)
	})
	assert.Regexp(t, "PD011933", err)

	// Check that we reaped with nonce timeout zero
	nc := ble.nonceManager.(*nonceCacheStruct)
	nc.reaperLock.Lock()
	defer nc.reaperLock.Unlock()
	assert.Len(t, nc.nextNonceBySigner, 0)

}

func TestReapLoop(t *testing.T) {
	nc := newNonceCache(10*time.Millisecond, func(ctx context.Context, signer tktypes.EthAddress) (uint64, error) {
		return 0, nil
	}).(*nonceCacheStruct)
	defer nc.Stop()
	ian, err := nc.IntentToAssignNonce(context.Background(), *tktypes.RandAddress())
	require.NoError(t, err)
	ian.Complete(context.Background())

	for {
		nc.reaperLock.Lock()
		cacheLen := len(nc.nextNonceBySigner)
		nc.reaperLock.Unlock()
		if cacheLen == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

}
