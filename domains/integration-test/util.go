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

package integrationtest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/noto/pkg/noto"
	nototypes "github.com/kaleido-io/paladin/domains/noto/pkg/types"
	zetotypes "github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zeto"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

func mapConfig(t *testing.T, config any) (m map[string]any) {
	configJSON, err := json.Marshal(&config)
	assert.NoError(t, err)
	err = json.Unmarshal(configJSON, &m)
	assert.NoError(t, err)
	return m
}

func newTestbed(t *testing.T, hdWalletSeed *testbed.UTInitFunction, domains map[string]*testbed.TestbedDomain) (context.CancelFunc, *pldconf.PaladinConfig, testbed.Testbed, rpcbackend.Backend) {
	tb := testbed.NewTestBed()
	url, conf, done, err := tb.StartForTest("./testbed.config.yaml", domains, hdWalletSeed)
	assert.NoError(t, err)
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return done, conf, tb, rpc
}

func deployContracts(ctx context.Context, t *testing.T, hdWalletSeed *testbed.UTInitFunction, deployer string, contracts map[string][]byte) map[string]string {
	tb := testbed.NewTestBed()
	url, _, done, err := tb.StartForTest("./testbed.config.yaml", map[string]*testbed.TestbedDomain{}, hdWalletSeed)
	assert.NoError(t, err)
	defer done()
	rpc := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))

	deployed := make(map[string]string, len(contracts))
	for name, contract := range contracts {
		build := domain.LoadBuild(contract)
		var addr string
		rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode",
			deployer, build.ABI, build.Bytecode.String(), tktypes.RawJSON(`{}`))
		if rpcerr != nil {
			assert.NoError(t, rpcerr.Error())
		}
		deployed[name] = addr
	}
	return deployed
}

func newNotoDomain(t *testing.T, config *nototypes.DomainConfig) (chan noto.Noto, *testbed.TestbedDomain) {
	waitForDomain := make(chan noto.Noto, 1)
	tbd := &testbed.TestbedDomain{
		Config: mapConfig(t, config),
		Plugin: plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
			domain := noto.New(callbacks)
			waitForDomain <- domain
			return domain
		}),
		RegistryAddress: tktypes.MustEthAddress(config.FactoryAddress),
	}
	return waitForDomain, tbd
}

func newZetoDomain(t *testing.T, config *zetotypes.DomainFactoryConfig) (chan zeto.Zeto, *testbed.TestbedDomain) {
	waitForDomain := make(chan zeto.Zeto, 1)
	tbd := &testbed.TestbedDomain{
		Config: mapConfig(t, config),
		Plugin: plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
			domain := zeto.New(callbacks)
			waitForDomain <- domain
			return domain
		}),
		RegistryAddress: tktypes.MustEthAddress(config.FactoryAddress),
		AllowSigning:    true,
	}
	return waitForDomain, tbd
}
