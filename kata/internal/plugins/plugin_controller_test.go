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
	"fmt"
	"os"
	"path"
	"runtime/debug"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	pbp "github.com/kaleido-io/paladin/kata/pkg/proto/plugins"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testPlugin interface {
	conf() *PluginConfig
	run(t *testing.T, ctx context.Context, id string, client pbp.PluginControllerClient)
}

type testPluginLoader struct {
	plugins map[string]testPlugin
	done    chan struct{}
}

func (tpl *testPluginLoader) run(t *testing.T, ctx context.Context, socketAddress string, loaderID uuid.UUID) {
	wg := new(sync.WaitGroup)
	defer func() {
		wg.Wait()
		close(tpl.done)
	}()

	conn, err := grpc.NewClient("unix:"+socketAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer conn.Close() // will close all the child conns too

	client := pbp.NewPluginControllerClient(conn)

	loaderStream, err := client.InitLoader(ctx, &pbp.PluginLoaderInit{
		Id: loaderID.String(),
	})
	assert.NoError(t, err)

	for {
		msg, err := loaderStream.Recv()
		if err != nil {
			log.L(ctx).Infof("loader stream closed: %s", err)
			return
		}
		tp := tpl.plugins[msg.Plugin.Name]
		if tp != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tp.run(t, ctx, msg.Plugin.Id, client)
			}()
		}
	}

}

func newTestDomainPluginController(t *testing.T, tdm *testDomainManager, testDomains map[string]*testDomain) (context.Context, *pluginController, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	args := &PluginControllerArgs{
		DomainManager: tdm,
		SocketAddress: path.Join(t.TempDir(), "ut.sock"),
		LoaderID:      uuid.New(),
		InitialConfig: &PluginControllerConfig{
			GRPC: GRPCConfig{
				ShutdownTimeout: confutil.P("1ms"),
			},
			Domains: make(map[string]*PluginConfig),
		},
	}
	testPlugins := make(map[string]testPlugin)
	for name, td := range testDomains {
		args.InitialConfig.Domains[name] = td.conf()
		testPlugins[name] = td
	}

	pc, err := NewPluginController(ctx, args)
	assert.NoError(t, err)

	tpl := &testPluginLoader{
		plugins: testPlugins,
		done:    make(chan struct{}),
	}
	go tpl.run(t, ctx, pc.SocketAddress(), pc.LoaderID())

	go func() {
		err := pc.Run(ctx)
		assert.NoError(t, err)
	}()

	return ctx, pc.(*pluginController), func() {
		recovered := recover()
		if recovered != nil {
			fmt.Fprintf(os.Stderr, "%v: %s", recovered, debug.Stack())
			panic(recovered)
		}
		cancelCtx()
		pc.Stop(ctx)
		<-tpl.done
	}

}

func TestInitPluginControllerBadPlugin(t *testing.T) {
	args := &PluginControllerArgs{
		DomainManager: nil,
		SocketAddress: path.Join(t.TempDir(), "ut.sock"),
		LoaderID:      uuid.New(),
		InitialConfig: &PluginControllerConfig{
			Domains: map[string]*PluginConfig{
				"!badname": {},
			},
		},
	}
	_, err := NewPluginController(context.Background(), args)
	assert.Regexp(t, "PD011106", err)
}

func TestInitPluginControllerBadSocket(t *testing.T) {
	args := &PluginControllerArgs{
		DomainManager: nil,
		SocketAddress: t.TempDir(), // can't use a dir as a socket
		LoaderID:      uuid.New(),
		InitialConfig: &PluginControllerConfig{},
	}
	_, err := NewPluginController(context.Background(), args)
	assert.Regexp(t, "bind", err)
}

func TestNotifyPluginUpdateNotStarted(t *testing.T) {
	args := &PluginControllerArgs{
		DomainManager: nil,
		SocketAddress: path.Join(t.TempDir(), "ut.sock"),
		LoaderID:      uuid.New(),
		InitialConfig: &PluginControllerConfig{},
	}
	pc, err := NewPluginController(context.Background(), args)
	assert.NoError(t, err)

	err = pc.WaitForInit(context.Background())
	assert.NoError(t, err)

	err = pc.PluginsUpdated(&PluginControllerConfig{})
	assert.NoError(t, err)
	err = pc.PluginsUpdated(&PluginControllerConfig{})
	assert.NoError(t, err)
}

func TestInflightHandleBadCorrelIDs(t *testing.T) {
	args := &PluginControllerArgs{
		DomainManager: nil,
		SocketAddress: path.Join(t.TempDir(), "ut.sock"),
		LoaderID:      uuid.New(),
		InitialConfig: &PluginControllerConfig{},
	}
	pc, err := NewPluginController(context.Background(), args)
	assert.NoError(t, err)

	inFlight := pc.(*pluginController).domainRequests
	assert.Nil(t, inFlight.getInflight(context.Background(), nil))
	assert.Nil(t, inFlight.getInflight(context.Background(), confutil.P("wrong")))
}

func TestLoaderErrors(t *testing.T) {
	ctx := context.Background()
	args := &PluginControllerArgs{
		DomainManager: nil,
		SocketAddress: path.Join(t.TempDir(), "ut.sock"),
		LoaderID:      uuid.New(),
		InitialConfig: &PluginControllerConfig{
			GRPC: GRPCConfig{
				ShutdownTimeout: confutil.P("1ms"),
			},
			Domains: map[string]*PluginConfig{
				"domain1": {
					Type:     LibraryTypeJar.Enum(),
					Location: "some/where",
				},
			},
		},
	}
	pc, err := NewPluginController(ctx, args)
	assert.NoError(t, err)

	conn, err := grpc.NewClient("unix:"+pc.SocketAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer conn.Close() // will close all the child conns too

	client := pbp.NewPluginControllerClient(conn)

	go func() {
		err := pc.Run(ctx)
		assert.NoError(t, err)
	}()

	// first load with wrong ID
	wrongLoader, err := client.InitLoader(ctx, &pbp.PluginLoaderInit{
		Id: uuid.NewString(),
	})
	assert.NoError(t, err)
	_, err = wrongLoader.Recv()
	assert.Regexp(t, "PD011200", err)

	// then load correctly
	loaderStream, err := client.InitLoader(ctx, &pbp.PluginLoaderInit{
		Id: pc.LoaderID().String(),
	})
	assert.NoError(t, err)

	loadReq, err := loaderStream.Recv()
	assert.NoError(t, err)

	_, err = client.LoadFailed(ctx, &pbp.PluginLoadFailed{
		Plugin:       loadReq.Plugin,
		ErrorMessage: "pop",
	})
	assert.NoError(t, err)

	// We should be notified of the error if we were waiting
	err = pc.WaitForInit(ctx)
	assert.Regexp(t, "pop", err)

	// then attempt double start of the loader
	dupLoader, err := client.InitLoader(ctx, &pbp.PluginLoaderInit{
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
	err = pc.PluginsUpdated(&PluginControllerConfig{
		Domains: map[string]*PluginConfig{
			"domain2": {
				Type:     LibraryTypeCShared.Enum(),
				Location: "some/where/else",
			},
		},
	})
	assert.NoError(t, err)

	pc.Stop(ctx)

	// Also check we don't block on the LoadFailed notification if the channel gets full (which it will after stop)
	for i := 0; i < 3; i++ {
		_, _ = pc.(*pluginController).LoadFailed(context.Background(), &pbp.PluginLoadFailed{Plugin: &pbp.PluginInfo{}})
	}
}
