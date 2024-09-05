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

/*
Some unit test could become very complex if they to mock the comms bus.
This utility function can be used to run an actual instance of the comms bus including its gRPC service.
*/
package util

import (
	"context"
	"os"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/commsbus"
	"github.com/stretchr/testify/require"
)

func NewCommsBusForTesting(ctx context.Context, t *testing.T) commsbus.CommsBus {

	file, err := os.CreateTemp("", "paladin.sock")
	require.NoError(t, err)
	socketAddress := file.Name()
	os.Remove(file.Name())
	commsBus, err := commsbus.NewCommsBus(ctx, &commsbus.Config{
		GRPC: commsbus.GRPCConfig{
			SocketAddress: &socketAddress,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, commsBus)
	require.NotNil(t, commsBus.Broker())
	return commsBus
}
