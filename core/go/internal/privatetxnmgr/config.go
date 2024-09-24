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
package privatetxnmgr

import (
	"github.com/kaleido-io/paladin/core/internal/flushwriter"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
)

type Config struct {
	Writer       flushwriter.Config `yaml:"writer"`
	Orchestrator OrchestratorConfig `yaml:"orchestrator"`
}

type OrchestratorConfig struct {
	MaxConcurrentProcess    *int    `yaml:"maxConcurrentProcess,omitempty"`
	MaxPendingEvents        *int    `yaml:"maxPendingEvents,omitempty"`
	EvaluationInterval      *string `yaml:"evalInterval,omitempty"`
	PersistenceRetryTimeout *string `yaml:"persistenceRetryTimeout,omitempty"`
	StaleTimeout            *string `yaml:"staleTimeout,omitempty"`
}

var orchestratorConfigDefault = OrchestratorConfig{
	MaxConcurrentProcess:    confutil.P(500),
	EvaluationInterval:      confutil.P("5m"),
	PersistenceRetryTimeout: confutil.P("5s"),
	StaleTimeout:            confutil.P("10m"),
	MaxPendingEvents:        confutil.P(500),
}
