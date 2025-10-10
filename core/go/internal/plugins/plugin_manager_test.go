/*
 * Copyright © 2025 Kaleido, Inc.
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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	prototk "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestAllLibraryTypesMapped(t *testing.T) {
	// Validates we don't change the map in config, without changing this map
	var libraryTypeEnum pldtypes.Enum[pldtypes.LibraryType]
	for _, o := range libraryTypeEnum.V().Options() {
		protoLibType, err := MapLibraryTypeToProto(pldtypes.LibraryType(o).Enum())
		require.NoError(t, err)
		require.NotEmpty(t, string(protoLibType))
	}
}

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
	testKeyManager       *testKeyManager
}

func (tm *testManagers) componentsmocks(t *testing.T) *componentsmocks.AllComponents {
	mm := metrics.NewMetricsManager(context.Background())
	mc := componentsmocks.NewAllComponents(t)
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
	if tm.testKeyManager == nil {
		tm.testKeyManager = &testKeyManager{}
	}
	mc.On("KeyManager").Return(tm.testKeyManager.mock(t)).Maybe()
	mc.On("MetricsManager").Return(mm).Maybe()

	return mc
}

func (ts *testManagers) allPlugins() map[string]plugintk.Plugin {
	testPlugins := make(map[string]plugintk.Plugin)
	for name, td := range ts.testDomainManager.domains {
		testPlugins[name] = td
	}
	for name, tt := range ts.testTransportManager.transports {
		testPlugins[name] = tt
	}
	for name, tr := range ts.testRegistryManager.registries {
		testPlugins[name] = tr
	}
	for name, tsm := range ts.testKeyManager.signingModules {
		testPlugins[name] = tsm
	}
	return testPlugins
}

func newTestPluginManager(t *testing.T, setup *testManagers) *pluginManager {
	udsString := tempUDS(t)
	loaderId := uuid.New()
	pc := NewPluginManager(context.Background(), udsString, loaderId, &pldconf.PluginManagerConfig{
		GRPC: pldconf.GRPCConfig{
			ShutdownTimeout: confutil.P("1ms"),
		},
	})
	mc := setup.componentsmocks(t)
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
	pc := NewPluginManager(context.Background(), tempUDS(t), uuid.New(), &pldconf.PluginManagerConfig{})
	err := pc.PostInit((&testManagers{testDomainManager: tdm}).componentsmocks(t))
	assert.Regexp(t, "PD020005", err)
}

func TestInitPluginManagerBadSocket(t *testing.T) {
	pc := NewPluginManager(context.Background(),
		t.TempDir(), /* can't use a dir as a socket */
		uuid.New(), &pldconf.PluginManagerConfig{},
	)
	err := pc.PostInit((&testManagers{}).componentsmocks(t))
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
		uuid.New(), &pldconf.PluginManagerConfig{},
	)

	err := pc.PostInit((&testManagers{}).componentsmocks(t))
	assert.Regexp(t, "PD011204", err)
}

func TestInitPluginManagerTCP4(t *testing.T) {
	longerThanUDSSafelySupportsCrossPlatform := make([]rune, 187)
	for i := 0; i < len(longerThanUDSSafelySupportsCrossPlatform); i++ {
		longerThanUDSSafelySupportsCrossPlatform[i] = (rune)('a' + (i % 26))
	}

	pc := NewPluginManager(context.Background(),
		"tcp4:127.0.0.1:0",
		uuid.New(), &pldconf.PluginManagerConfig{},
	)
	err := pc.PostInit((&testManagers{}).componentsmocks(t))
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
		uuid.New(), &pldconf.PluginManagerConfig{},
	)
	err := pc.PostInit((&testManagers{}).componentsmocks(t))
	require.NoError(t, err)

	err = pc.Start()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(pc.GRPCTargetURL(), "dns:///"))
}

func TestNotifyPluginUpdateNotStarted(t *testing.T) {
	pc := NewPluginManager(context.Background(), tempUDS(t), uuid.New(), &pldconf.PluginManagerConfig{})
	err := pc.PostInit((&testManagers{}).componentsmocks(t))
	require.NoError(t, err)

	err = pc.WaitForInit(context.Background(), prototk.PluginInfo_DOMAIN)
	require.NoError(t, err)
	err = pc.WaitForInit(context.Background(), prototk.PluginInfo_REGISTRY)
	require.NoError(t, err)
	err = pc.WaitForInit(context.Background(), prototk.PluginInfo_SIGNING_MODULE)
	require.NoError(t, err)
	err = pc.WaitForInit(context.Background(), prototk.PluginInfo_TRANSPORT)
	require.NoError(t, err)

	err = pc.ReloadPluginList()
	require.NoError(t, err)
	err = pc.ReloadPluginList()
	require.NoError(t, err)

	// System command sending should not block
	pc.SendSystemCommandToLoader(prototk.PluginLoad_THREAD_DUMP)
	pc.SendSystemCommandToLoader(prototk.PluginLoad_THREAD_DUMP)
}

func TestLoaderErrors(t *testing.T) {
	ctx := context.Background()
	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin[prototk.DomainMessage]{
				t:              t,
				connectFactory: domainConnectFactory,
				headerAccessor: domainHeaderAccessor,
				conf: &pldconf.PluginConfig{
					Type:    string(pldtypes.LibraryTypeCShared),
					Library: "some/where",
				},
			},
		},
	}
	pc := NewPluginManager(ctx,
		"tcp:127.0.0.1:0",
		uuid.New(),
		&pldconf.PluginManagerConfig{
			GRPC: pldconf.GRPCConfig{
				ShutdownTimeout: confutil.P("1ms"),
			},
		})
	err := pc.PostInit((&testManagers{testDomainManager: tdm}).componentsmocks(t))
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
	err = pc.WaitForInit(ctx, prototk.PluginInfo_DOMAIN)
	assert.Regexp(t, "pop", err)

	// Get a system command
	pc.SendSystemCommandToLoader(prototk.PluginLoad_THREAD_DUMP)
	loadReq, err = loaderStream.Recv()
	require.NoError(t, err)
	require.Equal(t, prototk.PluginLoad_THREAD_DUMP, *loadReq.SysCommand)

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
	err = pc.WaitForInit(cancelled, prototk.PluginInfo_DOMAIN)
	assert.Regexp(t, "PD010301", err)

	err = loaderStream.CloseSend()
	require.NoError(t, err)

	// Notify of a plugin after closed stream
	tdm.domains = map[string]plugintk.Plugin{
		"domain2": &mockPlugin[prototk.DomainMessage]{
			t:              t,
			connectFactory: domainConnectFactory,
			headerAccessor: domainHeaderAccessor,
			conf: &pldconf.PluginConfig{
				Type:    string(pldtypes.LibraryTypeJar),
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
