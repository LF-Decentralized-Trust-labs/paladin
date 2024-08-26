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

package kata

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/componentmgr"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupTestConfig(t *testing.T, mockers ...func(mockCM *componentmocks.ComponentManager, mockEngine *componentmocks.Engine)) (socketFile, loaderUUID, configFile string, done func()) {
	id := uuid.New()
	origCMFactory := componentManagerFactory
	mockCM := componentmocks.NewComponentManager(t)
	componentManagerFactory = func(bgCtx context.Context, socketAddress string, instanceUUID uuid.UUID, conf *componentmgr.Config, engine components.Engine) componentmgr.ComponentManager {
		assert.Equal(t, id, instanceUUID)
		assert.Equal(t, "http://localhost:8545", conf.Blockchain.HTTP.URL)
		return mockCM
	}
	mockEngine := componentmocks.NewEngine(t)
	origEngineFactory := engineFactory
	engineFactory = func(ctx context.Context, engineName string) (components.Engine, error) {
		assert.Equal(t, "unittest", engineName)
		return mockEngine, nil
	}
	for _, mocker := range mockers {
		mocker(mockCM, mockEngine)
	}
	configFile = path.Join(t.TempDir(), "paladin.conf.yaml")
	err := os.WriteFile(configFile, []byte(`{
	  "blockchain": { "http": { "url": "http://localhost:8545" } }
	}`), 0664)
	assert.NoError(t, err)
	return path.Join(t.TempDir(), "socket.file"), id.String(), configFile, func() {
		engineFactory = origEngineFactory
		componentManagerFactory = origCMFactory
	}
}

func TestEntrypointOK(t *testing.T) {

	cmStarted := make(chan struct{})
	socketFile, loaderUUID, configFile, done := setupTestConfig(t, func(mockCM *componentmocks.ComponentManager, mockEngine *componentmocks.Engine) {
		mockCM.On("Init").Return(nil)
		mockCM.On("StartComponents").Return(nil)
		mockCM.On("CompleteStart").Return(nil).Run(func(args mock.Arguments) {
			close(cmStarted)
		})
		mockCM.On("Stop").Return()
	})
	defer done()

	completed := make(chan any)
	go func() {
		defer func() {
			completed <- recover()
		}()
		Run(socketFile, loaderUUID, configFile, "unittest")
	}()

	<-cmStarted

	// Double start should panic
	assert.Panics(t, func() {
		Run(socketFile, loaderUUID, configFile, "unittest")
	})

	Stop()
	err := <-completed
	assert.Nil(t, err)

}
