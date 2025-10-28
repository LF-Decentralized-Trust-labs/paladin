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
package pldconf

import (
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
)

type SequencerConfig struct {
	AssembleTimeout         *string           `json:"assembleTimeout"`
	RequestTimeout          *string           `json:"requestTimeout"`
	BlockHeightTolerance    *uint64           `json:"blockHeightTolerance"`
	BlockRange              *uint64           `json:"blockRange"`
	ClosingGracePeriod      *int              `json:"closingGracePeriod"`
	MaxInflightTransactions *int              `json:"maxInflightTransactions"`
	MaxDispatchAhead        *int              `json:"maxDispatchAhead"`
	Writer                  FlushWriterConfig `json:"writer"`
}

type SequencerMinimumConfig struct {
	AssembleTimeout         time.Duration
	RequestTimeout          time.Duration
	BlockHeightTolerance    uint64
	BlockRange              uint64
	ClosingGracePeriod      int
	MaxInflightTransactions int
	MaxDispatchAhead        int
}

var SequencerDefaults = &SequencerConfig{
	AssembleTimeout:         confutil.P("60s"),
	RequestTimeout:          confutil.P("10s"),
	BlockHeightTolerance:    confutil.P(uint64(10)),
	BlockRange:              confutil.P(uint64(100)),
	ClosingGracePeriod:      confutil.P(4),
	MaxInflightTransactions: confutil.P(500),
	MaxDispatchAhead:        confutil.P(10),
}

var SequencerMinimum = &SequencerMinimumConfig{
	AssembleTimeout:         1 * time.Second,
	RequestTimeout:          1 * time.Second,
	BlockHeightTolerance:    1,
	BlockRange:              10,
	ClosingGracePeriod:      1,
	MaxInflightTransactions: 1,
	MaxDispatchAhead:        1,
}
