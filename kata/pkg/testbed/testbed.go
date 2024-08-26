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

	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
)

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
