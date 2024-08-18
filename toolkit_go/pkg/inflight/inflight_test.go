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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestInFlightLifecycleOK(t *testing.T) {

	ifm := NewInflightManager[uuid.UUID, string](func(s string) (uuid.UUID, error) {
		return uuid.Parse(s)
	})

	ctx := context.Background()
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	id := uuid.New()
	req := ifm.AddInflight(id)
	assert.Equal(t, id, req.ID())

	assert.Nil(t, ifm.GetInflightCorrelID(nil))
	assert.Nil(t, ifm.GetInflightCorrelID(prtTo("wrong")))
	assert.Nil(t, ifm.GetInflightCorrelID(prtTo(uuid.NewString())))

	assert.Equal(t, req, ifm.GetInflightCorrelID(prtTo(req.ID().String())))

	// Check context timeout
	_, err := req.Wait(cancelledCtx)
	assert.Regexp(t, "PD020100", err)

	// Complete
	go func() {
		req.Complete("hello")
	}()
	v, err := req.Wait(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "hello", v)

	// caller always responsible for cancelling
	req.Cancel()
	assert.Nil(t, ifm.GetInflight(req.ID()))

	// Duplicate notifies are swallowed
	req.Complete("ignore")
	req.Complete("mew")
}

func prtTo[T any](v T) *T {
	return &v
}
