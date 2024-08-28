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

package testbed

import (
	"context"
	"fmt"
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/componentmgr"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/domainmgr"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"gopkg.in/yaml.v3"
)

type Testbed interface {
	StartForTest(t *testing.T, configFile string, domains map[string]*TestbedDomain) (url string, done func())
}

type TestbedDomain struct {
	Config yaml.Node
	Plugin plugintk.Plugin
}

type testbed struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	rpcModule *rpcserver.RPCModule
	c         components.AllComponents
}

func NewTestBed() (tb *testbed) {
	tb = &testbed{}
	tb.ctx, tb.cancelCtx = context.WithCancel(context.Background())
	tb.initRPC()
	return tb
}

func (tb *testbed) EngineName() string {
	return "testbed"
}

func (tb *testbed) Start() error {
	// we don't have anything additional that runs beyond the components
	return nil
}

func (tb *testbed) Stop() {}

func (tb *testbed) Init(c components.AllComponents) (*components.ManagerInitResult, error) {
	tb.c = c
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{tb.rpcModule},
	}, nil
}

func (tb *testbed) StartForTest(configFile string, domains map[string]*TestbedDomain) (url string, done func(), err error) {
	ctx := context.Background()

	var conf *componentmgr.Config
	if err = componentmgr.ReadAndParseYAMLFile(ctx, configFile, &conf); err != nil {
		return "", nil, err
	}

	conf.DomainManagerConfig.Domains = make(map[string]*domainmgr.DomainConfig, len(domains))
	for name, domain := range domains {
		conf.DomainManagerConfig.Domains[name] = &domainmgr.DomainConfig{
			Plugin: plugins.PluginConfig{
				Type:    plugins.LibraryTypeCShared.Enum(),
				Library: "loaded/via/unit/test/loader",
			},
			Config: domain.Config,
		}
	}

	pluginInit := func(c components.AllComponents) (err error) {
		for name, domain := range domains {
			pc := c.PluginController()
			pl, err := plugins.NewUnitTestPluginLoader(pc.GRPCTargetURL(), pc.LoaderID().String(), map[string]plugintk.Plugin{
				name: domain.Plugin,
			})
			if err != nil {
				return err
			}
			go pl.Run()
		}
		return nil
	}

	cm, err := componentmgr.UnitTestStart(ctx, conf, tb, pluginInit)
	if err != nil {
		return "", nil, err
	}

	return fmt.Sprintf("http://%s", tb.c.RPCServer().HTTPAddr()), func() {
		cm.Stop()
	}, nil
}
