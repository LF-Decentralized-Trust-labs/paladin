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

import "github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"

type TransportManagerConfig struct {
	NodeName              string                      `json:"nodeName"`
	SendQueueLen          *int                        `json:"sendQueueLen"`
	PeerInactivityTimeout *string                     `json:"peerInactivityTimeout"`
	PeerReaperInterval    *string                     `json:"peerReaperInterval"`
	SendRetry             RetryConfigWithMax          `json:"sendRetry"`
	ReliableScanRetry     RetryConfig                 `json:"reliableScanRetry"`
	ReliableMessageResend *string                     `json:"reliableMessageResend"`
	ReliableMessageWriter FlushWriterConfig           `json:"reliableMessageWriter"`
	Transports            map[string]*TransportConfig `json:"transports"`
}

type TransportInitConfig struct {
	Retry RetryConfig `json:"retry"`
}

var TransportManagerDefaults = &TransportManagerConfig{
	SendQueueLen:          confutil.P(10),
	ReliableMessageResend: confutil.P("30s"),
	PeerInactivityTimeout: confutil.P("1m"),
	PeerReaperInterval:    confutil.P("30s"),
	ReliableScanRetry:     GenericRetryDefaults.RetryConfig,
	// SendRetry defaults are deliberately short
	SendRetry: RetryConfigWithMax{
		RetryConfig: RetryConfig{
			InitialDelay: confutil.P("50ms"),
			MaxDelay:     confutil.P("1s"),
			Factor:       confutil.P(2.0),
		},
		MaxAttempts: confutil.P(3),
	},
	ReliableMessageWriter: FlushWriterConfig{
		WorkerCount:  confutil.P(1),
		BatchTimeout: confutil.P("250ms"),
		BatchMaxSize: confutil.P(50),
	},
}

type TransportConfig struct {
	Init   TransportInitConfig `json:"init"`
	Plugin PluginConfig        `json:"plugin"`
	Config map[string]any      `json:"config"`
}
