// Copyright © 2024 Kaleido, Inc.
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

package retry

import "github.com/kaleido-io/paladin/kata/internal/confutil"

type Config struct {
	InitialDelay *string  `yaml:"initialDelay"`
	MaxDelay     *string  `yaml:"maxDelay"`
	Factor       *float64 `yaml:"factor"`
}

type ConfigWithMax struct {
	Config
	MaxAttempts *int    `yaml:"maxAttempts"`
	MaxTime     *string `yaml:"maxTime"`
}

var Defaults = &ConfigWithMax{
	Config: Config{
		InitialDelay: confutil.P("250ms"),
		MaxDelay:     confutil.P("30s"),
		Factor:       confutil.P(2.0),
	},
	MaxAttempts: confutil.P(3),
}
