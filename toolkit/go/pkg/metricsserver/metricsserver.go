// Copyright © 2025 Kaleido, Inc.
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

package metricsserver

import (
	"context"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/httpserver"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/router"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsServer interface {
	Start() error
	Stop()
}

func NewMetricsServer(ctx context.Context, registry *prometheus.Registry, conf *pldconf.MetricsServerConfig) (_ *metricsServer, err error) {
	s := &metricsServer{
		bgCtx: ctx,
	}

	// Add the HTTP server
	if *conf.Enabled {
		r, err := router.NewRouter(s.bgCtx, "Metrics (HTTP)", &conf.HTTPServerConfig)
		if err != nil {
			return s, err
		}

		r.HandleFunc("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP)
		s.httpServer = r
	}
	return s, err
}

var _ MetricsServer = &metricsServer{}

type metricsServer struct {
	bgCtx      context.Context
	httpServer httpserver.Server
}

func (s *metricsServer) Start() (err error) {
	if s.httpServer != nil {
		err = s.httpServer.Start()
	}
	return err
}

func (s *metricsServer) Stop() {
	wg := new(sync.WaitGroup)
	if s.httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.httpServer.Stop()
		}()
	}
	wg.Wait()
}
