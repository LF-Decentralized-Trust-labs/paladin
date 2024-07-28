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

package blockindexer

import (
	"github.com/kaleido-io/paladin/kata/internal/cache"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/retry"
	"github.com/kaleido-io/paladin/kata/internal/tls"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

type Config struct {
	FromBlock             types.RawJSON `yaml:"fromBlock"`
	CommitBatchSize       *int          `yaml:"commitBatchSize"`
	RequiredConfirmations *int          `yaml:"requiredConfirmations"`
	ChainHeadCacheLen     *int          `yaml:"chainHeadCacheLen"`
	BlockCache            cache.Config  `yaml:"blockCache"`
	BlockPollingInterval  *string       `yaml:"blockPollingInterval"`
	Retry                 retry.Config  `yaml:"retry"`
}

type RPCWSConnectConfig struct {
	URL string     `yaml:"url"`
	TLS tls.Config `yaml:"tls"`
}

var DefaultConfig = &Config{
	FromBlock:             types.RawJSON(`0`),
	CommitBatchSize:       confutil.P(50),
	RequiredConfirmations: confutil.P(0),
	ChainHeadCacheLen:     confutil.P(50),
	BlockPollingInterval:  confutil.P("10s"),
	BlockCache: cache.Config{
		Capacity: confutil.P(100),
	},
}
