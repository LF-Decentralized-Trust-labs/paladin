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

package pldconf

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
)

const DefaultHTTPPort = 8645
const DefaultWebSocketPort = 8646

var WSDefaults = RPCServerConfigWS{
	ReadBufferSize:  confutil.P("64KB"),
	WriteBufferSize: confutil.P("64KB"),
}

type RPCServerConfigHTTP struct {
	Disabled         bool                 `json:"disabled,omitempty"`
	StaticServers    []StaticServerConfig `json:"staticServers,omitempty"` // Configurations for static file servers handled by the HTTP server (e.g., for serving a UI hosted on the same server as the RPC server)
	HTTPServerConfig `json:",inline"`
}

type RPCServerConfigWS struct {
	Disabled         bool `json:"disabled,omitempty"`
	HTTPServerConfig `json:",inline"`
	ReadBufferSize   *string `json:"readBufferSize"`
	WriteBufferSize  *string `json:"writeBufferSize"`
}

type RPCServerConfig struct {
	HTTP RPCServerConfigHTTP `json:"http,omitempty"`
	WS   RPCServerConfigWS   `json:"ws,omitempty"`
}
