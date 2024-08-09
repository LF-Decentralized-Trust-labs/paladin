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
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/domains/noto/internal/noto"
)

var (
	toDomain    = "to-domain"
	testbedAddr = "http://127.0.0.1:49603"
	grpcAddr    = "unix:/tmp/testbed.paladin.1542386773.sock"
)

func runTest(ctx context.Context) error {
	domain, err := noto.Start(ctx, grpcAddr)
	if err != nil {
		return err
	}
	defer domain.Close()

	log.L(ctx).Infof("Listening for gRPC messages on %s", toDomain)
	err = domain.Listen(ctx, toDomain)
	if err != nil {
		return err
	}

	conf := ffresty.Config{URL: testbedAddr}
	rest := ffresty.NewWithConfig(ctx, conf)
	rpc := rpcbackend.NewRPCClient(rest)

	log.L(ctx).Infof("Calling testbed_configureInit")
	var result bool
	rpcerr := rpc.CallRPC(ctx, &result, "testbed_configureInit", "noto", `{}`)
	if rpcerr != nil {
		return fmt.Errorf("fail to call JSON RPC: %v", rpcerr)
	}
	return nil
}

func main() {
	ctx := context.Background()
	if err := runTest(ctx); err != nil {
		log.L(ctx).Fatalf("%s", err)
	}
}
