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
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
)

type Config struct {
	// optional remote hostname to return in local transport details
	ExternalHostname *string `json:"externalHostname"`
	// TLS configuration details
	TLS pldconf.TLSConfig `json:"tls"`
	// address to listen on
	Address *string `json:"address"`
	// port to listen on
	Port *int `json:"port"`
	// If true (default) a network can be built by publishing self-signed certs to a registry without a common CA.
	// This disables the default certificate verification chain, and instead performs a direct comparison
	// of the certificate against the registered certificate for the extracted node name.
	DirectCertVerification *bool `json:"directCertVerification,omitempty"`
	// By default directCertVerification will expect the CN of the subject to be the exact registered node name.
	// Optionally certSubjectMatcher can supply a regexp containing a SINGLE CAPTURE GROUP that can be used to extract the name from the subject string
	CertSubjectMatcher *string `json:"certSubjectMatcher,omitempty"`
}

var ConfigDefaults = &Config{
	Address:                confutil.P("0.0.0.0"), // public connectivity
	DirectCertVerification: confutil.P(true),      // with self-signed certificates
}

// This is the JSON structure that any node in the network must share to be connectable
// by this plugin. We require the local node's registered information to be available at configuration
// time otherwise we cannot start up.
type PublishedTransportDetails struct {
	Endpoint string `json:"endpoint"` // a GRPC target string that other nodes can use to connect to this node
	// A node specific PEM certificate/certificate-set to use to validate the certificate provided by a node
	// - used in direct certificate validation mode only
	// - can be the certificate itself for self-signed
	// - must be the direct parent (not the root of a chain - for that use normal CA verification)
	Issuers string `json:"issuers,omitempty"`
}

type PeerInfo struct {
	Endpoint string `json:"endpoint"`
}
