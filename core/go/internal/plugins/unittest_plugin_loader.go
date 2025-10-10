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
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type UnitTestPluginLoader interface {
	Run()  // runs in foreground
	Stop() // causes the run function to terminate all connections to plugins and wait to exit
}

type testPluginLoader struct {
	ctx          context.Context
	cancelCtx    context.CancelFunc
	grpcTarget   string
	loaderID     string
	plugins      map[string]plugintk.Plugin
	conn         *grpc.ClientConn
	loaderStream grpc.ServerStreamingClient[prototk.PluginLoad]
	wg           sync.WaitGroup
}

// Provides a convenient way for unit tests (in this package, and in the test bed)
// to load up Go coded plugins in-process, without having to first compile them
// to a C-Shared library.
func NewUnitTestPluginLoader(grpcTarget, loaderID string, plugins map[string]plugintk.Plugin) (_ UnitTestPluginLoader, err error) {
	tpl := &testPluginLoader{
		grpcTarget: grpcTarget,
		loaderID:   loaderID,
		plugins:    plugins,
	}
	tpl.ctx, tpl.cancelCtx = context.WithCancel(context.Background())

	tpl.conn, err = grpc.NewClient(tpl.grpcTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err == nil {
		client := prototk.NewPluginControllerClient(tpl.conn)
		tpl.loaderStream, err = client.InitLoader(tpl.ctx, &prototk.PluginLoaderInit{
			Id: tpl.loaderID,
		})
	}
	return tpl, err
}

func (tpl *testPluginLoader) Stop() {
	_ = tpl.conn.Close()
	tpl.cancelCtx()
	for _, p := range tpl.plugins {
		p.Stop()
	}
	tpl.wg.Wait()
}

func (tpl *testPluginLoader) Run() {
	tpl.wg.Add(1)
	defer func() {
		tpl.wg.Done()
	}()

	// We just run until the stream is closed
	for {
		msg, err := tpl.loaderStream.Recv()
		if err != nil {
			log.L(tpl.ctx).Infof("in-process loaded exiting: %s", err)
			return
		}
		tp := tpl.plugins[msg.Plugin.Name]
		if tp != nil {
			tpl.wg.Add(1)
			go func() {
				defer tpl.wg.Done()
				tp.Run(tpl.grpcTarget, msg.Plugin.Id)
			}()
		}
	}

}
