// Copyright contributors to Paladin, an LFDT project
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

package rpcserver

import (
	"net/http/httptest"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
)

func TestWSCheckOrigin_CORSDisabled(t *testing.T) {
	check := newWSCheckOrigin(&pldconf.CORSConfig{
		Enabled:        false,
		AllowedOrigins: []string{"http://localhost:31549"},
	})

	req := httptest.NewRequest("GET", "http://localhost:31549/", nil)
	req.Host = "localhost:31549"
	req.Header.Set("Origin", "http://localhost:31549")
	assert.True(t, check(req))

	req.Header.Set("Origin", "http://localhost:3000")
	assert.False(t, check(req))

	req.Header.Del("Origin")
	assert.True(t, check(req))
}

func TestWSCheckOrigin_CORSEnabledSpecificOrigin(t *testing.T) {
	check := newWSCheckOrigin(&pldconf.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	req := httptest.NewRequest("GET", "http://localhost:31549/", nil)
	req.Host = "localhost:31549"
	req.Header.Set("Origin", "http://localhost:3000")
	assert.True(t, check(req))

	req.Header.Set("Origin", "http://evil.example")
	assert.False(t, check(req))

	req.Header.Del("Origin")
	assert.True(t, check(req))
}

func TestWSCheckOrigin_DefaultWhenUnset(t *testing.T) {
	check := newWSCheckOrigin(&pldconf.CORSConfig{})

	req := httptest.NewRequest("GET", "http://localhost:31549/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	assert.True(t, check(req))
}

func TestWSCheckOrigin_ExplicitDisable(t *testing.T) {
	check := newWSCheckOrigin(&pldconf.CORSConfig{
		Enabled:        false,
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	req := httptest.NewRequest("GET", "http://localhost:31549/", nil)
	req.Host = "localhost:31549"
	req.Header.Set("Origin", "http://localhost:3000")
	assert.False(t, check(req))
}

func TestWSCheckOrigin_CORSEnabledWildcard(t *testing.T) {
	check := newWSCheckOrigin(&pldconf.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
	})

	req := httptest.NewRequest("GET", "http://localhost:31549/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	assert.True(t, check(req))
}

func TestWSCheckOrigin_InvalidOriginURL(t *testing.T) {
	check := newWSCheckOrigin(&pldconf.CORSConfig{
		Enabled:        false,
		AllowedOrigins: []string{"http://localhost:31549"},
	})

	req := httptest.NewRequest("GET", "http://localhost:31549/", nil)
	req.Host = "localhost:31549"
	req.Header.Set("Origin", "://invalid")
	assert.False(t, check(req))
}

func TestWSCheckOrigin_SameOriginCaseInsensitive(t *testing.T) {
	check := newWSCheckOrigin(&pldconf.CORSConfig{
		Enabled:        false,
		AllowedOrigins: []string{"http://localhost:31549"},
	})

	req := httptest.NewRequest("GET", "http://localhost:31549/", nil)
	req.Host = "LOCALHOST:31549"
	req.Header.Set("Origin", "http://localhost:31549")
	assert.True(t, check(req))
}
