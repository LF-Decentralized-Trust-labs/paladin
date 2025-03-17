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
	"github.com/stretchr/testify/require"
)

func TestInFlightLifecycleOK(t *testing.T) {

	ifm := NewInflightManager[uuid.UUID, string](func(s string) (uuid.UUID, error) {
		return uuid.Parse(s)
	})

	id := uuid.New()
	req := ifm.AddInflight(context.Background(), id)
	assert.Equal(t, id, req.ID())

	assert.Nil(t, ifm.GetInflightStr("wrong"))
	assert.Nil(t, ifm.GetInflightStr(uuid.NewString()))

	assert.Equal(t, req, ifm.GetInflightStr(req.ID().String()))

	// Complete
	go func() {
		req.Complete("hello")
	}()
	v, err := req.Wait()
	require.NoError(t, err)
	assert.Equal(t, "hello", v)

	// caller always responsible for cancelling
	req.Cancel()
	assert.Nil(t, ifm.GetInflight(req.ID()))

	// Duplicate notifies are swallowed
	req.Complete("ignore")
	req.Complete("mew")
}

func TestInFlightCancel(t *testing.T) {

	ifm := NewInflightManager[uuid.UUID, string](func(s string) (uuid.UUID, error) {
		return uuid.Parse(s)
	})

	id := uuid.New()
	req := ifm.AddInflight(context.Background(), id)
	assert.Equal(t, id, req.ID())

	go func() {
		ifm.Close()
	}()
	_, err := req.Wait()
	assert.Regexp(t, "PD020100", err)

	// check we do not block after close
	id2 := uuid.New()
	req2 := ifm.AddInflight(context.Background(), id2)
	assert.Equal(t, id2, req2.ID())
	assert.Equal(t, ifm.InFlightCount(), 1)
	_, err = req.Wait()
	assert.Regexp(t, "PD020100", err)
}
