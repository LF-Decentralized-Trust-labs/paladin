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

package httpserver

import (
	"github.com/kaleido-io/paladin/kata/internal/tls"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
)

type Config struct {
	TLS                   tls.Config `yaml:"tls"`
	CORS                  CORSConfig `yaml:"cors"`
	Address               *string    `yaml:"address"`
	Port                  *int       `yaml:"port"`
	DefaultRequestTimeout *string    `yaml:"defaultRequestTimeout"`
	MaxRequestTimeout     *string    `yaml:"maxRequestTimeout"`
	ReadTimeout           *string    `yaml:"readTimeout"`
	WriteTimeout          *string    `yaml:"writeTimeout"`
	ShutdownTimeout       *string    `yaml:"shutdownTimeout"`
}

var HTTPDefaults = &Config{
	Address:               confutil.P("127.0.0.1"),
	DefaultRequestTimeout: confutil.P("2m"),
	MaxRequestTimeout:     confutil.P("10m"),
	ShutdownTimeout:       confutil.P("10s"),
}

type CORSConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Debug            bool     `yaml:"debug"`
	AllowCredentials *bool    `yaml:"allowCredentials"`
	AllowedHeaders   []string `yaml:"allowedHeaders"`
	AllowedMethods   []string `yaml:"allowedMethods"`
	AllowedOrigins   []string `yaml:"allowedOrigins"`
	MaxAge           *string  `yaml:"maxAge"`
}
