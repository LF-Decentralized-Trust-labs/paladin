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

package grpctransport

import (
	"context"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GrpcPlugin struct{}

func (gp *GrpcPlugin) Run(pluginId, targetURL string) {
	ctx := context.Background()

	conn, err := grpc.NewClient(targetURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.L(ctx).Errorf("grpcplugin: error intializing connection to plugin controller, err: %v", err)
		return
	}
	defer conn.Close()

	client := prototk.NewPluginControllerClient(conn)

	stream, err := client.ConnectDomain(ctx)
	err = stream.Send(&prototk.DomainMessage{
		Header: &prototk.Header{
			PluginId:    pluginId,
			MessageId:   uuid.New().String(),
			MessageType: prototk.Header_REGISTER,
		},
	})
	if err != nil {
		log.L(ctx).Errorf("grpcplugin: error intializing streaming conn to plugin-controller: %v", err)
		return
	}

	ctx = stream.Context()
	
}

func (gp *GrpcPlugin) Stop() {

}
