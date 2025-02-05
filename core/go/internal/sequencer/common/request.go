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

type RetriableRequest struct {
	id             uuid.UUID                                              //unique string to identify the request (re-used across retries)
	requestTime    Time                                                   //time the request was most recently sent
	send           func(ctx context.Context, idempotencyKey string) error // function to send the request
	clock          Clock
	requestTimeout Duration
}

func NewRetriableRequest(ctx context.Context, clock Clock, timeout Duration, sendRequest func(ctx context.Context, idempotencyKey string) error) *RetriableRequest {
	id := uuid.New()
	r := &RetriableRequest{
		id:          id,
		clock:       clock,
		send:        sendRequest,
		requestTime: clock.Now(),
	}

	err := r.send(ctx, id.String())
	if err != nil {
		r.requestTime = r.clock.Now()
	}
	return r
}

// Prompt to check whether a retry is due and if so, send the request
func (r *RetriableRequest) Nudge(ctx context.Context) error {
	if r.clock.HasExpired(r.requestTime, r.requestTimeout) {
		err := r.send(ctx, r.id.String())
		if err != nil {
			r.requestTime = r.clock.Now()
		}
		return err
	}
	return nil
}
