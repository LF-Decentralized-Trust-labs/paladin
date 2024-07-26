/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/persistence"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"gopkg.in/yaml.v3"
)

// The domain testbed runs a comms bus, and hosts a Domain State Interface
// It provides RPC functions to invoke the domain directly
func main() {
	tb, err := newTestBed(os.Args)
	if err == nil {
		err = tb.run()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

type testbed struct {
	ctx        context.Context
	cancelCtx  context.CancelFunc
	conf       *TestBedConfig
	sigc       chan os.Signal
	rpcServer  rpcserver.Server
	stateStore statestore.StateStore
	bus        commsbus.CommsBus
	fromDomain commsbus.MessageHandler
	socketFile string
	ready      chan struct{}
	done       chan struct{}
}

func newTestBed(args []string) (tb *testbed, err error) {
	tb = &testbed{
		sigc:  make(chan os.Signal, 1),
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}
	tb.ctx, tb.cancelCtx = context.WithCancel(context.Background())
	if tb.conf, err = tb.setupConfig(args); err != nil {
		return nil, err
	}
	return tb, nil
}

func (tb *testbed) randSocket(baseDir string) (string, error) {
	f, err := os.CreateTemp(baseDir, "testbed.paladin.*.sock")
	if err == nil {
		err = os.Remove(f.Name())
	}
	return f.Name(), err
}

func (tb *testbed) cleanupSocket() {
	if _, err := os.Stat(tb.socketFile); err == nil {
		if err = os.Remove(tb.socketFile); err != nil {
			log.L(tb.ctx).Warnf("Failed to clean up %s: %s", tb.socketFile, err)
		}
	}
}

func (tb *testbed) listenTerm() {
	signal.Notify(tb.sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-tb.sigc
	tb.stop()
}

func (tb *testbed) stop() {
	log.L(tb.ctx).Infof("Testbed shutting down")
	tb.cancelCtx()
	if err := tb.bus.GRPCServer().Stop(tb.ctx); err != nil {
		log.L(tb.ctx).Warnf("Failed to shut down commsbus: %s", err)
	}
}

func (tb *testbed) setupConfig(args []string) (*TestBedConfig, error) {
	var configFile string
	if len(args) >= 2 {
		configFile = args[1]
	} else {
		configFile = "./domain_testbed/sqlite.memory.config.yaml"
	}
	configBytes, err := os.ReadFile(configFile)
	var conf TestBedConfig
	if err == nil {
		err = yaml.Unmarshal(configBytes, &conf)
	}
	if err != nil {
		return nil, err
	}
	tb.socketFile = confutil.StringOrEmpty(conf.CommsBus.GRPC.SocketAddress, "")
	if tb.socketFile == "" {
		if tb.socketFile, err = tb.randSocket(""); err != nil {
			return nil, err
		}
		conf.CommsBus.GRPC.SocketAddress = &tb.socketFile
	}
	tb.cleanupSocket()
	return &conf, nil
}

func (tb *testbed) run() error {
	ready := false
	defer func() {
		tb.cleanupSocket()
		close(tb.done)
		if !ready {
			close(tb.ready)
		}
	}()

	p, err := persistence.NewPersistence(tb.ctx, &tb.conf.DB)
	if err != nil {
		return err
	}
	defer p.Close()

	if tb.bus, err = commsbus.NewCommsBus(tb.ctx, &tb.conf.CommsBus); err != nil {
		return err
	}

	if tb.fromDomain, err = tb.bus.Broker().Listen(tb.ctx, "from-domain"); err != nil {
		return err
	}

	tb.stateStore = statestore.NewStateStore(tb.ctx, &tb.conf.StateStore, p)
	tb.rpcServer, err = rpcserver.NewServer(tb.ctx, &tb.conf.RPC)
	if err != nil {
		return err
	}
	tb.initRPC()

	go tb.listenTerm()

	ready = true
	close(tb.ready)
	tb.eventHandler()
	log.L(tb.ctx).Info("Testbed shutdown")
	return err
}
