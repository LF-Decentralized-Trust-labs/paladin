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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestEntrypoint(t *testing.T) {

	ple := NewPluginLibraryEntrypoint(func() PluginBase {
		return NewDomain(func(callbacks DomainCallbacks) DomainAPI {
			return &DomainAPIBase{}
		})
	})

	socketFile := tempSocketFile(t)
	pluginID := uuid.NewString()

	done := make(chan struct{})
	go func() {
		defer close(done)
		rc := ple.Run(socketFile, pluginID)
		assert.Equal(t, 0, rc)
	}()

	for {
		ple.l.Lock()
		pLen := len(ple.plugins)
		ple.l.Unlock()
		if pLen > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Check dup start returns failure
	assert.Equal(t, 1, ple.Run(socketFile, pluginID))

	ple.Stop(pluginID)

	ple.l.Lock()
	pLen := len(ple.plugins)
	ple.l.Unlock()
	assert.Zero(t, pLen)

}

func TestEntrypointDupStart(t *testing.T) {

	ple := NewPluginLibraryEntrypoint(func() PluginBase {
		panic("pop")
	})

	pluginID := uuid.NewString()
	rc := ple.Run(tempSocketFile(t), pluginID)
	assert.Equal(t, 1, rc)
}
