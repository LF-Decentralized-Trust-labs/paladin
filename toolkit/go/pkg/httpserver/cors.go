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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/rs/cors"
)

var DefaultCORS = &pldconf.CORSConfig{
	AllowCredentials: confutil.P(false),
	AllowedMethods:   []string{http.MethodHead, http.MethodGet, http.MethodPost},
	AllowedHeaders:   []string{},
	AllowedOrigins:   []string{"*"},
	MaxAge:           confutil.P("0"),
}

func WrapCorsIfEnabled(ctx context.Context, chain http.Handler, conf *pldconf.CORSConfig) http.Handler {
	if !conf.Enabled {
		return chain
	}
	corsOptions := cors.Options{
		AllowedOrigins:   confutil.StringSlice(conf.AllowedOrigins, DefaultCORS.AllowedOrigins),
		AllowedMethods:   confutil.StringSlice(conf.AllowedMethods, DefaultCORS.AllowedMethods),
		AllowedHeaders:   confutil.StringSlice(conf.AllowedHeaders, DefaultCORS.AllowedHeaders),
		AllowCredentials: confutil.Bool(conf.AllowCredentials, *DefaultCORS.AllowCredentials),
		MaxAge:           int(confutil.DurationSeconds(conf.MaxAge, 0, *DefaultCORS.MaxAge)),
		Debug:            confutil.Bool(&conf.Debug, false),
	}
	log.L(ctx).Debugf("CORS origins=%v methods=%v headers=%v creds=%t maxAge=%ds",
		corsOptions.AllowedOrigins,
		corsOptions.AllowedMethods,
		corsOptions.AllowedHeaders,
		corsOptions.AllowCredentials,
		corsOptions.MaxAge,
	)
	c := cors.New(corsOptions)
	return c.Handler(chain)
}
