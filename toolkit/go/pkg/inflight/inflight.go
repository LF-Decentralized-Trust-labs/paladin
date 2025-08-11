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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

type InflightManager[K comparable, T any] struct {
	lock     sync.Mutex
	parseStr func(string) (K, error)
	requests map[K]*InflightRequest[K, T]
	closed   bool
}

type InflightRequest[K comparable, T any] struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	ifm       *InflightManager[K, T]
	id        K
	queued    time.Time
	done      chan T
}

func NewInflightManager[K comparable, T any](parseStr func(string) (K, error)) *InflightManager[K, T] {
	return &InflightManager[K, T]{
		parseStr: parseStr,
		requests: make(map[K]*InflightRequest[K, T]),
	}
}

// Inflight requests are scoped to a context, and Wait() will cancel on either;
// - The supplied context closing
// - The inflight manager closing
func (ifm *InflightManager[K, T]) AddInflight(ctx context.Context, id K) *InflightRequest[K, T] {
	req := &InflightRequest[K, T]{
		ifm:    ifm,
		id:     id,
		queued: time.Now(),
		done:   make(chan T, 1),
	}
	req.ctx, req.cancelCtx = context.WithCancel(ctx)
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	ifm.requests[id] = req
	if ifm.closed {
		req.cancelCtx()
	}
	return req
}

func (ifm *InflightManager[K, T]) GetInflightStr(strID string) *InflightRequest[K, T] {
	id, err := ifm.parseStr(strID)
	if err != nil {
		log.L(context.Background()).Errorf("Invalid ID supplied '%s': %s", strID, err)
		return nil
	}
	return ifm.GetInflight(id)
}

func (ifm *InflightManager[K, T]) GetInflight(id K) *InflightRequest[K, T] {
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	return ifm.requests[id]
}

func (ifm *InflightManager[K, T]) waitInFlight(req *InflightRequest[K, T]) (T, error) {
	select {
	case <-req.ctx.Done():
		return *new(T), i18n.NewError(req.ctx, pldmsgs.MsgInflightRequestCancelled, req.Age())
	case reply := <-req.done:
		return reply, nil
	}
}

func (ifm *InflightManager[K, T]) InFlightCount() int {
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	return len(ifm.requests)
}

func (ifm *InflightManager[K, T]) cancelInFlight(req *InflightRequest[K, T]) {
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	req.cancelCtx()
	delete(ifm.requests, req.id)
}

func (ifm *InflightManager[K, T]) Close() {
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	ifm.closed = true
	for _, req := range ifm.requests {
		req.cancelCtx()
		delete(ifm.requests, req.id)
	}
}

func (req *InflightRequest[K, T]) ID() K {
	return req.id
}

func (req *InflightRequest[K, T]) Age() time.Duration {
	return time.Since(req.queued)
}

func (req *InflightRequest[K, T]) Complete(v T) {
	// Can only complete once, so do not block
	select {
	case req.done <- v:
	default:
	}
}

func (req *InflightRequest[K, T]) Wait() (T, error) {
	return req.ifm.waitInFlight(req)
}

func (req *InflightRequest[K, T]) Cancel() {
	req.ifm.cancelInFlight(req)
}
