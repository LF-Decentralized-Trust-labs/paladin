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

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWSConfigOK(t *testing.T) {
	ctx := context.Background()
	wsc, err := NewWSClient(ctx, &pldconf.WSClientConfig{HTTPClientConfig: pldconf.HTTPClientConfig{URL: "ws://localhost:8545"}})
	require.NoError(t, err)
	assert.Equal(t, "ws://localhost:8545", wsc.(*wsRPCClient).wsConf.WebSocketURL)
}

func TestWSConfigTLSOK(t *testing.T) {
	ctx := context.Background()
	wsc, err := ParseWSConfig(ctx, &pldconf.WSClientConfig{HTTPClientConfig: pldconf.HTTPClientConfig{URL: "wss://localhost:8545"}})
	require.NoError(t, err)
	assert.Equal(t, "wss://localhost:8545", wsc.WebSocketURL)
	assert.NotNil(t, wsc.TLSClientConfig)
}

func TestWSConfigBadURL(t *testing.T) {
	ctx := context.Background()
	_, err := NewWSClient(ctx, &pldconf.WSClientConfig{HTTPClientConfig: pldconf.HTTPClientConfig{URL: "http://localhost:8545"}})
	assert.Regexp(t, "PD020500", err)
}

func TestWSConfigBadTLS(t *testing.T) {
	ctx := context.Background()
	_, err := NewWSClient(ctx, &pldconf.WSClientConfig{HTTPClientConfig: pldconf.HTTPClientConfig{URL: "wss://localhost:8545", TLS: pldconf.TLSConfig{CAFile: t.TempDir()}}})
	assert.Regexp(t, "PD020401", err)
}

func TestHTTPConfigBadURL(t *testing.T) {
	ctx := context.Background()
	_, err := NewHTTPClient(ctx, &pldconf.HTTPClientConfig{URL: "wss://localhost:8545"})
	assert.Regexp(t, "PD020501", err)
}

func TestHTTPConfigBadTLS(t *testing.T) {
	ctx := context.Background()
	_, err := NewHTTPClient(ctx, &pldconf.HTTPClientConfig{URL: "https://localhost:8545", TLS: pldconf.TLSConfig{CAFile: t.TempDir()}})
	assert.Regexp(t, "PD020401", err)
}
