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
package inflight

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/msgs"
)

type InflightManager[K comparable, T any] struct {
	lock     sync.Mutex
	parseStr func(string) (K, error)
	requests map[K]*InflightRequest[K, T]
}

type InflightRequest[K comparable, T any] struct {
	ifm    *InflightManager[K, T]
	id     K
	queued time.Time
	done   chan T
}

func NewInflightManager[K comparable, T any](parseStr func(string) (K, error)) *InflightManager[K, T] {
	return &InflightManager[K, T]{
		parseStr: parseStr,
		requests: make(map[K]*InflightRequest[K, T]),
	}
}

func (ifm *InflightManager[K, T]) AddInflight(id K) *InflightRequest[K, T] {
	req := &InflightRequest[K, T]{
		ifm:    ifm,
		id:     id,
		queued: time.Now(),
		done:   make(chan T, 1),
	}
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	ifm.requests[id] = req
	return req
}

func (ifm *InflightManager[K, T]) GetInflightCorrelID(correlID *string) *InflightRequest[K, T] {
	if correlID == nil {
		return nil
	}
	id, err := ifm.parseStr(*correlID)
	if err != nil {
		log.L(context.Background()).Errorf("Invalid correlation ID supplied '%s': %s", *correlID, err)
		return nil
	}
	return ifm.GetInflight(id)
}

func (ifm *InflightManager[K, T]) GetInflight(id K) *InflightRequest[K, T] {
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	return ifm.requests[id]
}

func (ifm *InflightManager[K, T]) waitInFlight(ctx context.Context, req *InflightRequest[K, T]) (T, error) {
	select {
	case <-ctx.Done():
		return *new(T), i18n.NewError(ctx, msgs.MsgInflightRequestTimedOut, time.Since(req.queued))
	case reply := <-req.done:
		return reply, nil
	}
}

func (ifm *InflightManager[K, T]) cancelInFlight(req *InflightRequest[K, T]) {
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	delete(ifm.requests, req.id)
}

func (req *InflightRequest[K, T]) ID() K {
	return req.id
}

func (req *InflightRequest[K, T]) Complete(v T) {
	// Can only complete once, so do not block
	select {
	case req.done <- v:
	default:
	}
}

func (req *InflightRequest[K, T]) Wait(ctx context.Context) (T, error) {
	return req.ifm.waitInFlight(ctx, req)
}

func (req *InflightRequest[K, T]) Cancel() {
	req.ifm.cancelInFlight(req)
}
