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

/*
Package kata provides the main entry point for the kata shared library.

Stritcly speaking, this does not need to be under pkg because it is not intended to be imported by
other golang modules at build time.  The intention is that it is used as the entry point for the
shared library that is invoked as a c function ( most likely across JNA by portara java code )

Usage:

To start the server, use the Run function. It reads the configuration from a YAML file
and initializes the necessary components such as persistence layer, communication bus
(including the gRPC server to listen for incoming messages and message listeners),
and transaction manager.

To stop the server, use the Stop function. It gracefully stops the gRPC server and cleans up
any resources associated with it.

Configuration:

The configuration for the server is specified in a YAML file. It includes options for
persistence, gRPC server, and other components.

Example configuration file:

	persistence:
	  persistence:
		type: postgres
		sqlite:
			uri:           ":memory:"
			autoMigrate:   true
			migrationsDir: /path/to/migrationsdir
			debugQueries:  true
		postgres:
			uri:           postgres://postgres:<secret>@localhost:5432/demo?sslmode=disable
			autoMigrate:   true
			migrationsDir: /path/to/migrationsdir
			debugQueries:  true
	grpc:
	  socketAddress: /path/to/socket

Note: This package depends on other internal packages such as commsbus, confutil, persistence,
and transaction. These packages provide the necessary functionality for the server to work
properly.

For more information on how to use this package, refer to the documentation of individual
functions and types.
*/
package kata

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/log"

	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/persistence"
	"github.com/kaleido-io/paladin/kata/internal/transaction"
)

type Config struct {
	Peristence *persistence.Config `yaml:"persistence"`
	CommsBus   *commsbus.Config    `yaml:"commsBus"`
}

var commsBus commsbus.CommsBus

func Run(ctx context.Context, configFilePath string) {
	//ctx := log.WithLogField(context.Background(), "pid", strconv.Itoa(os.Getpid()))

	log.L(ctx).Infof("Kata Run: %s", configFilePath)
	config := Config{}

	err := confutil.ReadAndParseYAMLFile(ctx, configFilePath, &config)
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
