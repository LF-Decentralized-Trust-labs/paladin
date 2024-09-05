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
	"fmt"
	"path"
	"syscall"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestEngineFactory(t *testing.T) {
	tb, err := engineFactory(context.Background(), "testbed")
	assert.NoError(t, err)
	assert.NotNil(t, tb)

	_, err = engineFactory(context.Background(), "wrong")
	assert.Regexp(t, "PD011700", err)
}

func TestSignalHandlerStop(t *testing.T) {

	cmStarted := make(chan struct{})
	socketFile, loaderUUID, configFile, done := setupTestConfig(t, func(mockCM *componentmocks.ComponentManager, mockEngine *componentmocks.Engine) {
		mockCM.On("Init").Return(nil)
		mockCM.On("StartComponents").Return(nil)
		mockCM.On("StartManagers").Return(nil)
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

	inst := running.Load()
	(*inst).signals <- syscall.SIGQUIT

	err := <-completed
	assert.Nil(t, err)

}

func TestBadLoaderID(t *testing.T) {

	socketFile, _, configFile, done := setupTestConfig(t)
	defer done()

	Run(socketFile, "wrong", configFile, "unittest")

}

func TestBadConfigFile(t *testing.T) {

	socketFile, loaderUUID, _, done := setupTestConfig(t)
	defer done()

	Run(socketFile, loaderUUID, path.Join(t.TempDir(), "wrong.yaml"), "unittest")

}

func TestEngineFactoryFail(t *testing.T) {

	socketFile, loaderUUID, configFile, done := setupTestConfig(t)
	defer done()

	engineFactory = func(ctx context.Context, engineName string) (components.Engine, error) {
		return nil, fmt.Errorf("pop")
	}

	Run(socketFile, loaderUUID, configFile, "unittest")

}

func TestComponentManagerStartFail(t *testing.T) {

	socketFile, loaderUUID, configFile, done := setupTestConfig(t, func(mockCM *componentmocks.ComponentManager, mockEngine *componentmocks.Engine) {
		mockCM.On("Init").Return(nil)
		mockCM.On("StartComponents").Return(nil)
		mockCM.On("StartManagers").Return(nil)
		mockCM.On("CompleteStart").Return(fmt.Errorf("pop"))
		mockCM.On("Stop").Return()
	})
	defer done()

	Run(socketFile, loaderUUID, configFile, "unittest")

}
