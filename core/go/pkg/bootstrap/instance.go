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

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/componentmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/config"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
)

var componentManagerFactory = componentmgr.NewComponentManager

type instance struct {
	grpcTarget string
	loaderUUID string
	configFile string
	runMode    string

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

func newInstance(grpcTarget, loaderUUID, configFile, runMode string) *instance {
	i := &instance{
		grpcTarget: grpcTarget,
		loaderUUID: loaderUUID,
		configFile: configFile,
		runMode:    runMode,
		signals:    make(chan os.Signal),
		done:       make(chan struct{}),
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

	var conf pldconf.PaladinConfig
	if err = config.ReadAndParseYAMLFile(i.ctx, i.configFile, &conf); err != nil {
		log.L(i.ctx).Error(err.Error())
		return RC_FAIL
	}

	var additionalManagers []components.AdditionalManager
	switch i.runMode {
	case "testbed":
		additionalManagers = append(additionalManagers, testbed.NewTestBed())
	case "engine":
	default:
		log.L(i.ctx).Error(i18n.NewError(i.ctx, msgs.MsgEntrypointUnknownRunMode, i.runMode))
		return RC_FAIL
	}

	cm := componentManagerFactory(i.ctx, i.grpcTarget, id, &conf, additionalManagers...)
	// From this point need to ensure we stop the component manager
	defer cm.Stop()

	// Start it up
	err = cm.Init()
	if err == nil {
		// Managers start first - so they are ready to process
		err = cm.StartManagers()
	}
	if err == nil {
		// Then finally the active processing is started:
		// - The block indexer starts indexing
		// - The JSON/RPC front door is opened
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
