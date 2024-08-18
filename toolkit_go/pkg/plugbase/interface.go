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
package plugbase

import (
	"context"

	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	pb "google.golang.org/protobuf/proto"
)

// Each type of plugin (Domain, Transport etc.) maintains a bi-directional stream of protobuf messages,
// established from the plugin to the plugin-controller.
// Requests and responses can flow in either direction on this stream, and MUST be processed with parallelism
// on both sides.
//
// This interface maps the standardized fields of each of those message types that allows a common
// utility base request/reply functionality to be shared across plugin types.
type PluginMessage[M any] interface {
	Header() *prototk.Header // the common header for all plugin types
	RequestToPlugin() any
	ResponseFromPlugin() any
	RequestFromPlugin() any
	ResponseToPlugin() any
	Message() *M // ref to the proto message (which must be pb.Message)
	ProtoMessage() pb.Message
}

// Implemented by a particular type of plugin to handle:
// - Requests made by paladin down to the domain
// - Notifications that new plugins of that base type have been loaded
//
// Implementations can be synchronous - the plugin base will ensure it's run on appropriate go routine
type PluginImplementation[M any] interface {
	Wrap(*M) PluginMessage[M]
	NewHandler(proxy PluginProxy[M]) PluginHandler[M]
}

type PluginHandler[M any] interface {
	RequestToDomain(ctx context.Context, req PluginMessage[M]) (PluginMessage[M], error)
}

type PluginProxy[M any] interface {
	RequestFromDomain(ctx context.Context, req PluginMessage[M]) (PluginMessage[M], error)
}
