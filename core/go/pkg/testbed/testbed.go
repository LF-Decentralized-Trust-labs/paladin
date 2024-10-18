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
	"os"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/componentmgr"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/plugins"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"

	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func HDWalletSeedScopedToTest() *UTInitFunction {
	seed := tktypes.RandHex(32)
	return &UTInitFunction{
		ModifyConfig: func(conf *pldconf.PaladinConfig) {
			conf.Wallets[0].Signer.KeyStore.Static.Keys["seed"] = pldconf.StaticKeyEntryConfig{
				Encoding: "hex",
				Inline:   seed,
			}
		},
	}
}

type KeyMapping = pldapi.KeyMappingAndVerifier

type Testbed interface {
	components.AdditionalManager
	// Use GenerateSeed to get a valid seed
	StartForTest(configFile string, domains map[string]*TestbedDomain, initFunctions ...*UTInitFunction) (url string, conf *pldconf.PaladinConfig, done func(), err error)
	ResolveKey(ctx context.Context, fqLookup, algorithm, verifierType string) (resolvedKey *KeyMapping, err error)
	ExecTransactionSync(ctx context.Context, tx *pldapi.TransactionInput) (receipt *pldapi.TransactionReceipt, err error)
	ExecBaseLedgerCall(ctx context.Context, output any, tx *pldapi.TransactionInput) error
	EthClientKeyManagerShim() ethclient.KeyManager // CAREFUL - this will give you "nonce too low" if you clash with anything in-flight in Paladin managed TXs
	Components() AllComponents
}

type TestbedDomain struct {
	Config          map[string]any
	Plugin          plugintk.Plugin
	RegistryAddress *tktypes.EthAddress
	AllowSigning    bool
}

type testbed struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	rpcModule *rpcserver.RPCModule
	c         components.AllComponents
}

func NewTestBed() Testbed {
	tb := &testbed{}
	tb.ctx, tb.cancelCtx = context.WithCancel(context.Background())
	tb.initRPC()
	return tb
}

func (tb *testbed) Name() string {
	return "testbed"
}

func (tb *testbed) Start() error {
	// we don't have anything additional that runs beyond the components
	return nil
}

func (tb *testbed) Stop() {}

func (tb *testbed) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{tb.rpcModule},
	}, nil
}

func (tb *testbed) PostInit(c components.AllComponents) error {
	tb.c = c
	return nil
}

func (tb *testbed) Components() AllComponents {
	return tb.c
}

// redeclare the AllComponents interface to allow unit test
// code in the same package to access the AllComponents interface
// while keeping it internal
type AllComponents components.AllComponents

type UTInitFunction struct {
	ModifyConfig     func(conf *pldconf.PaladinConfig)
	PreManagerStart  func(c AllComponents) error
	PostManagerStart func(c AllComponents) error
}

func unitTestSocketFile() (fileName string, err error) {
	f, err := os.CreateTemp("", "testbed.paladin.*.sock")
	if err == nil {
		fileName = f.Name()
	}
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		err = os.Remove(fileName)
	}
	return
}

func unitTestComponentManagerStart(ctx context.Context, conf *pldconf.PaladinConfig, tb *testbed, callbacks ...*UTInitFunction) (cm componentmgr.ComponentManager, err error) {
	socketFile, err := unitTestSocketFile()
	if err == nil {
		cm = componentmgr.NewComponentManager(ctx, socketFile, uuid.New(), conf, tb)
		err = cm.Init()
	}
	for _, cb := range callbacks {
		if err == nil && cb.PreManagerStart != nil {
			err = cb.PreManagerStart(cm)
		}
	}
	if err == nil {
		err = cm.StartManagers()
	}
	for _, cb := range callbacks {
		if err == nil && cb.PostManagerStart != nil {
			err = cb.PostManagerStart(cm)
		}
	}
	if err == nil {
		err = cm.CompleteStart()
	}
	return cm, err
}

func (tb *testbed) ReceiveTransportMessage(context.Context, *components.TransportMessage) {
	// no-op
}

func (tb *testbed) StartForTest(configFile string, domains map[string]*TestbedDomain, initFunctions ...*UTInitFunction) (url string, conf *pldconf.PaladinConfig, done func(), err error) {
	ctx := context.Background()

	if err = pldconf.ReadAndParseYAMLFile(ctx, configFile, &conf); err != nil {
		return "", nil, nil, err
	}

	for _, init := range initFunctions {
		if init.ModifyConfig != nil {
			init.ModifyConfig(conf)
		}
	}

	conf.DomainManagerConfig.Domains = make(map[string]*pldconf.DomainConfig, len(domains))
	for name, domain := range domains {
		conf.DomainManagerConfig.Domains[name] = &pldconf.DomainConfig{
			Plugin: pldconf.PluginConfig{
				Type:    string(tktypes.LibraryTypeCShared),
				Library: "loaded/via/unit/test/loader",
			},
			Config:          domain.Config,
			RegistryAddress: domain.RegistryAddress.String(),
			AllowSigning:    domain.AllowSigning,
		}
	}

	var pl plugins.UnitTestPluginLoader
	initFunctions = append(initFunctions,
		// We add an init function that loads the plugin loader after the plugin controller has started.
		&UTInitFunction{
			PostManagerStart: func(c AllComponents) (err error) {
				loaderMap := make(map[string]plugintk.Plugin)
				for name, domain := range domains {
					loaderMap[name] = domain.Plugin
				}
				pc := c.PluginManager()
				pl, err = plugins.NewUnitTestPluginLoader(pc.GRPCTargetURL(), pc.LoaderID().String(), loaderMap)
				if err != nil {
					return err
				}
				go pl.Run()
				return nil
			},
		},
	)

	cm, err := unitTestComponentManagerStart(ctx, conf, tb, initFunctions...)
	if err != nil {
		return "", nil, nil, err
	}

	return fmt.Sprintf("http://%s", tb.c.RPCServer().HTTPAddr()), conf, func() {
		cm.Stop()
		if pl != nil {
			pl.Stop()
		}
	}, nil
}
