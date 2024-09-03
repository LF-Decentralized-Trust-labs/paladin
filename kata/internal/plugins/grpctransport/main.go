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

	"github.com/kaleido-io/paladin/toolkit/pkg/log"

	grpctransport "github.com/kaleido-io/paladin/kata/internal/plugins/grpctransport/plugin"
)

var transport *grpctransport.GRPCTransport
var ctx context.Context

func Run(pluginID, connString string) {
	ctx = context.Background()
	log.L(ctx).Info("grpctransport.Init")

	if transport != nil {
		log.L(ctx).Errorf("plugin is already initialized")
		return
	}

	transport, err := grpctransport.NewGRPCTransport(pluginID, connString)
	if err != nil {
		log.L(ctx).Errorf("error getting new transport")
	}

	err = transport.Init()
	if err != nil {
		log.L(ctx).Errorf("error initting transport")
	}

	transport.ServerWg.Wait()
}

func Stop() {
	if transport == nil {
		log.L(ctx).Errorf("stop called on an uninitialized plugin")
		return
	}

	err := transport.Shutdown()
	if err != nil {
		log.L(ctx).Errorf("stop called on an uninitialized plugin")
		return
	}
}
