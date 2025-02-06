/*
 * Copyright © 2025 Kaleido, Inc.
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
package common

import (
	"context"

	"github.com/google/uuid"
)

type IdempotentRequest struct {
	idempotencyKey   uuid.UUID                                                 //unique string to identify the request (re-used across retries)
	requestTime      Time                                                      //time the request was most recently sent
	firstRequestTime Time                                                      //time the request was first sent
	send             func(ctx context.Context, idempotencyKey uuid.UUID) error // function to send the request
	clock            Clock
	timeout          Duration
}

func NewIdempotentRequest(ctx context.Context, clock Clock, timeout Duration, sendRequest func(ctx context.Context, idempotencyKey uuid.UUID) error) *IdempotentRequest {
	idempotencyKey := uuid.New()
	r := &IdempotentRequest{
		idempotencyKey:   idempotencyKey,
		clock:            clock,
		send:             sendRequest,
		timeout:          timeout,
		requestTime:      nil,
		firstRequestTime: nil,
	}

	return r
}

func (r *IdempotentRequest) IdempotencyKey() uuid.UUID {
	return r.idempotencyKey
}

func (r *IdempotentRequest) FirstRequestTime() Time {
	return r.firstRequestTime
}

// Prompt to check whether a retry is due and if so, send the request
func (r *IdempotentRequest) Nudge(ctx context.Context) error {
	if r.requestTime == nil || r.clock.HasExpired(r.requestTime, r.timeout) {
		err := r.send(ctx, r.idempotencyKey)
		if err == nil {
			r.requestTime = r.clock.Now()
			if r.firstRequestTime == nil {
				r.firstRequestTime = r.requestTime
			}
		}
		return err
	}
	return nil
}
