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
package plugins

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	prototk "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func tempUDS(t *testing.T) string {
	// Not safe to use t.TempDir() as it generates too long paths including the test name
	f, err := os.CreateTemp("", "ut_*.sock")
	require.NoError(t, err)
	_ = f.Close()
	allocatedUDSName := f.Name()
	err = os.Remove(allocatedUDSName)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := os.Remove(allocatedUDSName)
		assert.True(t, err == nil || os.IsNotExist(err))
	})
	return "unix:" + allocatedUDSName
}

type testManagers struct {
	testDomainManager    *testDomainManager
	testTransportManager *testTransportManager
	testRegistryManager  *testRegistryManager
}

func (tm *testManagers) componentMocks(t *testing.T) *componentmocks.AllComponents {
	mc := componentmocks.NewAllComponents(t)
	if tm.testDomainManager == nil {
		tm.testDomainManager = &testDomainManager{}
	}
	mc.On("DomainManager").Return(tm.testDomainManager.mock(t)).Maybe()
	if tm.testTransportManager == nil {
		tm.testTransportManager = &testTransportManager{}
	}
	mc.On("TransportManager").Return(tm.testTransportManager.mock(t)).Maybe()
	if tm.testRegistryManager == nil {
		tm.testRegistryManager = &testRegistryManager{}
	}
	mc.On("RegistryManager").Return(tm.testRegistryManager.mock(t)).Maybe()
	return mc
}

func (ts *testManagers) allPlugins() map[string]plugintk.Plugin {
	testPlugins := make(map[string]plugintk.Plugin)
	for name, td := range ts.testDomainManager.domains {
		testPlugins[name] = td
	}
	for name, td := range ts.testTransportManager.transports {
		testPlugins[name] = td
	}
	for name, td := range ts.testRegistryManager.registries {
		testPlugins[name] = td
	}
	return testPlugins
}

func newTestPluginManager(t *testing.T, setup *testManagers) *pluginManager {
	udsString := tempUDS(t)
	loaderId := uuid.New()
	pc := NewPluginManager(context.Background(), udsString, loaderId, &PluginManagerConfig{
		GRPC: GRPCConfig{
			ShutdownTimeout: confutil.P("1ms"),
		},
	})
	mc := setup.componentMocks(t)
	ir, err := pc.PreInit(mc)
	assert.NotNil(t, ir)
	require.NoError(t, err)
	err = pc.PostInit(mc)
	require.NoError(t, err)
	err = pc.Start()
	require.NoError(t, err)

	return pc.(*pluginManager)
}

func TestControllerStartGracefulShutdownNoConns(t *testing.T) {
	pc := newTestPluginManager(t, &testManagers{})
	pc.Stop()
}

func TestInitPluginManagerBadPlugin(t *testing.T) {
	tdm := &testDomainManager{domains: map[string]plugintk.Plugin{
		"!badname": &mockPlugin[prototk.DomainMessage]{t: t},
	}}
	pc := NewPluginManager(context.Background(), tempUDS(t), uuid.New(), &PluginManagerConfig{})
	err := pc.PostInit((&testManagers{testDomainManager: tdm}).componentMocks(t))
	assert.Regexp(t, "PD020005", err)
}

func TestInitPluginManagerBadSocket(t *testing.T) {
	pc := NewPluginManager(context.Background(),
		t.TempDir(), /* can't use a dir as a socket */
		uuid.New(), &PluginManagerConfig{},
	)
	err := pc.PostInit((&testManagers{}).componentMocks(t))
	require.NoError(t, err)

	err = pc.Start()
	assert.Regexp(t, "bind", err)
}

func TestInitPluginManagerUDSTooLong(t *testing.T) {
	longerThanUDSSafelySupportsCrossPlatform := make([]rune, 187)
	for i := 0; i < len(longerThanUDSSafelySupportsCrossPlatform); i++ {
		longerThanUDSSafelySupportsCrossPlatform[i] = (rune)('a' + (i % 26))
	}

	pc := NewPluginManager(context.Background(),
		string(longerThanUDSSafelySupportsCrossPlatform), /* can't use a dir as a socket */
		uuid.New(), &PluginManagerConfig{},
	)

	err := pc.PostInit((&testManagers{}).componentMocks(t))
	assert.Regexp(t, "PD011204", err)
}

func TestInitPluginManagerTCP4(t *testing.T) {
	longerThanUDSSafelySupportsCrossPlatform := make([]rune, 187)
	for i := 0; i < len(longerThanUDSSafelySupportsCrossPlatform); i++ {
		longerThanUDSSafelySupportsCrossPlatform[i] = (rune)('a' + (i % 26))
	}

	pc := NewPluginManager(context.Background(),
		"tcp4:127.0.0.1:0",
		uuid.New(), &PluginManagerConfig{},
	)
	err := pc.PostInit((&testManagers{}).componentMocks(t))
	require.NoError(t, err)

	err = pc.Start()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(pc.GRPCTargetURL(), "dns:///"))
}

