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

package main

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

// Utility re-usable across components for doing sync waiting on correlation of a submitted TX to an event
type transactionWaitUtils[T any] struct {
	mux     sync.Mutex
	waiters map[uuid.UUID][]*transactionWaiter[T]
}

type transactionWaiter[T any] struct {
	twu  *transactionWaitUtils[T]
	txID uuid.UUID
	done chan T
}

func newTransactionWaitUtils[T any]() *transactionWaitUtils[T] {
	return &transactionWaitUtils[T]{
		waiters: make(map[uuid.UUID][]*transactionWaiter[T]),
	}
}

func (twu *transactionWaitUtils[T]) txWaiter(ctx context.Context, txID uuid.UUID) *transactionWaiter[T] {
	twu.mux.Lock()
	defer twu.mux.Unlock()
	tw := &transactionWaiter[T]{
		twu:  twu,
		txID: txID,
		done: make(chan T, 1), // slot to ensure no blocking
	}
	log.L(ctx).Infof("Adding waiter for TX %s", &txID)
	twu.waiters[txID] = append(twu.waiters[txID], tw)
	return tw
}

func (twu *transactionWaitUtils[T]) notifyTX(ctx context.Context, txID uuid.UUID, value T) {
	twu.mux.Lock()
	defer twu.mux.Unlock()

	waiters := twu.waiters[txID]
	for _, tw := range waiters {
		log.L(ctx).Infof("Notifying waiter for TX %s", &txID)
		select {
		case tw.done <- value:
		default:
		}
	}
}

// Caller must call cancel (regardless of whether wait is called)
func (tw *transactionWaiter[T]) cancel() {
	tw.twu.mux.Lock()
	defer tw.twu.mux.Unlock()
	waiters := tw.twu.waiters[tw.txID]
	if len(waiters) == 1 {
		delete(tw.twu.waiters, tw.txID)
	} else {
		newWaiters := []*transactionWaiter[T]{}
		for _, existing := range waiters {
			if existing != tw {
				newWaiters = append(newWaiters, existing)
			}
		}
		tw.twu.waiters[tw.txID] = newWaiters
	}
}

func (tw *transactionWaiter[T]) wait(ctx context.Context) (T, error) {
	select {
	case v := <-tw.done:
		log.L(ctx).Infof("Waiter for TX %s complete", &tw.txID)
		return v, nil
	case <-ctx.Done():
		return *new(T), i18n.NewError(ctx, msgs.MsgContextCanceled)
	}
}
