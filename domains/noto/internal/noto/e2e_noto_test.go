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
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/stretchr/testify/assert"
)

// TODO: fix things that should not be hard-coded
var (
	toDomain    = "to-domain"
	testbedAddr = "http://127.0.0.1:49603"
	grpcAddr    = "unix:/tmp/testbed.paladin.1542386773.sock"
	account1    = "0x9180ff8fa5c502b9bfe5dfeaf477e157dbfaba5c"
)

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

	var addressResult string
	var boolResult bool
	var objResult interface{}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	log.L(ctx).Infof("Calling testbed_deployBytecode")
	rpcerr := rpc.CallRPC(callCtx, &addressResult, "testbed_deployBytecode", account1, domain.Factory.Bytecode.String())
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Deployed to %s", addressResult)

	log.L(ctx).Infof("Calling testbed_configureInit")
	domainConfig := Config{FactoryAddress: addressResult}
	rpcerr = rpc.CallRPC(callCtx, &boolResult, "testbed_configureInit", "noto", domainConfig)
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}

	log.L(ctx).Infof("Calling testbed_deploy")
	rpcerr = rpc.CallRPC(callCtx, &objResult, "testbed_deploy", "noto", &NotoConstructor{
		Notary: account1,
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
}
