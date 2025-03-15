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
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "pldperf",
	Short: "A CLI tool to generate synthetic load against and triaging performance issues with a Paladin node",
	Long:  "Paladin Performance CLI is a tool to generate synthetic load against and triaging performance issues with a Paladin node.",
}

func init() {
	viper.SetEnvPrefix("PP")
	viper.AutomaticEnv()

	logger := &log.Logger{
		Out:   os.Stderr,
		Level: log.DebugLevel,
		Formatter: &log.TextFormatter{
			DisableSorting:  false,
			ForceColors:     true,
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02T15:04:05.000",
		},
	}
	log.SetFormatter(logger.Formatter)
}

func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		log.Errorln(err)
		return 1
	}
	return 0
}
