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

package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/perf/internal/conf"
	"github.com/LF-Decentralized-Trust-labs/paladin/perf/internal/perf"
	"github.com/LF-Decentralized-Trust-labs/paladin/perf/internal/server"
	"github.com/LF-Decentralized-Trust-labs/paladin/perf/internal/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configFilePath string
var instanceName string
var instanceIndex int
var daemonOverride bool
var deliquentAction string

var httpServer *server.HttpServer
var perfRunner perf.PerfRunner

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Executes a instance within a performance test suite to generate synthetic load against a Paladin node",
	Long:  "Executes a instance within a performance test suite to generate synthetic load against a Paladin node",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadConfig(configFilePath)
		if err != nil {
			return err
		}

		if !config.Daemon {
			config.Daemon = daemonOverride
		}

		if instanceName != "" && instanceIndex != -1 {
			log.Warn("both the \"instance-name\" and \"instance-index\" flags were provided, using \"instance-name\"")
		}

		instanceConfig, err := selectInstance(config)
		if err != nil {
			return err
		}

		runnerConfig, err := generateRunnerConfigFromInstance(instanceConfig, config)
		if err != nil {
			return err
		}

		configYaml, err := yaml.Marshal(instanceConfig)
		if err != nil {
			return err
		}

		perfRunner = perf.New(runnerConfig, util.NewReportForTestInstance(string(configYaml), instanceName))
		httpServer = server.NewHttpServer()

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := perfRunner.Init()
		if err != nil {
			return err
		}

		go httpServer.Run()
		return perfRunner.Start()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&configFilePath, "config", "c", "", "Path to performance config that describes the nodes and test instances")
	runCmd.Flags().StringVarP(&instanceName, "instance-name", "n", "", "Instance within performance config to run")
	runCmd.Flags().IntVarP(&instanceIndex, "instance-idx", "i", -1, "Index of the instance within performance config to run")
	runCmd.Flags().BoolVarP(&daemonOverride, "daemon", "d", false, "Run in long-lived, daemon mode. Any provided test length is ignored.")
	runCmd.Flags().StringVarP(&deliquentAction, "delinquent", "", "exit", "Action to take when delinquent messages are detected. Valid options: [exit log]")

	runCmd.MarkFlagRequired("config")
}

func loadConfig(filename string) (*conf.PerformanceTestConfig, error) {
	if d, err := ioutil.ReadFile(filename); err != nil {
		return nil, err
	} else {
		var config *conf.PerformanceTestConfig
		var err error
		if path.Ext(filename) == ".yaml" || path.Ext(filename) == ".yml" {
			err = yaml.Unmarshal(d, &config)
		} else {
			err = json.Unmarshal(d, &config)
		}

		if err != nil {
			return nil, err
		}
		return config, nil
	}
}

func selectInstance(config *conf.PerformanceTestConfig) (*conf.InstanceConfig, error) {
	if instanceName != "" {
		for _, i := range config.Instances {
			if i.Name == instanceName {
				return &i, nil
			}
		}
		return nil, errors.Errorf("did not find instance named \"%s\" within the provided config", instanceName)
	} else if instanceIndex != -1 {
		if instanceIndex >= len(config.Instances) || instanceIndex < 0 {
			return nil, errors.Errorf("provided instance index \"%d\" is outside of the range of instances within the provided config", instanceIndex)
		}
		return &config.Instances[instanceIndex], nil
	}

	return nil, errors.Errorf("please set either the \"instance-name\" or \"instance-index\" ")
}

func generateRunnerConfigFromInstance(instance *conf.InstanceConfig, perfConfig *conf.PerformanceTestConfig) (*conf.RunnerConfig, error) {
	runnerConfig := &conf.RunnerConfig{
		Tests: instance.Tests,
	}

	runnerConfig.HTTPConfig = perfConfig.HTTPConfig
	runnerConfig.WSConfig = perfConfig.WSConfig

	log.Infof("Running test against endpoint \"%s\"\n", perfConfig.Nodes[instance.NodeIndex].HTTPEndpoint)

	runnerConfig.HTTPConfig.URL = perfConfig.Nodes[instance.NodeIndex].HTTPEndpoint
	runnerConfig.WSConfig.URL = perfConfig.Nodes[instance.NodeIndex].WSEndpoint

	runnerConfig.SigningKey = instance.SigningKey
	runnerConfig.ContractOptions = instance.ContractOptions
	runnerConfig.LogLevel = perfConfig.LogLevel
	runnerConfig.NoWaitSubmission = instance.NoWaitSubmission
	runnerConfig.MaxSubmissionsPerSecond = instance.MaxSubmissionsPerSecond
	runnerConfig.Length = instance.Length
	runnerConfig.Daemon = perfConfig.Daemon
	runnerConfig.LogEvents = perfConfig.LogEvents
	runnerConfig.DelinquentAction = conf.DelinquentAction(deliquentAction)
	runnerConfig.MaxTimePerAction = instance.MaxTimePerAction
	runnerConfig.MaxActions = instance.MaxActions
	runnerConfig.RampLength = instance.RampLength

	// If delinquent action has been set on the test run instance this overrides the command line
	if instance.DelinquentAction != "" {
		runnerConfig.DelinquentAction = instance.DelinquentAction
	}

	setDefaults(runnerConfig)

	err := validateConfig(runnerConfig, instance, perfConfig)
	if err != nil {
		return nil, err
	}

	return runnerConfig, nil
}

func setDefaults(runnerConfig *conf.RunnerConfig) {
	if runnerConfig.MaxTimePerAction.Seconds() == 0 {
		runnerConfig.MaxTimePerAction = 60 * time.Second
	}

	for i := range runnerConfig.Tests {
		if runnerConfig.Tests[i].ActionsPerLoop <= 0 {
			runnerConfig.Tests[i].ActionsPerLoop = 1
		}
	}
}

func validateConfig(cfg *conf.RunnerConfig, instance *conf.InstanceConfig, globalConfig *conf.PerformanceTestConfig) error {
	if len(globalConfig.Nodes) > 0 && ((instance.NodeIndex + 1) > len(globalConfig.Nodes)) {
		return fmt.Errorf("NodeIndex %d not valid - only %d nodes have been configured", instance.NodeIndex, len(globalConfig.Nodes))
	}
	return nil
}
