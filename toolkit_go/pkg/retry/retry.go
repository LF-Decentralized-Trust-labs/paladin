// Copyright © 2023 Kaleido, Inc.
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
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tkmsgs"
)

type Retry struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	factor       float64
	maxAttempts  int
}

func NewRetryIndefinite(conf *Config) *Retry {
	return &Retry{
		initialDelay: confutil.DurationMin(conf.InitialDelay, 0, *Defaults.InitialDelay),
		maxDelay:     confutil.DurationMin(conf.MaxDelay, 0, *Defaults.MaxDelay),
		factor:       confutil.Float64Min(conf.Factor, 1.0, *Defaults.Factor),
	}
}

func NewRetryLimited(conf *ConfigWithMax) *Retry {
	base := NewRetryIndefinite(&conf.Config)
	base.maxAttempts = confutil.IntMin(conf.MaxAttempts, 0, *Defaults.MaxAttempts)
	base.maxDelay = confutil.DurationMin(conf.MaxDelay, 0, *Defaults.MaxDelay)
	return base
}

// Do invokes the function until the function returns false, or the retry pops.
// This simple interface doesn't pass through errors or return values, on the basis
// you'll be using a closure for that.
func (r *Retry) Do(ctx context.Context, do func(attempt int) (retryable bool, err error)) error {
	attempt := 0
	for {
		attempt++
		retry, err := do(attempt)
		if err != nil {
			log.L(ctx).Errorf("%s (attempt=%d)", err, attempt)
		}
		if !retry || err == nil || (r.maxAttempts > 0 && attempt >= r.maxAttempts) {
			return err
		}
		if err := r.WaitDelay(ctx, attempt); err != nil {
			return err
		}
	}
}

func (r *Retry) WaitDelay(ctx context.Context, failureCount int) error {
	if failureCount > 0 {
		retryDelay := r.initialDelay
		for i := 0; i < (failureCount - 1); i++ {
			retryDelay = time.Duration(float64(retryDelay) * r.factor)
			if retryDelay > r.maxDelay {
				retryDelay = r.maxDelay
				break
			}
		}
		log.L(ctx).Debugf("Retrying after %.2f (failures=%d)", retryDelay.Seconds(), failureCount)
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return i18n.NewError(ctx, tkmsgs.MsgContextCanceled)
		}
	}
	return nil
}

// SetMaxAttempts is useful for unit tests
func (r *Retry) UTSetMaxAttempts(maxAttempts int) {
	r.maxAttempts = maxAttempts
}
