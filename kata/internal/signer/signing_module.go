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

package signer

import (
	"context"

	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

// SigningModule provides functions for the protobuf request/reply functions from the proto interface defined
// in signing_module.
// This module can be wrapped and loaded into the core Paladin runtime as an embedded module called directly
// on the comms bus, or wrapped in a remote process connected over gRPC.
type SigningModule interface {
	Resolve(ctx context.Context, req *proto.ResolveKeyRequest) (res proto.ResolveKeyResponse, err error)
	Sign(ctx context.Context, req *proto.SignRequest) (res proto.SignResponse, err error)
}

type signingModule struct {
}
