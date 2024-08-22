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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/componentmgr"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"gopkg.in/yaml.v3"
)

var exitProcess = os.Exit

// The domain testbed runs a comms bus, and hosts a Domain State Interface
// It provides RPC functions to invoke the domain directly
func main() {
	tb := newTestBed()
	err := tb.setupConfig(os.Args)
	if err == nil {
		err = tb.run()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		exitProcess(1)
	}
}

type testbed struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	// TODO: Remove once testbed gets started by the module loader
	sigc       chan os.Signal
	socketFile string
	instanceID uuid.UUID

	conf       *componentmgr.Config
	rpcModule  *rpcserver.RPCModule
	components components.AllComponents

	ready chan error
	done  chan struct{}
}

func newTestBed() (tb *testbed) {
	tb = &testbed{
		sigc:       make(chan os.Signal, 1),
		instanceID: uuid.New(),
		ready:      make(chan error, 1),
		done:       make(chan struct{}),
	}
	tb.ctx, tb.cancelCtx = context.WithCancel(context.Background())
	tb.initRPC()
	return tb
}

func (tb *testbed) listenTerm() {
	signal.Notify(tb.sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-tb.sigc
	tb.stop()
}

func (tb *testbed) stop() {
	log.L(tb.ctx).Infof("Testbed shutting down")
	tb.cancelCtx()
}

func (tb *testbed) tempSocketFile() (fileName string, err error) {
	f, err := os.CreateTemp(confutil.StringOrEmpty(tb.conf.TempDir, ""), "testbed.paladin.*.sock")
	if err == nil {
		fileName = f.Name()
	}
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		err = os.Remove(fileName)
	}
	return
}

func (tb *testbed) setupConfig(args []string) error {
	configFile := "./testbed/sqlite.memory.config.yaml"
	if len(args) >= 2 {
		configFile = args[1]
	}
	configBytes, err := os.ReadFile(configFile)
	if err == nil {
		err = yaml.Unmarshal(configBytes, &tb.conf)
	}
	if tb.conf.GRPC.Address == "" {
		tb.conf.GRPC.Address, err = tb.tempSocketFile()
	}
	if err != nil {
		return err
	}
	return nil
}

func (tb *testbed) run() (err error) {
	ready := false
	defer func() {
		tb.cancelCtx()
		close(tb.done)
		if !ready {
			tb.ready <- err
		}
		close(tb.ready)
	}()

	cm := componentmgr.NewComponentManager(tb.ctx, tb.instanceID, tb.conf, tb)
	err = cm.Start()
	if err != nil {
		return fmt.Errorf("Initialization failed: %s", err)
	}

	log.L(tb.ctx).Info("Testbed started")
	tb.listenTerm()
	cm.Stop()
	log.L(tb.ctx).Info("Testbed shutdown")
	return err
}
