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

package rpcclient

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/tls"
	"github.com/stretchr/testify/assert"
)

func TestWSConfigOK(t *testing.T) {
	ctx := context.Background()
	wsc, err := ParseWSConfig(ctx, &WSConfig{HTTPConfig: HTTPConfig{URL: "ws://localhost:8545"}})
	assert.NoError(t, err)
	assert.Equal(t, "ws://localhost:8545", wsc.WebSocketURL)
}

func TestWSConfigTLSOK(t *testing.T) {
	ctx := context.Background()
	wsc, err := ParseWSConfig(ctx, &WSConfig{HTTPConfig: HTTPConfig{URL: "wss://localhost:8545"}})
	assert.NoError(t, err)
	assert.Equal(t, "wss://localhost:8545", wsc.WebSocketURL)
	assert.NotNil(t, wsc.TLSClientConfig)
}

func TestWSConfigBadURL(t *testing.T) {
	ctx := context.Background()
	_, err := ParseWSConfig(ctx, &WSConfig{HTTPConfig: HTTPConfig{URL: "http://localhost:8545"}})
	assert.Regexp(t, "PD011513", err)
}

func TestWSConfigBadTLS(t *testing.T) {
	ctx := context.Background()
	_, err := ParseWSConfig(ctx, &WSConfig{HTTPConfig: HTTPConfig{URL: "wss://localhost:8545", TLS: tls.Config{CAFile: t.TempDir()}}})
	assert.Regexp(t, "PD010901", err)
}

func TestHTTPonfigOK(t *testing.T) {
	ctx := context.Background()
	r, err := ParseHTTPConfig(ctx, &HTTPConfig{URL: "http://localhost:8545"})
	assert.NoError(t, err)
	assert.Equal(t, "http://localhost:8545", r.BaseURL)
}

func TestHTTPConfigTLSOK(t *testing.T) {
	ctx := context.Background()
	r, err := ParseHTTPConfig(ctx, &HTTPConfig{URL: "https://localhost:8545"})
	assert.NoError(t, err)
	assert.Equal(t, "https://localhost:8545", r.BaseURL)
}

func TestHTTPConfigBadURL(t *testing.T) {
	ctx := context.Background()
	_, err := ParseHTTPConfig(ctx, &HTTPConfig{URL: "wss://localhost:8545"})
	assert.Regexp(t, "PD011514", err)
}

func TestHTTPConfigBadTLS(t *testing.T) {
	ctx := context.Background()
	_, err := ParseHTTPConfig(ctx, &HTTPConfig{URL: "https://localhost:8545", TLS: tls.Config{CAFile: t.TempDir()}})
	assert.Regexp(t, "PD010901", err)
}
