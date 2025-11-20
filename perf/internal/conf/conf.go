// Copyright © 2025 Kaleido, Inc.
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

package conf

import (
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
)

type RunnerConfig struct {
	LogLevel                string
	Test                    TestCaseConfig
	Length                  time.Duration
	WSConfig                pldconf.WSClientConfig
	HTTPConfig              pldconf.HTTPClientConfig
	DelinquentAction        DelinquentAction
	Daemon                  bool
	LogEvents               bool
	MaxTimePerAction        time.Duration
	MaxActions              int64
	RampLength              time.Duration
	MaxSubmissionsPerSecond int
	CompletionTimeout       time.Duration
	NoWaitSubmission        bool
	NodeKillConfig          *NodeKillConfig
	Nodes                   []NodeConfig
}

type PerformanceTestConfig struct {
	LogLevel   string                   `json:"logLevel" yaml:"logLevel"`
	Instances  []InstanceConfig         `json:"instances" yaml:"instances"`
	WSConfig   pldconf.WSClientConfig   `json:"wsConfig,omitempty" yaml:"wsConfig,omitempty"`
	HTTPConfig pldconf.HTTPClientConfig `json:"httpConfig,omitempty" yaml:"httpConfig,omitempty"`
	Daemon     bool                     `json:"daemon,omitempty" yaml:"daemon,omitempty"`
	Nodes      []NodeConfig             `json:"nodes" yaml:"nodes"`
	LogEvents  bool                     `json:"logEvents,omitempty" yaml:"logEvents,omitempty"`
}

type InstanceConfig struct {
	Name                    string           `json:"name" yaml:"name"`
	Test                    TestCaseConfig   `json:"test" yaml:"test"`
	Length                  time.Duration    `json:"length" yaml:"length"`
	MaxTimePerAction        time.Duration    `json:"maxTimePerAction,omitempty" yaml:"maxTimePerAction,omitempty"`
	MaxActions              int64            `json:"maxActions,omitempty" yaml:"maxActions,omitempty"`
	RampLength              time.Duration    `json:"rampLength,omitempty" yaml:"rampLength,omitempty"`
	MaxSubmissionsPerSecond int              `json:"maxSubmissionsPerSecond" yaml:"maxSubmissionsPerSecond"`
	DelinquentAction        DelinquentAction `json:"delinquentAction,omitempty" yaml:"delinquentAction,omitempty"`
	CompletionTimeout       time.Duration    `json:"completionTimeout,omitempty" yaml:"completionTimeout,omitempty"`
	NoWaitSubmission        bool             `json:"noWaitSubmission,omitempty" yaml:"noWaitSubmission,omitempty"`
	NodeKillConfig          *NodeKillConfig  `json:"nodeKillConfig,omitempty" yaml:"nodeKillConfig,omitempty"`
}

type TestCaseConfig struct {
	Name           TestName `json:"name" yaml:"name"`
	Workers        int      `json:"workers" yaml:"workers"`
	ActionsPerLoop int      `json:"actionsPerLoop" yaml:"actionsPerLoop"`
}

type NodeConfig struct {
	Name         string `json:"name" yaml:"name"`
	HTTPEndpoint string `json:"httpEndpoint" yaml:"httpEndpoint"`
	WSEndpoint   string `json:"wsEndpoint" yaml:"wsEndpoint"`
}

type NodeKillConfig struct {
	KillCommandTemplate string        `json:"killCommandTemplate,omitempty" yaml:"killCommandTemplate,omitempty"`
	HealthCheckCommand  string        `json:"healthCheckCommand,omitempty" yaml:"healthCheckCommand,omitempty"`
	HealthCheckTemplate string        `json:"healthCheckTemplate,omitempty" yaml:"healthCheckTemplate,omitempty"`
	RestartTimeout      time.Duration `json:"restartTimeout,omitempty" yaml:"restartTimeout,omitempty"`
	KillInterval        time.Duration `json:"killInterval,omitempty" yaml:"killInterval,omitempty"`
}

type TestName string

const (
	// PerfTestPublicContract invokes a public smart contract and checks for transaction receipts
	PerfTestPublicContract TestName = "public_contract"
	// PerfTestPrivateTransactionNodeRestart drives pente transactions across nodes, kills a node, and verifies recovery
	PerfTestPrivateTransactionNodeRestart TestName = "private_transaction_node_restart"
)

type DelinquentAction string

const (
	// DelinquentActionExit causes paladin perf to exit after detecting delinquent messages
	DelinquentActionExit DelinquentAction = "exit"
	// DelinquentActionLog causes paladin perf to log and move on after delinquent messages
	DelinquentActionLog DelinquentAction = "log"
)
