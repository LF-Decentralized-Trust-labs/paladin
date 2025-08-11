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

package pldapi

import "github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"

type PeerInfo struct {
	Name              string             `docstruct:"PeerInfo" json:"name"`
	Stats             PeerStats          `docstruct:"PeerInfo" json:"stats"`
	Activated         pldtypes.Timestamp `docstruct:"PeerInfo" json:"activated"`
	OutboundTransport string             `docstruct:"PeerInfo" json:"outboundTransport,omitempty"`
	Outbound          map[string]any     `docstruct:"PeerInfo" json:"outbound,omitempty"`
	OutboundError     error              `docstruct:"PeerInfo" json:"outboundError,omitempty"`
}

type PeerStats struct {
	SentMsgs            uint64              `docstruct:"PeerStats" json:"sentMsgs"`
	ReceivedMsgs        uint64              `docstruct:"PeerStats" json:"receivedMsgs"`
	SentBytes           uint64              `docstruct:"PeerStats" json:"sentBytes"`
	ReceivedBytes       uint64              `docstruct:"PeerStats" json:"receivedBytes"`
	LastSend            *pldtypes.Timestamp `docstruct:"PeerStats" json:"lastSend"`
	LastReceive         *pldtypes.Timestamp `docstruct:"PeerStats" json:"lastReceive"`
	ReliableHighestSent uint64              `docstruct:"PeerStats" json:"reliableHighestSent"`
	ReliableAckBase     uint64              `docstruct:"PeerStats" json:"reliableAckBase"`
}
