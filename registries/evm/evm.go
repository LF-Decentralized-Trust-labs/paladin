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
	"C"
)
import (
	"github.com/LF-Decentralized-Trust-labs/paladin/registries/evm/internal/evmregistry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
)

var ple = plugintk.NewPluginLibraryEntrypoint(func() plugintk.PluginBase {
	return plugintk.NewRegistry(func(callbacks plugintk.RegistryCallbacks) plugintk.RegistryAPI {
		return evmregistry.NewEVMRegistry(callbacks)
	})
})

//export Run
func Run(grpcTargetPtr, pluginUUIDPtr *C.char) int {
	return ple.Run(
		C.GoString(grpcTargetPtr),
		C.GoString(pluginUUIDPtr),
	)
}

//export Stop
func Stop(pluginUUIDPtr *C.char) {
	ple.Stop(C.GoString(pluginUUIDPtr))
}

func main() {}
