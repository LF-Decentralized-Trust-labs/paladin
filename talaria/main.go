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
	"flag"
	"sync"

	commsbus "github.com/kaleido-io/talaria/pkg/commsbus"
	talaria "github.com/kaleido-io/talaria/pkg/talaria"
)

/*
	Driver for the Talaria flow, the goal here to be able to start the Talaria system with a fake registry
	and a fake comms bus, and watch messages go throguh to Talaria, over a gRPC local socket to the plugin
	and then go from one plugin to another.

	For a diagram of what this looks like refer to the README.
*/

var (
	// TODO: Starting even this demo shouldn't require needing to provide 3 sets of port information
	commsbusport = flag.Int("commsbusport", 8080, "the port to run the comms bus on")
	registryport = flag.Int("registryport", 8081, "the port to run the registry on")
	talariaport  = flag.Int("talariaport", 8082, "the port for talaria to be listening to")
)

func main() {
	ctx := context.Background()
	flag.Parse()
	var wg sync.WaitGroup

	// Initialise the registry
	re := talaria.NewLocalAPIRegistryProvider(*registryport)

	// Initialise talaria
	tal := talaria.NewTalaria(re, *talariaport)
	tal.Initialise(ctx)

	// Start the comms bus
	cas := commsbus.NewCommsBusAPIServer(*commsbusport, tal)
	wg.Add(1)
	go func(){
		cas.StartServer(ctx)
	}()

	wg.Wait()
}