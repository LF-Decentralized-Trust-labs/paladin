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

package baseledgertx

import (
	"testing"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestInitConfigOK(t *testing.T) {
	enterpriseTHFactory := TransactionHandlerFactory{}
	conf := config.RootSection("unittest")
	enterpriseTHFactory.InitConfig(conf)

	assert.Equal(t, "enterprise", enterpriseTHFactory.Name())

	// gas price defaults check
	gasPriceConf := conf.SubSection(GasPriceSection)
	assert.Equal(t, defaultGasPriceCacheEnabled, gasPriceConf.GetBool(GasPriceCacheEnabled))
	assert.Equal(t, defaultGasPriceCacheTTL, gasPriceConf.GetString(GasPriceCacheTTLDurationString))

	// transaction controller defaults check
	controllerConf := conf.SubSection(TransactionControllerSection)
	assert.Equal(t, defaultTransactionControllerMaxInFlightEngine, controllerConf.GetInt(TransactionControllerMaxInFlightEngineInt))
	assert.Equal(t, defaultTransactionControllerInterval, controllerConf.GetString(TransactionControllerIntervalDurationString))
	assert.Equal(t, defaultTransactionControllerRetryFactor, controllerConf.GetFloat64(TransactionControllerRetryFactorFloat))
	assert.Equal(t, defaultTransactionControllerRetryInitDelay, controllerConf.GetString(TransactionControllerRetryInitDelayDurationString))
	assert.Equal(t, defaultTransactionControllerRetryMaxDelay, controllerConf.GetString(TransactionControllerRetryMaxDelayDurationString))

	// transaction engine defaults check
	engineConf := conf.SubSection(TransactionEngineSection)
	assert.Equal(t, defaultTransactionEngineInterval, engineConf.GetString(TransactionEngineIntervalDurationString))
	assert.Equal(t, defaultTransactionEngineMaxInFlight, engineConf.GetInt(TransactionEngineMaxInFlightTransactionsInt))
	assert.Equal(t, defaultTransactionEngineResubmitInterval, engineConf.GetString(TransactionEngineResubmitIntervalDurationString))
}
