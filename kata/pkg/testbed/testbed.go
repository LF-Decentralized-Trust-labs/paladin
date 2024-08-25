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

package testbed

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/componentmgr"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"gopkg.in/yaml.v3"
)

type testbed struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	initFunctions []func(c components.AllComponents) error

	socketFile string
	instanceID uuid.UUID

	conf       *componentmgr.Config
	rpcModule  *rpcserver.RPCModule
	components components.AllComponents

	ready   chan error
	stopped chan struct{}
	done    chan struct{}
}

func NewTestBed(initFunctions ...func(c components.AllComponents) error) (tb *testbed) {
	tb = &testbed{
		instanceID:    uuid.New(),
		ready:         make(chan error, 1),
		initFunctions: initFunctions,
		stopped:       make(chan struct{}),
		done:          make(chan struct{}),
	}
	tb.ctx, tb.cancelCtx = context.WithCancel(context.Background())
	tb.initRPC()
	return tb
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
			close(tb.ready)
		}
	}()

	cm := componentmgr.NewComponentManager(tb.ctx, tb.socketFile, tb.instanceID, tb.conf, tb)
	err = cm.Init()
	if err == nil {
		err = cm.StartComponents()
	}
	for _, fn := range tb.initFunctions {
		if err == nil {
			err = fn(cm)
		}
	}
	if err == nil {
		err = cm.CompleteStart()
	}
	if err != nil {
		return fmt.Errorf("Initialization failed: %s", err)
	}
	ready = true
	close(tb.ready)

	log.L(tb.ctx).Info("Testbed started")
	<-tb.stopped
	cm.Stop()
	log.L(tb.ctx).Info("Testbed shutdown")
	return err
}

func (tb *testbed) EngineName() string {
	return "testbed"
}

func (tb *testbed) Start() error {
	// we don't have anything additional that runs beyond the components
	return nil
}

func (tb *testbed) Stop() {
	close(tb.stopped)
}

func (tb *testbed) Init(c components.AllComponents) (*components.ManagerInitResult, error) {
	tb.components = c
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{tb.rpcModule},
	}, nil
}
