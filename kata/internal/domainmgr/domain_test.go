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

package main

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

const fakeCoinConstructorABI = `{
	"type": "constructor",
	"inputs": [
	  {
		"name": "notary",
		"type": "string"
	  },
	  {
		"name": "name",
		"type": "string"
	  },
	  {
		"name": "symbol",
		"type": "string"
	  }
	],
	"outputs": null
}`

const fakeCoinStateSchema = `{
	"type": "tuple",
	"internalType": "struct FakeCoin",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "owner",
			"type": "address",
			"indexed": true
		},
		{
			"name": "amount",
			"type": "uint256",
			"indexed": true
		}
	]
}`

type testPlugin struct {
	plugintk.DomainAPIBase
	initialized atomic.Bool
	d           *domain
}

func (tp *testPlugin) Initialized() {
	tp.initialized.Store(true)
}

func newTestPlugin(domainFuncs *plugintk.DomainAPIFunctions) *testPlugin {
	return &testPlugin{
		DomainAPIBase: plugintk.DomainAPIBase{
			Functions: domainFuncs,
		},
	}
}

func newTestDomain(t *testing.T, realDB bool, domainConfig *prototk.DomainConfig) (context.Context, *domainManager, *testPlugin, error, func()) {

	ctx, dm, _, done := newTestDomainManager(t, realDB, &DomainManagerConfig{
		Domains: map[string]*DomainConfig{
			"test1": {
				Config: yamlNode(t, `{"some":"conf"}`),
			},
		},
	})
	defer done()

	tp := newTestPlugin(&plugintk.DomainAPIFunctions{
		ConfigureDomain: func(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
			assert.Equal(t, "test1", cdr.Name)
			assert.YAMLEq(t, `{"some":"conf"}`, cdr.ConfigYaml)
			return &prototk.ConfigureDomainResponse{
				DomainConfig: domainConfig,
			}, nil
		},
		InitDomain: func(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
			assert.Len(t, idr.AbiStateSchemas, 1)
			return &prototk.InitDomainResponse{}, nil
		},
	})

	domainID := uuid.New()
	_, err := dm.DomainRegistered("test1", domainID, tp)
	assert.NoError(t, err)

	da, err := dm.GetDomainByName(ctx, "test1")
	assert.NoError(t, err)

	tp.d = da.(*domain)
	tp.d.initRetry.UTSetMaxAttempts(1)
	return ctx, dm, tp, <-tp.d.initDone, done
}

func TestRegisterAndFindAvailableStates(t *testing.T) {

	addr := ethtypes.MustNewAddress(types.RandHex(20))
	ctx, dm, tp, regErr, done := newTestDomain(t, true, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: addr.String(),
		FactoryContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.NoError(t, regErr)
	assert.True(t, tp.initialized.Load())
	byAddr, err := dm.getDomainByAddress(ctx, addr)
	assert.NoError(t, err)
	assert.Equal(t, tp.d, byAddr)

}

func TestRegisterAndFindAvailableBadSchemas(t *testing.T) {
	_, _, tp, regErr, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: ethtypes.MustNewAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			`!!! Wrong`,
		},
	})
	defer done()
	assert.Regexp(t, "PD011602", regErr)
	assert.False(t, tp.initialized.Load())
}

func TestRegisterAndFindAvailableBadConstructor(t *testing.T) {
	_, _, tp, regErr, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     `!!!wrong`,
		FactoryContractAddress: ethtypes.MustNewAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011603", regErr)
	assert.False(t, tp.initialized.Load())
}

func TestRegisterAndFindAvailableBadAddress(t *testing.T) {
	_, _, tp, regErr, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: `!wrong`,
		FactoryContractAbiJson: `[]`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011606", regErr)
	assert.False(t, tp.initialized.Load())
}

func TestRegisterAndFindAvailableFactoryABI(t *testing.T) {
	_, _, tp, regErr, done := newTestDomain(t, false, &prototk.DomainConfig{
		ConstructorAbiJson:     fakeCoinConstructorABI,
		FactoryContractAddress: ethtypes.MustNewAddress(types.RandHex(20)).String(),
		FactoryContractAbiJson: `!!!wrong`,
		AbiStateSchemasJson: []string{
			fakeCoinStateSchema,
		},
	})
	defer done()
	assert.Regexp(t, "PD011605", regErr)
	assert.False(t, tp.initialized.Load())
}
