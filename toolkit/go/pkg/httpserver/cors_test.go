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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorsWrapperDisabled(t *testing.T) {
	hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CalledServer", "true")
	})
	s := httptest.NewServer(WrapCorsIfEnabled(context.Background(), hf, &pldconf.CORSConfig{}))

	req, err := http.NewRequest(http.MethodOptions, s.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://some.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "header1")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "true", res.Header.Get("CalledServer"))
}

func TestCorsWrapperEnabledWildcardPreflight(t *testing.T) {
	hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CalledServer", "true")
	})
	s := httptest.NewServer(WrapCorsIfEnabled(context.Background(), hf, &pldconf.CORSConfig{
		Enabled: true,
		Debug:   true,
	}))

	req, err := http.NewRequest(http.MethodOptions, s.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://some.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "header1")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, res.StatusCode)
	assert.Empty(t, "", res.Header.Get("CalledServer"))
	assert.Equal(t, "", res.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, []string{"Origin, Access-Control-Request-Method, Access-Control-Request-Headers"}, res.Header["Vary"])
}

func TestCorsWrapperEnabledHostOk(t *testing.T) {
	hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CalledServer", "true")
	})
	s := httptest.NewServer(WrapCorsIfEnabled(context.Background(), hf, &pldconf.CORSConfig{
		Enabled:        true,
		Debug:          true,
		AllowedOrigins: []string{"https://some.example"},
	}))

	req, err := http.NewRequest(http.MethodGet, s.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://some.example")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "true", res.Header.Get("CalledServer"))
	assert.Equal(t, "https://some.example", res.Header.Get("Access-Control-Allow-Origin"))
}

func TestCorsWrapperEnabledHostFail(t *testing.T) {
	hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CalledServer", "true")
	})
	s := httptest.NewServer(WrapCorsIfEnabled(context.Background(), hf, &pldconf.CORSConfig{
		Enabled:        true,
		Debug:          true,
		AllowedOrigins: []string{"https://some.example"},
	}))

	req, err := http.NewRequest(http.MethodGet, s.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://another.example")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	// The server still gets called
	assert.Equal(t, "true", res.Header.Get("CalledServer"))
	// But the browser does not get the header to trust it
	assert.Empty(t, res.Header.Get("Access-Control-Allow-Origin"))
}