func TestInitPluginManagerTCP6(t *testing.T) {
	longerThanUDSSafelySupportsCrossPlatform := make([]rune, 187)
	for i := 0; i < len(longerThanUDSSafelySupportsCrossPlatform); i++ {
		longerThanUDSSafelySupportsCrossPlatform[i] = (rune)('a' + (i % 26))
	}

	pc := NewPluginManager(context.Background(),
		"tcp6:[::1]:0",
		uuid.New(), &PluginManagerConfig{},
	)
	err := pc.PostInit((&testManagers{}).componentMocks(t))
	require.NoError(t, err)

	err = pc.Start()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(pc.GRPCTargetURL(), "dns:///"))
}

func TestNotifyPluginUpdateNotStarted(t *testing.T) {
	pc := NewPluginManager(context.Background(), tempUDS(t), uuid.New(), &PluginManagerConfig{})
	err := pc.PostInit((&testManagers{}).componentMocks(t))
	require.NoError(t, err)

	err = pc.WaitForInit(context.Background())
	require.NoError(t, err)

	err = pc.ReloadPluginList()
	require.NoError(t, err)
	err = pc.ReloadPluginList()
	require.NoError(t, err)
}

func TestLoaderErrors(t *testing.T) {
	ctx := context.Background()
	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin[prototk.DomainMessage]{
				t:              t,
				connectFactory: domainConnectFactory,
				headerAccessor: domainHeaderAccessor,
				conf: &components.PluginConfig{
					Type:    components.LibraryTypeCShared.Enum(),
					Library: "some/where",
				},
			},
		},
	}
	pc := NewPluginManager(ctx,
		"tcp:127.0.0.1:0",
		uuid.New(),
		&PluginManagerConfig{
			GRPC: GRPCConfig{
				ShutdownTimeout: confutil.P("1ms"),
			},
		})
	err := pc.PostInit((&testManagers{testDomainManager: tdm}).componentMocks(t))
	require.NoError(t, err)

	err = pc.Start()
	require.NoError(t, err)

	conn, err := grpc.NewClient(pc.GRPCTargetURL(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close() // will close all the child conns too

	client := prototk.NewPluginControllerClient(conn)

	// first load with wrong ID
	wrongLoader, err := client.InitLoader(ctx, &prototk.PluginLoaderInit{
		Id: uuid.NewString(),
	})
	require.NoError(t, err)
	_, err = wrongLoader.Recv()
	assert.Regexp(t, "PD011200", err)

	// then load correctly
	loaderStream, err := client.InitLoader(ctx, &prototk.PluginLoaderInit{
		Id: pc.LoaderID().String(),
	})
	require.NoError(t, err)

	loadReq, err := loaderStream.Recv()
	require.NoError(t, err)

	_, err = client.LoadFailed(ctx, &prototk.PluginLoadFailed{
		Plugin:       loadReq.Plugin,
		ErrorMessage: "pop",
	})
	require.NoError(t, err)

	// We should be notified of the error if we were waiting
	err = pc.WaitForInit(ctx)
	assert.Regexp(t, "pop", err)

	// then attempt double start of the loader
	dupLoader, err := client.InitLoader(ctx, &prototk.PluginLoaderInit{
		Id: pc.LoaderID().String(),
	})
	require.NoError(t, err)
	_, err = dupLoader.Recv()
	assert.Regexp(t, "PD011201", err)

	// If we come back, we won't be (only one caller of WaitForInit supported)
	// - check it times out context not an error on load
	cancelled, cancelCtx := context.WithCancel(context.Background())
	cancelCtx()
	err = pc.WaitForInit(cancelled)
	assert.Regexp(t, "PD010301", err)

	err = loaderStream.CloseSend()
	require.NoError(t, err)

	// Notify of a plugin after closed stream
	tdm.domains = map[string]plugintk.Plugin{
		"domain2": &mockPlugin[prototk.DomainMessage]{
			t:              t,
			connectFactory: domainConnectFactory,
			headerAccessor: domainHeaderAccessor,
			conf: &components.PluginConfig{
				Type:    components.LibraryTypeJar.Enum(),
				Library: "some/where/else",
			},
		},
	}
	err = pc.ReloadPluginList()
	require.NoError(t, err)

	pc.Stop()

	// Also check we don't block on the LoadFailed notification if the channel gets full (which it will after stop)
	for i := 0; i < 3; i++ {
		_, _ = pc.(*pluginManager).LoadFailed(context.Background(), &prototk.PluginLoadFailed{Plugin: &prototk.PluginInfo{}})
	}
}
