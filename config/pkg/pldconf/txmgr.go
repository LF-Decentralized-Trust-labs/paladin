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

package pldconf

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
)

type TxManagerConfig struct {
	ABI              ABIConfig          `json:"abi"`
	Transactions     TransactionsConfig `json:"transactions"`
	ReceiptListeners ReceiptListeners   `json:"receiptListeners"`
}

type ABIConfig struct {
	Cache CacheConfig `json:"cache"`
}

type TransactionsConfig struct {
	Cache CacheConfig `json:"cache"`
}

type ReceiptListeners struct {
	Retry                 RetryConfig `json:"retry"`
	ReadPageSize          *int        `json:"readPageSize"`
	StateGapCheckInterval *string     `json:"stateGapCheckInterval"`
}

var TxManagerDefaults = &TxManagerConfig{
	ABI: ABIConfig{
		Cache: CacheConfig{
			Capacity: confutil.P(100),
		},
	},
	Transactions: TransactionsConfig{
		Cache: CacheConfig{
			Capacity: confutil.P(100),
		},
	},
	ReceiptListeners: ReceiptListeners{
		Retry:                 GenericRetryDefaults.RetryConfig,
		ReadPageSize:          confutil.P(100),
		StateGapCheckInterval: confutil.P("1s"),
	},
}
