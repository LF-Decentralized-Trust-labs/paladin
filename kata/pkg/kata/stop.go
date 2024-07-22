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

package kata

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/log"
)

func Stop(ctx context.Context, socketAddress string) {
	log.L(ctx).Infof("Stop: %s", socketAddress)
	if commsBus != nil {
		err := commsBus.GRPCServer().Stop(ctx)
		if err != nil {
			log.L(ctx).Errorf("Failed to stop GRPC server: %s", err)
		}
	}
}
