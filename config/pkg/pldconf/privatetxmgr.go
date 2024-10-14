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

import "github.com/kaleido-io/paladin/config/pkg/confutil"

type PrivateTxManagerConfig struct {
	Writer           FlushWriterConfig                  `json:"writer"`
	Orchestrator     PrivateTxManagerOrchestratorConfig `json:"orchestrator"`
	StateDistributer StateDistributerConfig             `json:"stateDistributer"`
}

type StateDistributerConfig struct {
	AcknowledgementWriter FlushWriterConfig `json:"acknowledgementWriter"`
	ReceivedStateWriter   FlushWriterConfig `json:"receivedStateWriter"`
}

var PrivateTxManagerDefaults = &PrivateTxManagerConfig{
	Orchestrator: PrivateTxManagerOrchestratorConfig{
		MaxConcurrentProcess:    confutil.P(500),
		EvaluationInterval:      confutil.P("5m"),
		PersistenceRetryTimeout: confutil.P("5s"),
		StaleTimeout:            confutil.P("10m"),
		MaxPendingEvents:        confutil.P(500),
	},
}

type PrivateTxManagerOrchestratorConfig struct {
	MaxConcurrentProcess    *int    `json:"maxConcurrentProcess,omitempty"`
	MaxPendingEvents        *int    `json:"maxPendingEvents,omitempty"`
	EvaluationInterval      *string `json:"evalInterval,omitempty"`
	PersistenceRetryTimeout *string `json:"persistenceRetryTimeout,omitempty"`
	StaleTimeout            *string `json:"staleTimeout,omitempty"`
}
