// Copyright © 2021 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package retry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryEventuallyOk(t *testing.T) {
	r := NewRetryIndefinite(&pldconf.RetryConfig{
		InitialDelay: confutil.P("1ms"),
		MaxDelay:     confutil.P("3ms"),
	})
	err := r.Do(context.Background(), func(i int) (retry bool, err error) {
		if i < 10 {
			err = fmt.Errorf("pop")
		}
		return true, err
	})
	require.NoError(t, err, "pop")
}

func TestRetryDeadlineTimeout(t *testing.T) {
	r := NewRetryIndefinite(&pldconf.RetryConfig{
		InitialDelay: confutil.P("1s"),
		MaxDelay:     confutil.P("1s"),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Millisecond)
	defer cancel()
	err := r.Do(ctx, func(i int) (retry bool, err error) {
		return true, fmt.Errorf("pop")
	})
	assert.Regexp(t, "PD020000", err)
}

func TestRetryContextCanceled(t *testing.T) {
	r := NewRetryIndefinite(&pldconf.RetryConfig{
		InitialDelay: confutil.P("1s"),
		MaxDelay:     confutil.P("1s"),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.Do(ctx, func(i int) (retry bool, err error) {
		return true, fmt.Errorf("pop")
	})
	assert.Regexp(t, "PD020000", err)
}

func TestRetryLimited(t *testing.T) {
	r := NewRetryLimited(&pldconf.RetryConfigWithMax{
		RetryConfig: pldconf.RetryConfig{
			InitialDelay: confutil.P("1ms"),
			MaxDelay:     confutil.P("1ms"),
		},
		MaxAttempts: confutil.P(5),
	})
	callCount := 0
	err := r.Do(context.Background(), func(i int) (retry bool, err error) {
		callCount = i
		return true, fmt.Errorf("pop")
	})
	assert.Regexp(t, "pop", err)
	assert.Equal(t, 5, callCount)
}

func TestRetryUTLimited(t *testing.T) {
	r := NewRetryIndefinite(&pldconf.RetryConfig{
		InitialDelay: confutil.P("1ms"),
		MaxDelay:     confutil.P("1ms"),
	})
	r.UTSetMaxAttempts(5)
	callCount := 0
	err := r.Do(context.Background(), func(i int) (retry bool, err error) {
		callCount = i
		return true, fmt.Errorf("pop")
	})
	assert.Regexp(t, "pop", err)
	assert.Equal(t, 5, callCount)
}

func TestDefaultsOverride(t *testing.T) {
	r := NewRetryLimited(&pldconf.RetryConfigWithMax{}, &pldconf.RetryConfigWithMax{
		RetryConfig: pldconf.RetryConfig{
			InitialDelay: confutil.P("1ms"),
			MaxDelay:     confutil.P("2ms"),
			Factor:       confutil.P(3.14),
		},
		MaxAttempts: confutil.P(42),
	})
	assert.Equal(t, 1*time.Millisecond, r.initialDelay)
	assert.Equal(t, 2*time.Millisecond, r.maxDelay)
	assert.Equal(t, 3.14, r.factor)
	assert.Equal(t, 42, r.maxAttempts)

}
