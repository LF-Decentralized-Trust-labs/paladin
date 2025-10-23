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

type PublicTxManagerConfig struct {
	Manager        PublicTxManagerManagerConfig      `json:"manager"`
	Orchestrator   PublicTxManagerOrchestratorConfig `json:"orchestrator"`
	GasPrice       GasPriceConfig                    `json:"gasPrice"`
	BalanceManager BalanceManagerConfig              `json:"balanceManager"`
	GasLimit       GasLimitConfig                    `json:"gasLimit"`
}

var PublicTxManagerDefaults = PublicTxManagerConfig{
	Manager: PublicTxManagerManagerConfig{
		MaxInFlightOrchestrators: confutil.P(50),
		Interval:                 confutil.P("5s"),
		OrchestratorIdleTimeout:  confutil.P("1s"),
		OrchestratorStaleTimeout: confutil.P("5m"),
		OrchestratorSwapTimeout:  confutil.P("10m"),
		NonceCacheTimeout:        confutil.P("1h"),
		Retry: RetryConfig{
			InitialDelay: confutil.P("250ms"),
			MaxDelay:     confutil.P("30s"),
			Factor:       confutil.P(2.0),
		},
		SubmissionWriter: FlushWriterConfig{
			WorkerCount:  confutil.P(5),
			BatchTimeout: confutil.P("75ms"),
			BatchMaxSize: confutil.P(50),
		},
		ActivityRecords: PublicTxManagerActivityRecordsConfig{
			CacheConfig: CacheConfig{
				// Status cache can be is shared across orchestrators, allowing status to live beyond TX completion
				// while still only being in memory
				Capacity: confutil.P(1000),
			},
			RecordsPerTransaction: confutil.P(25),
		},
	},
	Orchestrator: PublicTxManagerOrchestratorConfig{
		MaxInFlight:          confutil.P(500),
		Interval:             confutil.P("5s"),
		ResubmitInterval:     confutil.P("5m"),
		StaleTimeout:         confutil.P("5m"),
		StageRetryTime:       confutil.P("10s"),
		PersistenceRetryTime: confutil.P("5s"),
		SubmissionRetry: RetryConfigWithMax{
			RetryConfig: RetryConfig{
				InitialDelay: confutil.P("250ms"),
				MaxDelay:     confutil.P("10s"),
				Factor:       confutil.P(4.0),
			},
			MaxAttempts: confutil.P(3),
		},
	},
	// Gas price defaults are optimised for getting transactions onto chain as easily as possible
	// When spending real gas, a user might want to be more conservative with setting caps or not allowing the fixed price to be increased
	GasPrice: GasPriceConfig{
		IncreasePercentage:      confutil.P(10), // default to 10% increase - this is what Besu and other blockchains define as the minimum increase for a replacement transaction
		MaxPriorityFeePerGasCap: nil,            // No cap by default
		MaxFeePerGasCap:         nil,            // No cap by default
		FixedGasPrice:           nil,
		EthFeeHistory: EthFeeHistoryConfig{
			PriorityFeePercentile: confutil.P(85), // Default to 85th percentile for getting transactions onto chain as easily as possible
			HistoryBlockCount:     confutil.P(20), // Default to 20 blocks for fee history
			BaseFeeBufferFactor:   confutil.P(1),  // Default to 1x buffer for base fee
			Cache: GasPriceCacheConfig{
				Enabled:     confutil.P(true),  // Default to enabled
				RefreshTime: confutil.P("30s"), // Default to 30 seconds refresh time
			},
		},
		GasOracleAPI: &GasOracleAPIConfig{
			HTTPClientConfig: DefaultHTTPConfig,
			Method:           confutil.P("GET"), // Default to GET method
			Cache: GasPriceCacheConfig{
				Enabled:     confutil.P(true),  // Default to enabled
				RefreshTime: confutil.P("30s"), // Default to 30 seconds refresh time
			},
		},
	},
	BalanceManager: BalanceManagerConfig{
		Cache: CacheConfig{
			Capacity: confutil.P(100),
			// TODO: Enable a KB based cache with TTL in Paladin
			// Enabled:  confutil.P(true),
			// Size:     confutil.P("5m"),
			// TTL:      confutil.P("30s"),
		},
	},
	GasLimit: GasLimitConfig{
		GasEstimateFactor: confutil.P(1.5),
	},
}

