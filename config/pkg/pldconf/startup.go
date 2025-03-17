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

package pldconf

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
)

type StartupConfig struct {
	BlockchainConnectRetry RetryConfigWithMax `json:"blockchainConnectRetry"`
}

var StartupConfigDefaults = StartupConfig{
	BlockchainConnectRetry: RetryConfigWithMax{
		RetryConfig: RetryConfig{
			InitialDelay: confutil.P("500ms"),
			MaxDelay:     confutil.P("2s"),
			Factor:       confutil.P(2.0),
		},
		MaxAttempts: confutil.P(10),
	},
}
