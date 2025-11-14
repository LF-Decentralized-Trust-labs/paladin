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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func Test_IdempotentRequestOK(t *testing.T) {
	ctx := context.Background()
	clock := RealClock()
	requested := false
	request := NewIdempotentRequest(ctx, clock, clock.Duration(1000), func(ctx context.Context, idempotencyKey uuid.UUID) error {
		requested = true
		return nil
	})
	err := request.Nudge(ctx)
	assert.NoError(t, err)
	assert.True(t, requested)
}

func Test_IdempotentRequestErrorFromSend(t *testing.T) {
	ctx := context.Background()
	clock := RealClock()
	request := NewIdempotentRequest(ctx, clock, clock.Duration(1000), func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return assert.AnError
	})
	err := request.Nudge(ctx)
	assert.Error(t, err)

}

func Test_IdempotentRequest_RetryOnNudgeIfExpired(t *testing.T) {
	ctx := context.Background()
	clock := &FakeClockForTesting{}
	requested := 0
	request := NewIdempotentRequest(ctx, clock, clock.Duration(1000), func(ctx context.Context, idempotencyKey uuid.UUID) error {
		requested++
		return nil
	})
	err := request.Nudge(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, requested)
	clock.Advance(1001) //Just after expiry
	err = request.Nudge(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 2, requested)

}

func Test_IdempotentRequest_NoRetryOnNudgeIfNotExpired(t *testing.T) {
	ctx := context.Background()
	clock := &FakeClockForTesting{}
	requested := 0
	request := NewIdempotentRequest(ctx, clock, clock.Duration(1000), func(ctx context.Context, idempotencyKey uuid.UUID) error {
		requested++
		return nil
	})
	err := request.Nudge(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, requested)
	clock.Advance(999) //Just before expiry
	err = request.Nudge(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, requested)

}

func Test_IdempotentRequest_FirstRequestTime(t *testing.T) {
	ctx := context.Background()
	clock := &FakeClockForTesting{}
	request := NewIdempotentRequest(ctx, clock, clock.Duration(1000), func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})
	err := request.Nudge(ctx)
	assert.NoError(t, err)
	start := clock.Now()
	assert.Equal(t, start, request.FirstRequestTime())
	clock.Advance(1001)
	err = request.Nudge(ctx)
	assert.NoError(t, err)
	end := clock.Now()
	assert.Equal(t, start, request.FirstRequestTime())
	assert.Equal(t, end, request.requestTime)
	assert.NotEqual(t, request.FirstRequestTime(), request.requestTime)

}
