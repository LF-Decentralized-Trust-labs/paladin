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
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/blockindexer"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/persistence"
	"github.com/kaleido-io/paladin/kata/internal/rpcclient"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
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
	ctx            context.Context
	cancelCtx      context.CancelFunc
	conf           *TestBedConfig
	sigc           chan os.Signal
	rpcServer      rpcserver.Server
	stateStore     statestore.StateStore
	blockindexer   blockindexer.BlockIndexer
	chainID        int64
	blockchainRPC  rpcbackend.WebSocketRPCClient
	signer         signer.SigningModule
	bus            commsbus.CommsBus
	fromDomain     commsbus.MessageHandler
	socketFile     string
	destToDomain   string
	destFromDomain string
	inflight       map[string]*inflightRequest
	inflightLock   sync.Mutex
	domainRegistry map[string]*testbedDomain
	domainLock     sync.Mutex
	ready          chan error
	done           chan struct{}
}

func newTestBed() (tb *testbed) {
	tb = &testbed{
		sigc:           make(chan os.Signal, 1),
		inflight:       make(map[string]*inflightRequest),
		domainRegistry: make(map[string]*testbedDomain),
		ready:          make(chan error, 1),
		done:           make(chan struct{}),
	}
	tb.ctx, tb.cancelCtx = context.WithCancel(context.Background())
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
	_ = tb.bus.GRPCServer().Stop(tb.ctx)
}

func (tb *testbed) tempSocketFile() (string, error) {
	f, err := os.CreateTemp(confutil.StringOrEmpty(tb.conf.TempDir, ""), "testbed.paladin.*.sock")
	if err != nil {
		return "", err
	}
	return f.Name(), err
}

func (tb *testbed) setupConfig(args []string) error {
	configFile := "./domain_testbed/sqlite.memory.config.yaml"
	if len(args) >= 2 {
		configFile = args[1]
	}
	configBytes, err := os.ReadFile(configFile)
	if err == nil {
		err = yaml.Unmarshal(configBytes, &tb.conf)
	}
	if err == nil {
		tb.socketFile = confutil.StringOrEmpty(tb.conf.CommsBus.GRPC.SocketAddress, "")
		if tb.socketFile == "" {
			tb.socketFile, err = tb.tempSocketFile()
			tb.conf.CommsBus.GRPC.SocketAddress = &tb.socketFile
		}
	}
	if err == nil {
		// Possible a file might be lying around that needs deleting
		err = tb.cleanupOldSocket()
	}
	if err != nil {
		return err
	}
	tb.destFromDomain = confutil.StringNotEmpty(tb.conf.Destinations.FromDomain, "from-domain")
	tb.destToDomain = confutil.StringNotEmpty(tb.conf.Destinations.ToDomain, "to-domain")
	return nil
}

func (tb *testbed) cleanupOldSocket() error {
	if _, err := os.Stat(tb.socketFile); err == nil {
		if err = os.Remove(tb.socketFile); err != nil {
			return err
		}
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

	p, err := persistence.NewPersistence(tb.ctx, &tb.conf.DB)
	if err != nil {
		return fmt.Errorf("Persistence init failed: %s", err)
	}
	defer p.Close()

	tb.bus, err = commsbus.NewCommsBus(tb.ctx, &tb.conf.CommsBus)
	if err == nil {
		tb.fromDomain, err = tb.bus.Broker().Listen(tb.ctx, "from-domain")
	}
	if err != nil {
		return fmt.Errorf("Comms bus init failed: %s", err)
	}

	tb.stateStore = statestore.NewStateStore(tb.ctx, &tb.conf.StateStore, p)
	tb.rpcServer, err = rpcserver.NewServer(tb.ctx, &tb.conf.RPCServer)
	if err == nil {
		err = tb.initRPC()
	}
	if err != nil {
		return fmt.Errorf("RPC init failed: %s", err)
	}

	rpcConf, err := rpcclient.ParseWSConfig(tb.ctx, &tb.conf.Blockchain.WS)
	if err == nil {
		tb.blockchainRPC = rpcbackend.NewWSRPCClient(rpcConf)
		err = tb.blockchainRPC.Connect(tb.ctx)
	}
	if err == nil {
		tb.blockindexer, err = blockindexer.NewBlockIndexer(tb.ctx, &tb.conf.BlockIndexer, &tb.conf.Blockchain.WS, p)
	}
	if err == nil {
		err = tb.setupChainID()
	}
	if err != nil {
		return fmt.Errorf("Blockchain init failed: %s", err)
	}
	tb.blockindexer.Start()

	tb.signer, err = signer.NewSigningModule(tb.ctx, &tb.conf.Signer)
	if err != nil {
		return fmt.Errorf("Signer init failed: %s", err)
	}

	go tb.listenTerm()

	tb.blockindexer.Stop()

	tb.ready <- nil
	ready = true
	tb.eventHandler()
	log.L(tb.ctx).Info("Testbed shutdown")
	return err
}

func logJSON(v interface{}) string {
	ret := ""
	b, _ := json.Marshal(v)
	if len(b) > 0 {
		ret = (string)(b)
	}
	return ret
}
