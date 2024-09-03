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
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	prototk "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func tempUDS(t *testing.T) string {
	// Not safe to use t.TempDir() as it generates too long paths including the test name
	f, err := os.CreateTemp("", "ut_*.sock")
	assert.NoError(t, err)
	_ = f.Close()
	allocatedUDSName := f.Name()
	err = os.Remove(allocatedUDSName)
	assert.NoError(t, err)
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

func (ts *testManagers) DomainRegistration() DomainRegistration {
	if ts.testDomainManager == nil {
		ts.testDomainManager = &testDomainManager{}
	}
	return ts.testDomainManager
}

func (ts *testManagers) TransportRegistration() TransportRegistration {
	if ts.testTransportManager == nil {
		ts.testTransportManager = &testTransportManager{}
	}
	return ts.testTransportManager
}

func (ts *testManagers) RegistryRegistration() RegistryRegistration {
	if ts.testRegistryManager == nil {
		ts.testRegistryManager = &testRegistryManager{}
	}
	return ts.testRegistryManager
}

func (ts *testManagers) allPlugins() map[string]plugintk.Plugin {
	testPlugins := make(map[string]plugintk.Plugin)
	for name, td := range ts.DomainRegistration().(*testDomainManager).domains {
		testPlugins[name] = td
	}
	for name, td := range ts.TransportRegistration().(*testTransportManager).transports {
		testPlugins[name] = td
	}
	for name, td := range ts.RegistryRegistration().(*testRegistryManager).registries {
		testPlugins[name] = td
	}
	return testPlugins
}

func TestControllerStartGracefulShutdownNoConns(t *testing.T) {
	pc, err := NewPluginController(context.Background(), tempUDS(t), uuid.New(), &testManagers{}, &PluginControllerConfig{})
	assert.NoError(t, err)
	err = pc.Start()
	assert.NoError(t, err)
	pc.Stop()
}

func TestInitPluginControllerBadPlugin(t *testing.T) {
	tdm := &testDomainManager{domains: map[string]plugintk.Plugin{
		"!badname": &mockPlugin[prototk.DomainMessage]{},
	}}
	_, err := NewPluginController(context.Background(), tempUDS(t), uuid.New(), &testManagers{testDomainManager: tdm}, &PluginControllerConfig{})
	assert.Regexp(t, "PD011106", err)
}

func TestInitPluginControllerBadSocket(t *testing.T) {
	pc, err := NewPluginController(context.Background(),
		t.TempDir(), /* can't use a dir as a socket */
		uuid.New(), &testManagers{}, &PluginControllerConfig{},
	)
	assert.NoError(t, err)

	err = pc.Start()
	assert.Regexp(t, "bind", err)
}

func TestInitPluginControllerUDSTooLong(t *testing.T) {
	longerThanUDSSafelySupportsCrossPlatform := make([]rune, 187)
	for i := 0; i < len(longerThanUDSSafelySupportsCrossPlatform); i++ {
		longerThanUDSSafelySupportsCrossPlatform[i] = (rune)('a' + (i % 26))
	}

	_, err := NewPluginController(context.Background(),
		string(longerThanUDSSafelySupportsCrossPlatform), /* can't use a dir as a socket */
		uuid.New(), &testManagers{}, &PluginControllerConfig{},
	)

	assert.Regexp(t, "PD011204", err)
}

func TestInitPluginControllerTCP4(t *testing.T) {
	longerThanUDSSafelySupportsCrossPlatform := make([]rune, 187)
	for i := 0; i < len(longerThanUDSSafelySupportsCrossPlatform); i++ {
		longerThanUDSSafelySupportsCrossPlatform[i] = (rune)('a' + (i % 26))
	}

	pc, err := NewPluginController(context.Background(),
		"tcp4:127.0.0.1:0",
		uuid.New(), &testManagers{}, &PluginControllerConfig{},
	)
	assert.NoError(t, err)

	err = pc.Start()
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(pc.GRPCTargetURL(), "dns:///"))
}

