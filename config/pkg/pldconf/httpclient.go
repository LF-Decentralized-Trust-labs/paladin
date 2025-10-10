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

type HTTPBasicAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type HTTPRetryConfig struct {
	Enabled          bool    `json:"enabled"`
	Count            *int    `json:"count,omitempty"`
	InitialDelay     *string `json:"initialDelay,omitempty"`
	MaximumDelay     *string `json:"maximumDelay,omitempty"`
	ErrorStatusCodes string  `json:"errorStatusCodes,omitempty"` // a regex string to match against the status codes which should be retried
}

type HTTPClientConfig struct {
	URL               string                 `json:"url"`
	HTTPHeaders       map[string]interface{} `json:"httpHeaders"`
	Auth              HTTPBasicAuthConfig    `json:"auth"`
	TLS               TLSConfig              `json:"tls"`
	Retry             HTTPRetryConfig        `json:"retry,omitempty"`
	RequestTimeout    *string                `json:"requestTimeout,omitempty"`
	ConnectionTimeout *string                `json:"connectionTimeout,omitempty"`
}

var DefaultHTTPConfig = &HTTPClientConfig{
	ConnectionTimeout: confutil.P("30s"),
	RequestTimeout:    confutil.P("30s"),
	Retry: HTTPRetryConfig{
		Enabled:      false,
		Count:        confutil.P(5),
		InitialDelay: confutil.P("250ms"),
		MaximumDelay: confutil.P("30s"),
	},
}
