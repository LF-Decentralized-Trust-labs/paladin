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
	"github.com/kaleido-io/paladin/config/pkg/confutil"
)

type HTTPBasicAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type HTTPProxyConfig struct {
	URL string `json:"url"`
}

type HTTPRetryConfig struct {
	Count                *int    `json:"count,omitempty"`
	Enabled              bool    `json:"enabled,omitempty"`
	ErrorStatusCodeRegex string  `json:"errorStatusCodeRegex,omitempty"`
	InitialDelay         *string `json:"initialDelay,omitempty"`
	MaximumDelay         *string `json:"maxDelay,omitempty"`
}

type HTTPThrottleConfig struct {
	Burst             *int `json:"burst,omitempty"`
	RequestsPerSecond *int `json:"requestsPerSecond,omitempty"`
}

type HTTPClientConfig struct {
	URL                   string                 `json:"url"`
	HTTPHeaders           map[string]interface{} `json:"httpHeaders"`
	Auth                  HTTPBasicAuthConfig    `json:"auth"`
	TLS                   TLSConfig              `json:"tls"`
	Proxy                 HTTPProxyConfig        `json:"proxy"`
	Retry                 HTTPRetryConfig        `json:"retry"`
	Throttle              HTTPThrottleConfig     `json:"throttle"`
	RequestTimeout        *string                `json:"requestTimeout,omitempty"`
	ConnectionTimeout     *string                `json:"connectionTimeout,omitempty"`
	ExpectContinueTimeout *string                `json:"expectContinueTimeout,omitempty"`
	IdleConnTimeout       *string                `json:"idleConnTimeout,omitempty"`
	MaxConnsPerHost       *int                   `json:"maxConnsPerHost,omitempty"`
	MaxIdleConns          *int                   `json:"maxIdleConns,omitempty"`
	TLSHandshakeTimeout   *string                `json:"tlsHandshakeTimeout,omitempty"`
}

var DefaultHTTPConfig = &HTTPClientConfig{
	ConnectionTimeout:     confutil.P("30s"),
	RequestTimeout:        confutil.P("30s"),
	ExpectContinueTimeout: confutil.P("1s"),
	IdleConnTimeout:       confutil.P("475ms"),
	MaxConnsPerHost:       confutil.P(0),
	MaxIdleConns:          confutil.P(100),
	TLSHandshakeTimeout:   confutil.P("10s"),
	Retry: HTTPRetryConfig{
		Count:        confutil.P(5),
		InitialDelay: confutil.P("250s"),
		MaximumDelay: confutil.P("30s"),
	},
}
