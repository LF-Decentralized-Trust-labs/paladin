// Copyright © 2024 Kaleido, Inc.
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

package commsbus

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"

	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

type Config struct {
	Broker BrokerConfig `yaml:"broker"`
	GRPC   GRPCConfig   `yaml:"grpc"`
}

type CommsBus interface {
	Broker() Broker
	GRPCServer() GRPCServer
}

func NewCommsBus(ctx context.Context, conf *Config) (CommsBus, error) {

	if conf == nil {
		log.L(ctx).Error("Missing comms bus config")
		return nil, i18n.NewError(ctx, msgs.MsgConfigFileMissingMandatoryValue, "commsBus")
	}

	broker, err := newBroker()
	if err != nil {
		log.L(ctx).Error("Failed to create broker", err)
		return nil, err
	}

	grpcServer, err := newGRPCServer(ctx, broker, &conf.GRPC)
	if err != nil {
		log.L(ctx).Errorf("Failed to create grpc server: %s", err)
		return nil, err
	}

	//TODO is this the best time to start the server? As a side effect of creating it?
	go func() {
		grpcServerError := grpcServer.Run(ctx)
		if grpcServerError != nil {
			log.L(ctx).Error("Failed to create run server", err)
		}
	}()
	return &commsBus{
		broker,
		grpcServer,
	}, nil
}

type commsBus struct {
	broker     Broker
	grpcServer GRPCServer
}

func (c *commsBus) Broker() Broker {
	return c.broker
}
func (c *commsBus) GRPCServer() GRPCServer {
	return c.grpcServer
}
