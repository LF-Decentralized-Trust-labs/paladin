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

package components

import (
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
)

// PreInitComponents are ones that are initialized before managers.
// PreInit components do not depend on any other components, they hold their
// own interface in their package.
type PreInitComponents interface {
	KeyManager() ethclient.KeyManager // TODO: move to separate component
	EthClientFactory() ethclient.EthClientFactory
	Persistence() persistence.Persistence
	StateStore() statestore.StateStore
	BlockIndexer() blockindexer.BlockIndexer
	RPCServer() rpcserver.RPCServer
}

// Managers are initialized after base components with access to them, and provide
// output that is used to finalize startup of the LateBoundComponents.
//
// Their start informs the configuration of the late bound components, so they
// must start before them. But they still have access to those.
//
// So that they can call each other, their external mockable interfaces provided
// to the are all defined in this package.
type Managers interface {
	DomainManager() DomainManager
	TransportManager() TransportManager
	RegistryManager() RegistryManager
	PluginManager() PluginManager
}

// All managers conform to a standard lifecycle
type ManagerLifecycle interface {
	// Init only depends on the configuration and components - no other managers
	PreInit(PreInitComponents) (*ManagerInitResult, error)
	// Post-init allows the manager to cross-bind to other components, or the Engine
	PostInit(AllComponents) error
	Start() error
	Stop()
}

// Managers get limited influence over the event streams created for them,
// to ensure consistent naming and lifecycle management semantics
type ManagerEventStream struct {
	ABI     abi.ABI
	Handler blockindexer.InternalStreamCallback
}

// Managers can instruct the init of some of the PostInitComponents in a generic way
type ManagerInitResult struct {
	EventStreams []*ManagerEventStream
	RPCModules   []*rpcserver.RPCModule
}

type PreInitComponentsAndManagers interface {
	PreInitComponents
	Managers
}

// Two examples of an engine exist:
// - The runtime engine of Paladin, which does real work
// - The testbed, which provides a JSON/RPC testing interface for domains in isolation from the engine
// The other component do not know or care which engine is orchestrating them.
type Engine interface {
	EngineName() string
	Init(PreInitComponentsAndManagers) (*ManagerInitResult, error)
	Start() error
	Stop()
	ReceiveTransportMessage(*TransportMessage)
}

type AllComponents interface {
	PreInitComponents
	Managers
	Engine() Engine
}
