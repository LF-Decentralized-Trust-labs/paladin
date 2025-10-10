// Copyright © 2024 Kaleido, Inc.
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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
)

type Retry struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	factor       float64
	maxAttempts  int
}

func NewRetryIndefinite(conf *pldconf.RetryConfig, defaults ...*pldconf.RetryConfig) *Retry {
	def := &pldconf.GenericRetryDefaults.RetryConfig
	if len(defaults) > 0 {
		def = defaults[0]
	}
	return &Retry{
		initialDelay: confutil.DurationMin(conf.InitialDelay, 0, *def.InitialDelay),
		maxDelay:     confutil.DurationMin(conf.MaxDelay, 0, *def.MaxDelay),
		factor:       confutil.Float64Min(conf.Factor, 1.0, *def.Factor),
	}
}

func NewRetryLimited(conf *pldconf.RetryConfigWithMax, defaults ...*pldconf.RetryConfigWithMax) *Retry {
	def := pldconf.GenericRetryDefaults
	if len(defaults) > 0 {
		def = defaults[0]
	}
	base := NewRetryIndefinite(&conf.RetryConfig, &def.RetryConfig)
	base.maxAttempts = confutil.IntMin(conf.MaxAttempts, 0, *def.MaxAttempts)
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
			return i18n.NewError(ctx, pldmsgs.MsgContextCanceled)
		}
	}
	return nil
}

// UTSetMaxAttempts is a UNIT TEST ONLY function to switch an unlimited retry, into a limited retry.
// This is helpful to provoke code to return an error condition, rather than just spinning indefinitely
// retrying against that error condition. For unit tests that are testing individual error conditions.
func (r *Retry) UTSetMaxAttempts(maxAttempts int) {
	r.maxAttempts = maxAttempts
}
