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
	"path"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/componentmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentmgrmocks"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func setupTestConfig(t *testing.T, mockers ...func(mockCM *componentmgrmocks.ComponentManager)) (socketFile, loaderUUID, configFile string, done func()) {
	id := uuid.New()
	origCMFactory := componentManagerFactory
	mockCM := componentmgrmocks.NewComponentManager(t)
	componentManagerFactory = func(bgCtx context.Context, grpcTarget string, instanceUUID uuid.UUID, conf *pldconf.PaladinConfig, additionalManagers ...components.AdditionalManager) componentmgr.ComponentManager {
		assert.Equal(t, id, instanceUUID)
		assert.Equal(t, "http://localhost:8545", conf.Blockchain.HTTP.URL)
		return mockCM
	}
	for _, mocker := range mockers {
		mocker(mockCM)
	}
	configFile = path.Join(t.TempDir(), "paladin.conf.yaml")
	err := os.WriteFile(configFile, []byte(`{
	  "blockchain": { "http": { "url": "http://localhost:8545" } }
	}`), 0664)
	require.NoError(t, err)
	return path.Join(t.TempDir(), "socket.file"), id.String(), configFile, func() {
		componentManagerFactory = origCMFactory
	}
}

func TestEntrypointOK(t *testing.T) {

	cmStarted := make(chan struct{})
	socketFile, loaderUUID, configFile, done := setupTestConfig(t, func(mockCM *componentmgrmocks.ComponentManager) {
		mockCM.On("Init").Return(nil)
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
		Run(socketFile, loaderUUID, configFile, "testbed")
	}()

	<-cmStarted

	// Double start should panic
	assert.Panics(t, func() {
		Run(socketFile, loaderUUID, configFile, "testbed")
	})

	Stop()
	err := <-completed
	assert.Nil(t, err)

}

func TestEntrypointBadMode(t *testing.T) {

	socketFile, loaderUUID, configFile, done := setupTestConfig(t, func(mockCM *componentmgrmocks.ComponentManager) {})
	defer done()

	rc := Run(socketFile, loaderUUID, configFile, "wrong")
	require.Equal(t, RC_FAIL, rc)

}
