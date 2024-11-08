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

import "sync/atomic"

var running atomic.Pointer[instance]

func Run(grpcTarget, loaderUUID, configFile, runMode string) RC {
	inst := newInstance(grpcTarget, loaderUUID, configFile, runMode)
	if !running.CompareAndSwap(nil, inst) {
		panic("double started")
	}
	return inst.run()
}

func Stop() {
	inst := running.Load()
	if inst != nil {
		inst.stop()
	}
}
