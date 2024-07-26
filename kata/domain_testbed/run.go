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
	"github.com/kaleido-io/paladin/kata/internal/persistence"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"gopkg.in/yaml.v3"
)

// The domain testbed runs a comms bus, and hosts a Domain State Interface
// It provides RPC functions to invoke the domain directly
func main() {
	err := run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func randSocket(baseDir string) (string, error) {
	f, err := os.CreateTemp(baseDir, "testbed.paladin.*.sock")
	if err == nil {
		err = os.Remove(f.Name())
	}
	return f.Name(), err
}

func cleanup(ctx context.Context, sf string) {
	if _, err := os.Stat(sf); err == nil {
		if err = os.Remove(sf); err != nil {
			log.L(ctx).Warnf("Failed to clean up %s: %s", sf, err)
		}
	}
}

func listenTerm(ctx context.Context, cancelCtx context.CancelFunc, bus commsbus.CommsBus) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sigc
	log.L(ctx).Infof("Testbed shutting down")
	cancelCtx()
	if err := bus.GRPCServer().Stop(ctx); err != nil {
		log.L(ctx).Warnf("Failed to shut down commsbus: %s", err)
	}
}

func setupConfig(ctx context.Context, args []string) (*TestBedConfig, error) {
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
	if conf.CommsBus.GRPC.SocketAddress == nil || len(*conf.CommsBus.GRPC.SocketAddress) == 0 {
		rs, err := randSocket("")
		if err != nil {
			return nil, err
		}
		conf.CommsBus.GRPC.SocketAddress = &rs
	}
	cleanup(ctx, *conf.CommsBus.GRPC.SocketAddress)
	return &conf, nil
}

func run(args []string) error {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	conf, err := setupConfig(ctx, args)
	if err != nil {
		return err
	}
	defer cleanup(ctx, *conf.CommsBus.GRPC.SocketAddress)

	p, err := persistence.NewPersistence(ctx, &conf.DB)
	if err != nil {
		return err
	}
	defer p.Close()

	_ = statestore.NewStateStore(ctx, &conf.StateStore, p)

	bus, err := commsbus.NewCommsBus(ctx, &conf.CommsBus)
	if err != nil {
		return err
	}

	fromDomain, err := bus.Broker().Listen(ctx, "to-domain")
	if err != nil {
		return err
	}

	go listenTerm(ctx, cancelCtx, bus)
	eventHandler(ctx, fromDomain)
	log.L(ctx).Info("Testbed shutdown")
	return err
}
