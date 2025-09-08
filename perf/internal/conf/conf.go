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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
)

type RunnerConfig struct {
	LogLevel                string
	Tests                   []TestCaseConfig
	Length                  time.Duration
	SigningKey              string
	ContractOptions         ContractOptions
	WSConfig                pldconf.WSClientConfig
	HTTPConfig              pldconf.HTTPClientConfig
	DelinquentAction        DelinquentAction
	Daemon                  bool
	LogEvents               bool
	MaxTimePerAction        time.Duration
	MaxActions              int64
	RampLength              time.Duration
	NoWaitSubmission        bool
	MaxSubmissionsPerSecond int
}

type PerformanceTestConfig struct {
	LogLevel   string                   `json:"logLevel"`
	Instances  []InstanceConfig         `json:"instances"`
	WSConfig   pldconf.WSClientConfig   `json:"wsConfig,omitempty"`
	HTTPConfig pldconf.HTTPClientConfig `json:"httpConfig,omitempty"`
	Daemon     bool                     `json:"daemon,omitempty"`
	Nodes      []NodeConfig             `json:"nodes"`
	LogEvents  bool                     `json:"logEvents,omitempty"`
}

type InstanceConfig struct {
	Name                    string           `json:"name"`
	Tests                   []TestCaseConfig `json:"tests"`
	Length                  time.Duration    `json:"length"`
	NodeIndex               int              `json:"nodeIndex"`
	SigningKey              string           `json:"signingKey,omitempty"`
	ContractOptions         ContractOptions  `json:"contractOptions,omitempty"`
	MaxTimePerAction        time.Duration    `json:"maxTimePerAction,omitempty"`
	MaxActions              int64            `json:"maxActions,omitempty"`
	RampLength              time.Duration    `json:"rampLength,omitempty"`
	NoWaitSubmission        bool             `json:"noWaitSubmission"`
	MaxSubmissionsPerSecond int              `json:"maxSubmissionsPerSecond"`
	DelinquentAction        DelinquentAction `json:"delinquentAction,omitempty"`
}

type TestCaseConfig struct {
	Name           TestName `json:"name"`
	Workers        int      `json:"workers"`
	ActionsPerLoop int      `json:"actionsPerLoop"`
}

type NodeConfig struct {
	Name         string `json:"name"`
	HTTPEndpoint string `json:"httpEndpoint"`
	WSEndpoint   string `json:"wsEndpoint"`
}

type ContractOptions struct {
	Address string `json:"address"`
}

type TestName string

const (
	// PerfTestPublicContract invokes a public smart contract and checks for transaction receipts
	PerfTestPublicContract TestName = "public_contract"
)

type DelinquentAction string

const (
	// DelinquentActionExit causes paladin perf to exit after detecting delinquent messages
	DelinquentActionExit DelinquentAction = "exit"
	// DelinquentActionLog causes paladin perf to log and move on after delinquent messages
	DelinquentActionLog DelinquentAction = "log"
)
