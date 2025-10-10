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
package plugintk

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"google.golang.org/protobuf/encoding/protojson"
	pb "google.golang.org/protobuf/proto"
)

// The external interface of a plugin is run a run function with two strings, which should
// not exit in any other case apart from being stopped. This includes retrying connection
// to the gRPC endpoint.
type Plugin interface {
	Run(pluginID, connString string)
	Stop()
}

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
	PluginMessageWrapper[M]
	NewHandler(proxy PluginProxy[M]) PluginHandler[M]
}

type PluginMessageWrapper[M any] interface {
	Wrap(*M) PluginMessage[M]
}

type PluginHandler[M any] interface {
	RequestToPlugin(ctx context.Context, req PluginMessage[M]) (PluginMessage[M], error)
}

type PluginProxy[M any] interface {
	RequestFromPlugin(ctx context.Context, req PluginMessage[M]) (PluginMessage[M], error)
}

func PluginMessageToJSON[M any](msg PluginMessage[M]) (s string) {
	b, _ := protojson.Marshal(msg.ProtoMessage())
	if b != nil {
		s = string(b)
	}
	return
}

// Maps the response payload to the requested type (avoids boilerplate in plugin types)
func responseToPluginAs[M, ResType, T any](ctx context.Context, msg PluginMessage[M], err error, unwrap func(*ResType) *T) (*T, error) {
	if err != nil {
		return nil, err
	}
	res := msg.ResponseToPlugin()
	iRes, ok := res.(*ResType)
	if !ok {
		return nil, i18n.NewError(ctx, pldmsgs.MsgPluginUnexpectedResponse, iRes, new(ResType))
	}
	return unwrap(iRes), nil
}

func callPluginImpl[IN, OUT any](ctx context.Context, in *IN, fn func(context.Context, *IN) (*OUT, error)) (*OUT, error) {
	if fn == nil {
		return nil, i18n.NewError(ctx, pldmsgs.MsgPluginUnimplementedRequest, new(IN))
	}
	return fn(ctx, in)
}