type PublicTxManagerManagerConfig struct {
	MaxInFlightOrchestrators *int                                 `json:"maxInFlightOrchestrators"`
	Interval                 *string                              `json:"interval"`
	OrchestratorIdleTimeout  *string                              `json:"orchestratorIdleTimeout"`  // idle orchestrators exit after this time
	OrchestratorStaleTimeout *string                              `json:"orchestratorStaleTimeout"` // stale orchestrators exit after this time - TODO: Define stale
	OrchestratorSwapTimeout  *string                              `json:"orchestratorSwapTimeout"`  // orchestrators are cycled out after this time, when all slots are full
	NonceCacheTimeout        *string                              `json:"nonceCacheTimeout"`
	ActivityRecords          PublicTxManagerActivityRecordsConfig `json:"activityRecords"`
	SubmissionWriter         FlushWriterConfig                    `json:"submissionWriter"`
	Retry                    RetryConfig                          `json:"retry"`
}

type PublicTxManagerActivityRecordsConfig struct {
	CacheConfig
	RecordsPerTransaction *int `json:"entriesPerTransaction"`
}

type BalanceManagerConfig struct {
	Cache CacheConfig `json:"cache"`
}

// FixedGasPricing represents EIP-1559 gas pricing configuration
type FixedGasPricing struct {
	MaxFeePerGas         *string `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *string `json:"maxPriorityFeePerGas"`
}

// EthFeeHistoryConfig represents configuration for dynamic EIP-1559 gas pricing using eth_feeHistory
type EthFeeHistoryConfig struct {
	// PriorityFeePercentile for priority fee calculation (0-100)
	PriorityFeePercentile *int `json:"priorityFeePercentile"`

	// Number of historical blocks to query for fee history
	HistoryBlockCount *int `json:"historyBlockCount"`

	// Factor to multiply base fee by for buffering (default: 1)
	BaseFeeBufferFactor *int `json:"baseFeeBufferFactor"`

	// Cache configuration for gas price caching
	Cache GasPriceCacheConfig `json:"cache"`
}

type GasPriceConfig struct {
	IncreasePercentage      *int                `json:"increasePercentage"`
	MaxPriorityFeePerGasCap *string             `json:"maxPriorityFeePerGasCap"`
	MaxFeePerGasCap         *string             `json:"maxFeePerGasCap"`
	FixedGasPrice           *FixedGasPricing    `json:"fixedGasPrice"`
	EthFeeHistory           EthFeeHistoryConfig `json:"ethFeeHistory"`
	GasOracleAPI            *GasOracleAPIConfig `json:"gasOracleAPI"`
}

type GasLimitConfig struct {
	GasEstimateFactor *float64 `json:"gasEstimateFactor"`
}

type GasOracleAPIConfig struct {
	HTTPClientConfig `json:",inline"`
	Method           *string             `json:"method"`
	Body             *string             `json:"body"`
	ResponseTemplate string              `json:"responseTemplate"`
	Cache            GasPriceCacheConfig `json:"cache"`
}

type PublicTxManagerOrchestratorConfig struct {
	MaxInFlight               *int               `json:"maxInFlight"`
	Interval                  *string            `json:"interval"`
	ResubmitInterval          *string            `json:"resubmitInterval"`
	StaleTimeout              *string            `json:"staleTimeout"`
	StageRetryTime            *string            `json:"stageRetryTime"`
	PersistenceRetryTime      *string            `json:"persistenceRetryTime"`
	UnavailableBalanceHandler *string            `json:"unavailableBalanceHandler"`
	SubmissionRetry           RetryConfigWithMax `json:"submissionRetry"`
	TimeLineLoggingMaxEntries int                `json:"timelineMaxEntries"`
}

// GasPriceCacheConfig represents cache configuration for gas price clients
type GasPriceCacheConfig struct {
	// Enabled controls whether caching is enabled
	Enabled *bool `json:"enabled"`

	// RefreshTime specifies how often the cache should be refreshed
	RefreshTime *string `json:"refreshTime"`
}
