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
package plugins

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	pb "google.golang.org/protobuf/proto"
)

type inflightManager[T any] struct {
	getMsgAndCorrelIDs func(T) (string, *string)
	lock               sync.Mutex
	requests           map[uuid.UUID]*inflightRequest[T]
}

type inflightRequest[T any] struct {
	ifm    *inflightManager[T]
	id     uuid.UUID
	queued time.Time
	done   chan T
}

func newInFlightRequests[T any](getMsgAndCorrelIDs func(T) (string, *string)) *inflightManager[T] {
	return &inflightManager[T]{
		getMsgAndCorrelIDs: getMsgAndCorrelIDs,
		requests:           make(map[uuid.UUID]*inflightRequest[T]),
	}
}

func (ifm *inflightManager[T]) addInflight(ctx context.Context, msg pb.Message) *inflightRequest[T] {
	id := uuid.New()
	inFlight := &inflightRequest[T]{
		ifm:    ifm,
		id:     uuid.New(),
		queued: time.Now(),
		done:   make(chan T, 1),
	}
	log.L(ctx).Infof("--> %s [%s]", id, msg.ProtoReflect().Descriptor().FullName())
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	ifm.requests[id] = inFlight
	return inFlight
}

func (ifm *inflightManager[T]) getInflight(ctx context.Context, correlID *string) *inflightRequest[T] {
	if correlID == nil {
		return nil
	}
	id, err := uuid.Parse(*correlID)
	if err != nil {
		log.L(ctx).Errorf("Invalid correlation ID supplied '%s': %s", *correlID, err)
		return nil
	}
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	return ifm.requests[id]
}

func (ifm *inflightManager[T]) waitInFlight(ctx context.Context, inFlight *inflightRequest[T]) (T, error) {
	select {
	case <-ctx.Done():
		log.L(ctx).Errorf("<!- %s", inFlight.id)
		return *new(T), fmt.Errorf("timeout")
	case reply := <-inFlight.done:
		log.L(ctx).Infof("<-- %s", inFlight.id)
		return reply, nil
	}
}

func (ifm *inflightManager[T]) cancelInFlight(req *inflightRequest[T]) {
	ifm.lock.Lock()
	defer ifm.lock.Unlock()
	delete(ifm.requests, req.id)
}

func (req *inflightRequest[T]) wait(ctx context.Context) (T, error) {
	return req.ifm.waitInFlight(ctx, req)
}

func (req *inflightRequest[T]) cancel() {
	req.ifm.cancelInFlight(req)
}
