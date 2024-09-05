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
	"strconv"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"

	"github.com/kaleido-io/paladin/core/internal/commsbus"
	"github.com/kaleido-io/paladin/core/internal/componentmgr"
	"github.com/kaleido-io/paladin/core/internal/transaction"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
)

type CommsBusConfig struct {
	Peristence *persistence.Config `yaml:"persistence"`
	CommsBus   *commsbus.Config    `yaml:"commsBus"`
}

var commsBus commsbus.CommsBus

func TestCommsBusRun(ctx context.Context, configFilePath string) {
	ctx = log.WithLogField(ctx, "pid", strconv.Itoa(os.Getpid()))

	log.L(ctx).Infof("Kata Run: %s", configFilePath)
	config := CommsBusConfig{}

	err := componentmgr.ReadAndParseYAMLFile(ctx, configFilePath, &config)
	if err != nil {
		log.L(ctx).Errorf("failed to read and parse YAML file: %v", err)
		return
	}
	//Validate config

	//Initialise the persistence layer
	persistence, err := persistence.NewPersistence(ctx, config.Peristence)
	if err != nil {
		log.L(ctx).Errorf("failed to initialise persistence: %v", err)
		return
	}

	//Initialise the commsbus
	commsBus, err = commsbus.NewCommsBus(ctx, config.CommsBus)
	if err != nil {
		log.L(ctx).Errorf("failed to initialise commsBus: %v", err)
		return
	}

	//Initialise the transaction manager
	err = transaction.Init(ctx, persistence, commsBus)
	if err != nil {
		log.L(ctx).Errorf("failed to initialise transaction manager: %v", err)
		return
	}

}

func CommsBus() commsbus.CommsBus {
	return commsBus
}

func CommsBusStop(ctx context.Context, socketAddress string) {
	log.L(ctx).Infof("Stop: %s", socketAddress)
	if commsBus != nil {
		err := commsBus.GRPCServer().Stop(ctx)
		if err != nil {
			log.L(ctx).Errorf("Failed to stop GRPC server: %s", err)
		}
	}
}
