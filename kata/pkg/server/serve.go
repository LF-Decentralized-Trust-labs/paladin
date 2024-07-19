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
package server

import (
	"context"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/hyperledger/firefly-common/pkg/log"
	"google.golang.org/grpc"

	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/persistence"
	"github.com/kaleido-io/paladin/kata/internal/transaction"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

type grpcServer struct {
	listener net.Listener
	server   *grpc.Server
	done     chan error
}

var serverLock sync.Mutex

var servers = map[string]*grpcServer{}

func newRPCServer(socketAddress string, broker commsbus.Broker) (*grpcServer, error) {
	ctx := log.WithLogField(context.Background(), "pid", strconv.Itoa(os.Getpid()))
	log.L(ctx).Infof("server starting at unix socket %s", socketAddress)
	l, err := net.Listen("unix", socketAddress)
	if err != nil {
		log.L(ctx).Error("failed to listen: ", err)
		return nil, err
	}
	s := grpc.NewServer()

	proto.RegisterKataMessageServiceServer(s, NewKataMessageService(ctx, broker))

	log.L(ctx).Infof("server listening at %v", l.Addr())
	return &grpcServer{
		listener: l,
		server:   s,
		done:     make(chan error),
	}, nil
}

type GRPCConfig struct {
	SocketAddress *string `yaml:"socketAddress"`
}

type Config struct {
	Peristence *persistence.Config `yaml:"persistence"`
	GRPC       *GRPCConfig         `yaml:"grpc"`
}

func Run(ctx context.Context, configFilePath string) {
	log.L(ctx).Infof("Kata Run: %s", configFilePath)
	config := Config{}

	err := confutil.ReadAndParseYAMLFile(ctx, configFilePath, &config)
	if err != nil {
		log.L(ctx).Errorf("failed to read and parse YAML file: %v", err)
		return
	}
	//Validate config
	if config.GRPC == nil || config.GRPC.SocketAddress == nil {
		log.L(ctx).Errorf("missing grpc config in config file %s", configFilePath)
		return
	}

	//Initialise the persistence layer
	persistence, err := persistence.NewPersistence(ctx, config.Peristence)
	if err != nil {
		log.L(ctx).Errorf("failed to initialise persistence: %v", err)
		return
	}

	//Initialise the commsbus
	broker, err := commsbus.NewBroker(ctx, &commsbus.BrokerConfig{})
	if err != nil {
		log.L(ctx).Errorf("failed to initialise broker: %v", err)
		return
	}

	//Initialise the transaction manager
	err = transaction.Init(ctx, persistence, broker)
	if err != nil {
		log.L(ctx).Errorf("failed to initialise transaction manager: %v", err)
		return
	}

	runGRPCServer(ctx, *config.GRPC.SocketAddress, broker)
}
func runGRPCServer(ctx context.Context, socketAddress string, broker commsbus.Broker) {
	log.L(ctx).Infof("Run: %s", socketAddress)

	serverLock.Lock()
	_, exists := servers[socketAddress]
	serverLock.Unlock()

	if exists {
		log.L(ctx).Errorf("Server %s already running", socketAddress)
		return
	}
	s, err := newRPCServer(socketAddress, broker)
	if err != nil {
		return
	}

	serverLock.Lock()
	servers[socketAddress] = s
	serverLock.Unlock()

	log.L(ctx).Infof("Server %s started", socketAddress)
	s.done <- s.server.Serve(s.listener)
	log.L(ctx).Infof("Server %s ended", socketAddress)
}

func Stop(ctx context.Context, socketAddress string) {
	log.L(ctx).Infof("Stop: %s", socketAddress)

	serverLock.Lock()
	s := servers[socketAddress]
	serverLock.Unlock()

	if s != nil {
		log.L(ctx).Infof("Stopping server on address %s", socketAddress)
		s.server.GracefulStop()
		serverErr := <-s.done
		log.L(ctx).Infof("Server %s stopped (err=%v)", socketAddress, serverErr)
	} else {
		log.L(ctx).Infof("No server for address %s", socketAddress)
	}

	serverLock.Lock()
	delete(servers, socketAddress)
	serverLock.Unlock()
}