func TestInitPluginControllerTCP6(t *testing.T) {
	longerThanUDSSafelySupportsCrossPlatform := make([]rune, 187)
	for i := 0; i < len(longerThanUDSSafelySupportsCrossPlatform); i++ {
		longerThanUDSSafelySupportsCrossPlatform[i] = (rune)('a' + (i % 26))
	}

	pc, err := NewPluginController(context.Background(),
		"tcp6:[::1]:0",
		uuid.New(), &testManagers{}, &PluginControllerConfig{},
	)
	assert.NoError(t, err)

	err = pc.Start()
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(pc.GRPCTargetURL(), "dns:///"))
}

func TestNotifyPluginUpdateNotStarted(t *testing.T) {
	pc, err := NewPluginController(context.Background(), tempUDS(t), uuid.New(), &testManagers{}, &PluginControllerConfig{})
	assert.NoError(t, err)

	err = pc.WaitForInit(context.Background())
	assert.NoError(t, err)

	err = pc.ReloadPluginList()
	assert.NoError(t, err)
	err = pc.ReloadPluginList()
	assert.NoError(t, err)
}

func TestLoaderErrors(t *testing.T) {
	ctx := context.Background()
	tdm := &testDomainManager{
		domains: map[string]plugintk.Plugin{
			"domain1": &mockPlugin[prototk.DomainMessage]{
				connectFactory: domainConnectFactory,
				headerAccessor: domainHeaderAccessor,
				conf: &PluginConfig{
					Type:    LibraryTypeCShared.Enum(),
					Library: "some/where",
				},
			},
		},
	}
	pc, err := NewPluginController(ctx,
		"tcp:127.0.0.1:0",
		uuid.New(),
		&testManagers{testDomainManager: tdm}, &PluginControllerConfig{
			GRPC: GRPCConfig{
				ShutdownTimeout: confutil.P("1ms"),
			},
		})
	assert.NoError(t, err)

	err = pc.Start()
	assert.NoError(t, err)

	conn, err := grpc.NewClient(pc.GRPCTargetURL(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer conn.Close() // will close all the child conns too

	client := prototk.NewPluginControllerClient(conn)

	// first load with wrong ID
	wrongLoader, err := client.InitLoader(ctx, &prototk.PluginLoaderInit{
		Id: uuid.NewString(),
	})
	assert.NoError(t, err)
	_, err = wrongLoader.Recv()
	assert.Regexp(t, "PD011200", err)

	// then load correctly
	loaderStream, err := client.InitLoader(ctx, &prototk.PluginLoaderInit{
		Id: pc.LoaderID().String(),
	})
	assert.NoError(t, err)

	loadReq, err := loaderStream.Recv()
	assert.NoError(t, err)

	_, err = client.LoadFailed(ctx, &prototk.PluginLoadFailed{
		Plugin:       loadReq.Plugin,
		ErrorMessage: "pop",
	})
	assert.NoError(t, err)

	// We should be notified of the error if we were waiting
	err = pc.WaitForInit(ctx)
	assert.Regexp(t, "pop", err)

	// then attempt double start of the loader
	dupLoader, err := client.InitLoader(ctx, &prototk.PluginLoaderInit{
		Id: pc.LoaderID().String(),
	})
	assert.NoError(t, err)
	_, err = dupLoader.Recv()
	assert.Regexp(t, "PD011201", err)

	// If we come back, we won't be (only one caller of WaitForInit supported)
	// - check it times out context not an error on load
	cancelled, cancelCtx := context.WithCancel(context.Background())
	cancelCtx()
	err = pc.WaitForInit(cancelled)
	assert.Regexp(t, "PD010301", err)

	err = loaderStream.CloseSend()
	assert.NoError(t, err)

	// Notify of a plugin after closed stream
	tdm.domains = map[string]plugintk.Plugin{
		"domain2": &mockPlugin[prototk.DomainMessage]{
			connectFactory: domainConnectFactory,
			headerAccessor: domainHeaderAccessor,
			conf: &PluginConfig{
				Type:    LibraryTypeJar.Enum(),
				Library: "some/where/else",
			},
		},
	}
	err = pc.ReloadPluginList()
	assert.NoError(t, err)

	pc.Stop()

	// Also check we don't block on the LoadFailed notification if the channel gets full (which it will after stop)
	for i := 0; i < 3; i++ {
		_, _ = pc.(*pluginController).LoadFailed(context.Background(), &prototk.PluginLoadFailed{Plugin: &prototk.PluginInfo{}})
	}
}
