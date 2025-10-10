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

type WSClientConfig struct {
	HTTPClientConfig       `json:",inline"`
	InitialConnectAttempts *int        `json:"initialConnectAttempts"`
	ConnectionTimeout      *string     `json:"connectionTimeout"`
	ConnectRetry           RetryConfig `json:"connectRetry"`
	ReadBufferSize         *string     `json:"readBufferSize"`
	WriteBufferSize        *string     `json:"writeBufferSize"`
	HeartbeatInterval      *string     `json:"heartbeatInterval"`
}

var DefaultWSConfig = &WSClientConfig{
	ReadBufferSize:         confutil.P("16Kb"),
	WriteBufferSize:        confutil.P("16Kb"),
	InitialConnectAttempts: confutil.P(0),
	ConnectionTimeout:      confutil.P("30s"),
	HeartbeatInterval:      confutil.P("15s"),
	ConnectRetry:           GenericRetryDefaults.RetryConfig,
}
