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

package server

import (
	"context"
	"encoding/json"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

type HttpServer struct {
	srv      *http.Server
	mux      *http.ServeMux
	shutdown chan struct{}
}

func NewHttpServer() *HttpServer {
	hs := &HttpServer{
		mux: http.NewServeMux(),
	}

	hs.srv = &http.Server{
		Addr:    ":5050",
		Handler: hs.mux,
	}

	hs.mux.HandleFunc("/status", statusHandler)
	hs.mux.Handle("/metrics", promhttp.Handler())

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	hs.shutdown = make(chan struct{})

	return hs
}

func (hs *HttpServer) Run() {
	log.Info("Starting server HTTP server")

	go hs.gracefulShutdown()

	if err := hs.srv.ListenAndServe(); err != http.ErrServerClosed {
		// Error starting or closing listener:
		log.Errorf("HTTP server ListenAndServe: %v\n", err)
	}

	<-hs.shutdown
}

func statusHandler(writer http.ResponseWriter, request *http.Request) {
	status := struct {
		Up bool `json:"up"`
	}{Up: true}

	writer.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(writer)
	err := encoder.Encode(&status)
	if err != nil {
		log.Error(err)
	}

	return
}

func (hs *HttpServer) gracefulShutdown() {
	log.Info("Graceful shutdown listener started")
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	signal.Notify(signalCh, os.Kill)
	signal.Notify(signalCh, syscall.SIGTERM)
	signal.Notify(signalCh, syscall.SIGQUIT)
	signal.Notify(signalCh, syscall.SIGKILL)

	<-signalCh

	log.Warn("Received shutdown signal")
	hs.Shutdown()
}

func (hs *HttpServer) Shutdown() {
	log.Warn("Server shutting down in 5s")
	time.Sleep(5 * time.Second)

	// We received an interrupt signal, shut down.
	if err := hs.srv.Shutdown(context.Background()); err != nil {
		// Error from closing listeners, or context timeout:
		log.Errorf("HTTP server Shutdown: %v\n", err)
	}
	close(hs.shutdown)
}
