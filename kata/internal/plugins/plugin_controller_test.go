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
	"path"
	"sync"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/google/uuid"
	pbp "github.com/kaleido-io/paladin/kata/pkg/proto/plugins"
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
		assert.NoError(t, err)
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
			DomainPlugins: make(map[string]*PluginConfig),
		},
	}
	testPlugins := make(map[string]testPlugin)
	for name, td := range testDomains {
		args.InitialConfig.DomainPlugins[name] = td.conf()
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

	err = pc.WaitForInit(ctx)
	assert.NoError(t, err)

	return ctx, pc.(*pluginController), func() {
		cancelCtx()
		pc.Stop(ctx)
		<-tpl.done
	}

}
