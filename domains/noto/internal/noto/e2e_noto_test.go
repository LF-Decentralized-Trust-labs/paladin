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

package noto

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
)

var (
	toDomain    = "to-domain"
	testbedAddr = "http://127.0.0.1:49600"
	grpcAddr    = "dns:localhost:49601"
	notaryName  = "notary1"
)

func findMethod(contractABI abi.ABI, method string) *abi.Entry {
	for _, entry := range contractABI {
		if entry.Name == method {
			return entry
		}
	}
	return nil
}

func TestNoto(t *testing.T) {
	ctx := context.Background()

	domain, err := New(ctx, grpcAddr)
	assert.NoError(t, err)
	defer domain.Close()

	log.L(ctx).Infof("Listening for gRPC messages on %s", toDomain)
	err = domain.Listen(ctx, toDomain)
	assert.NoError(t, err)

	conf := ffresty.Config{URL: testbedAddr}
	rest := ffresty.NewWithConfig(ctx, conf)
	rpc := rpcbackend.NewRPCClient(rest)

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	log.L(ctx).Infof("Calling testbed_deployBytecode")
	var addressResult string
	rpcerr := rpc.CallRPC(callCtx, &addressResult, "testbed_deployBytecode", notaryName, domain.Factory.ABI, domain.Factory.Bytecode.String(), `{}`)
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Deployed to %s", addressResult)

	log.L(ctx).Infof("Calling testbed_configureInit")
	var boolResult bool
	domainConfig := Config{FactoryAddress: addressResult}
	rpcerr = rpc.CallRPC(callCtx, &boolResult, "testbed_configureInit", "noto", domainConfig)
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	log.L(ctx).Infof("Calling testbed_deploy")
	var deployResult ethtypes.Address0xHex
	rpcerr = rpc.CallRPC(callCtx, &deployResult, "testbed_deploy", "noto", &NotoConstructor{
		Notary: notaryName,
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}

	log.L(ctx).Infof("Calling testbed_invoke")
	transfer := findMethod(domain.Contract.ABI, "transfer")
	assert.NotNil(t, transfer)
	rpcerr = rpc.CallRPC(callCtx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     notaryName,
		To:       types.EthAddress(deployResult),
		Function: *transfer,
		Inputs: types.RawJSON(`{
			"inputs": [],
			"outputs": [],
			"signature": "0x",
			"data": "0x"
		}`),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
}
