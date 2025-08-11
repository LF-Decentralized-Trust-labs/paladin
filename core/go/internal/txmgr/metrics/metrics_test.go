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

package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestInitMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncRpc("rpcMethod1")

	metrics.IncRpc("rpcMethod2")
	metrics.IncRpc("rpcMethod2")

	metrics.IncRpc("rpcMethod3")
	metrics.IncRpc("rpcMethod3")
	metrics.IncRpc("rpcMethod3")

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Assembled transactions metric
	assert.Equal(t, metricFamilies[0].GetName(), "transaction_manager_rpc_total")

	assert.Equal(t, metricFamilies[0].GetMetric()[0].GetCounter().GetValue(), float64(1))
	assert.Equal(t, metricFamilies[0].GetMetric()[0].GetLabel()[0].GetValue(), "rpcMethod1")
	assert.Equal(t, metricFamilies[0].GetMetric()[1].GetCounter().GetValue(), float64(2))
	assert.Equal(t, metricFamilies[0].GetMetric()[1].GetLabel()[0].GetValue(), "rpcMethod2")
	assert.Equal(t, metricFamilies[0].GetMetric()[2].GetCounter().GetValue(), float64(3))
	assert.Equal(t, metricFamilies[0].GetMetric()[2].GetLabel()[0].GetValue(), "rpcMethod3")
}
