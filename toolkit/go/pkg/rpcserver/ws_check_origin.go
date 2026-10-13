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
	"net/http"
	"net/url"
	"strings"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/httpserver"
)

func newWSCheckOrigin(corsConf *pldconf.CORSConfig) func(*http.Request) bool {
	corsConf = wsCORSConfig(corsConf)
	if !corsConf.Enabled {
		return wsSameOriginCheck
	}
	allowedOrigins := confutil.StringSlice(corsConf.AllowedOrigins, httpserver.DefaultCORS.AllowedOrigins)
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		for _, allowed := range allowedOrigins {
			if allowed == "*" || origin == allowed {
				return true
			}
		}
		return false
	}
}

// wsCORSConfig applies WS CORS defaults when no explicit policy was configured.
// Runtime config loaded from YAML does not merge PaladinConfigDefaults, so operator-
// generated configs often have cors.enabled=false unless explicitly set.
func wsCORSConfig(corsConf *pldconf.CORSConfig) *pldconf.CORSConfig {
	if len(corsConf.AllowedOrigins) == 0 && !corsConf.Enabled {
		return &pldconf.RPCServerConfigDefaults.WS.CORS
	}
	return corsConf
}

// wsSameOriginCheck mirrors gorilla/websocket's default CheckOrigin behavior.
func wsSameOriginCheck(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return equalASCIIFold(u.Host, r.Host)
}

func equalASCIIFold(s, t string) bool {
	return strings.EqualFold(s, t)
}
