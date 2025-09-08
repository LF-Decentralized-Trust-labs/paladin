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

type HTTPServerConfig struct {
	TLS                   TLSConfig  `json:"tls"`
	CORS                  CORSConfig `json:"cors"`
	Address               *string    `json:"address"`
	Port                  *int       `json:"port"`
	DefaultRequestTimeout *string    `json:"defaultRequestTimeout"`
	MaxRequestTimeout     *string    `json:"maxRequestTimeout"`
	ReadTimeout           *string    `json:"readTimeout"`
	WriteTimeout          *string    `json:"writeTimeout"`
	ShutdownTimeout       *string    `json:"shutdownTimeout"`
}

var HTTPDefaults = &HTTPServerConfig{
	Address:               confutil.P("127.0.0.1"),
	DefaultRequestTimeout: confutil.P("2m"),
	MaxRequestTimeout:     confutil.P("10m"),
	ShutdownTimeout:       confutil.P("10s"),
}

type CORSConfig struct {
	Enabled          bool     `json:"enabled"`
	Debug            bool     `json:"debug"`
	AllowCredentials *bool    `json:"allowCredentials"`
	AllowedHeaders   []string `json:"allowedHeaders"`
	AllowedMethods   []string `json:"allowedMethods"`
	AllowedOrigins   []string `json:"allowedOrigins"`
	MaxAge           *string  `json:"maxAge"`
}
type StaticServerConfig struct {
	Enabled      bool   `json:"enabled"`
	StaticPath   string `json:"staticPath"`   // Path to the static files in the server FS e.g /app/ui
	URLPath      string `json:"urlPath"`      // URL path to serve the static files e.g /ui -> http://host:port/ui
	BaseRedirect string `json:"baseRedirect"` // if the URL path is hit directly without a filename, this redirect is sent
}

type DebugServerConfig struct {
	Enabled *bool `json:"enabled"`
	HTTPServerConfig
}

var DebugServerDefaults = &DebugServerConfig{
	Enabled: confutil.P(false),
}

type MetricsServerConfig struct {
	Enabled *bool `json:"enabled"`
	HTTPServerConfig
}

var MetricsServerDefaults = &MetricsServerConfig{
	Enabled: confutil.P(false),
}
