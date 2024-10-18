// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transportmgr

import (
	"context"

	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
)

func (tm *transportManager) RPCModule() *rpcserver.RPCModule {
	return tm.rpcModule
}

func (tm *transportManager) initRPC() {
	tm.rpcModule = rpcserver.NewRPCModule("transport").
		Add("transport_nodeName", tm.rpcNodeName()).
		Add("transport_localTransports", tm.rpcLocalTransports()).
		Add("transport_localTransportDetails", tm.rpcLocalTransportDetails())
}

func (tm *transportManager) rpcNodeName() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context,
	) (string, error) {
		return tm.localNodeName, nil
	})
}

func (tm *transportManager) rpcLocalTransports() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context,
	) ([]string, error) {
		return tm.getTransportNames(), nil
	})
}

func (tm *transportManager) rpcLocalTransportDetails() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		transportName string,
	) (string, error) {
		return tm.getLocalTransportDetails(ctx, transportName)
	})
}
