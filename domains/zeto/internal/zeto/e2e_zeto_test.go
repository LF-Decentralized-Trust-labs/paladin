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

package zeto

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
)

var (
	toDomain       = "to-domain"
	testbedAddr    = "http://localhost:49610"
	grpcAddr       = "dns:localhost:49611"
	notaryName     = "notary"
	recipient1Name = "recipient1"
	recipient2Name = "recipient2"
)

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	assert.NoError(t, err)
	return result
}

func newTestDomain(t *testing.T) (context.Context, context.CancelFunc, *Zeto, rpcbackend.Backend) {
	ctx := context.Background()
	domain, err := New(ctx, grpcAddr)
	assert.NoError(t, err)

	log.L(ctx).Infof("Listening for gRPC messages on %s", toDomain)
	err = domain.Listen(ctx, toDomain)
	assert.NoError(t, err)

	conf := ffresty.Config{URL: testbedAddr}
	rest := ffresty.NewWithConfig(ctx, conf)
	rpc := rpcbackend.NewRPCClient(rest)

	callCtx, cancelCtx := context.WithTimeout(ctx, 10*time.Second)
	cancel := func() {
		domain.Close()
		cancelCtx()
	}
	return callCtx, cancel, domain, rpc
}

func TestZeto(t *testing.T) {
	log.L(context.Background()).Infof("TestZeto")
	ctx, cancel, _, rpc := newTestDomain(t)
	defer cancel()

	domainName := "zeto_" + types.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)
	factory := loadBuild(zetoFactoryJSON)

	log.L(ctx).Infof("Deploying Zeto factory")
	var factoryAddress string
	rpcerr := rpc.CallRPC(ctx, &factoryAddress, "testbed_deployBytecode",
		notaryName, factory.ABI, factory.Bytecode.String(), `{}`)
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Zeto factory deployed to %s", factoryAddress)

	log.L(ctx).Infof("Configuring Zeto domain")
	var boolResult bool
	domainConfig := Config{
		FactoryAddress: factoryAddress,
	}
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_configureInit",
		domainName, domainConfig)
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	log.L(ctx).Infof("Deploying an instance of Zeto")
	var zetoAddress ethtypes.Address0xHex
	rpcerr = rpc.CallRPC(ctx, &zetoAddress, "testbed_deploy",
		domainName, &ZetoConstructorParams{Notary: notaryName})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress)
}
