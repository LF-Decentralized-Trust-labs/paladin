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

package kata

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/componentmgr"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

var engineFactory = func(ctx context.Context, engineName string) (components.Engine, error) {
	switch engineName {
	case "testbed":
		return testbed.NewTestBed(), nil
	default:
		return nil, i18n.NewError(ctx, msgs.MsgEntrypointUnknownEngine, engineName)
	}
}

var componentManagerFactory = componentmgr.NewComponentManager

type instance struct {
	socketAddress string
	engineName    string
	loaderUUID    string
	configFile    string

	ctx       context.Context
	cancelCtx context.CancelFunc
	signals   chan os.Signal
	stopped   atomic.Bool
	done      chan struct{}
}

type RC int

const (
	RC_OK   RC = 0
	RC_FAIL RC = 1
)

func newInstance(socketAddress, loaderUUID, configFile, engineName string) *instance {
	i := &instance{
		socketAddress: socketAddress,
		loaderUUID:    loaderUUID,
		configFile:    configFile,
		engineName:    engineName,
		signals:       make(chan os.Signal),
		done:          make(chan struct{}),
	}
	i.ctx, i.cancelCtx = context.WithCancel(log.WithLogField(context.Background(), "pid", strconv.Itoa(os.Getpid())))
	return i
}

func (i *instance) signalHandler() {
	signal.Notify(i.signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	sig := <-i.signals
	if sig != nil {
		log.L(i.ctx).Infof("Stopping due to signal %s", sig)
		i.stop()
	}
}

func (i *instance) run() RC {
	defer func() {
		close(i.done)
		running.Store(nil)
	}()
	go i.signalHandler()

	id, err := uuid.Parse(i.loaderUUID)
	if err != nil {
		log.L(i.ctx).Errorf("Invalid loader UUID %q: %s", i.loaderUUID, err)
		return RC_FAIL
	}

	var conf componentmgr.Config
	if err = componentmgr.ReadAndParseYAMLFile(i.ctx, i.configFile, &conf); err != nil {
		log.L(i.ctx).Error(err.Error())
		return RC_FAIL
	}

	engine, err := engineFactory(i.ctx, i.engineName)
	if err != nil {
		log.L(i.ctx).Error(err.Error())
		return RC_FAIL
	}

	cm := componentManagerFactory(i.ctx, i.socketAddress, id, &conf, engine)
	// From this point need to ensure we stop the component manager
	defer cm.Stop()

	// Start it up
	err = cm.Init()
	if err == nil {
		err = cm.StartComponents()
	}
	if err == nil {
		err = cm.CompleteStart()
	}
	if err != nil {
		log.L(i.ctx).Error(err.Error())
		return RC_FAIL
	}

	// We're started... we just wait for the request to stop
	<-i.ctx.Done()

	return RC_OK
}

func (i *instance) stop() {
	if i.stopped.CompareAndSwap(false, true) {
		i.cancelCtx()
		close(i.signals)
		<-i.done
	}
}
