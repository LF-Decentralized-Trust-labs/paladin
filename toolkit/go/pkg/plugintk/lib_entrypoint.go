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
package plugintk

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
)

type PluginLibraryEntrypoint struct {
	l       sync.Mutex
	factory func() PluginBase
	plugins map[string]PluginBase
}

func NewPluginLibraryEntrypoint(factory func() PluginBase) *PluginLibraryEntrypoint {
	// Debug logging until anything different initialized by plugin
	log.SetLevel("debug")
	return &PluginLibraryEntrypoint{
		factory: factory,
		plugins: make(map[string]PluginBase),
	}
}

func (ple *PluginLibraryEntrypoint) Run(grpcTarget, pluginUUID string) (rc int) {
	defer func() {
		panicked := recover()
		if panicked != nil {
			// print the stack
			fmt.Fprintf(os.Stderr, "%s\n", debug.Stack())
			// set the rc
			rc = 1
		}
	}()

	log.L(context.Background()).Infof("Starting plugin ID %s", pluginUUID)
	p := ple.factory()
	ple.addPlugin(pluginUUID, p)

	p.Run(grpcTarget, pluginUUID)
	return 0
}

func (ple *PluginLibraryEntrypoint) Stop(pluginUUID string) {
	p := ple.removePlugin(pluginUUID)
	if p != nil {
		p.Stop()
	}
}

func (ple *PluginLibraryEntrypoint) addPlugin(pluginUUID string, p PluginBase) {
	ple.l.Lock()
	defer ple.l.Unlock()

	if _, existing := ple.plugins[pluginUUID]; existing {
		panic("duplicate start " + pluginUUID)
	}
	ple.plugins[pluginUUID] = p
}

func (ple *PluginLibraryEntrypoint) removePlugin(pluginUUID string) PluginBase {
	ple.l.Lock()
	defer ple.l.Unlock()

	p := ple.plugins[pluginUUID]
	delete(ple.plugins, pluginUUID)
	return p
}
