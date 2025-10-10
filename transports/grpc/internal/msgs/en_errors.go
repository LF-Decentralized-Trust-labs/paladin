// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package msgs

import (
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

var registered sync.Once
var pde = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix("PD03", "Paladin GRPC Transport")
	})
	return i18n.PDE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	// Generic PD0300XX
	MsgListenerPortAndAddressRequired       = pde("PD030000", "port and address for listener are required")
	MsgInvalidTransportConfig               = pde("PD030001", "Invalid transport configuration")
	MsgConfIncompatibleWithDirectCertVerify = pde("PD030002", "When directCertVerification is enabled, TLS and clientAuth must be enabled, with no additional CA configuration or insecureSkipHostVerify")
	MsgInvalidSubjectRegexp                 = pde("PD030003", "subjectMatchRegex is invalid")
	MsgVerifierRequiresOneCert              = pde("PD030004", "certificate verifier expected exactly one certificate from peer certs=%d")
	MsgSubjectRegexpMismatch                = pde("PD030005", "subjectMatchRegex did not match the subject in the certificate")
	MsgPeerTransportDetailsInvalid          = pde("PD030006", "published peer transport details for node '%s' are invalid")
	MsgPeerCertificateIssuerInvalid         = pde("PD030007", "peer '%s' did not provide a certificate signed an expected issuer received=%s issuers=%v")
	MsgTLSNegotiationFailed                 = pde("PD030008", "TLS negotiation did not result in a verified peer node name")
	MsgAuthContextNotAvailable              = pde("PD030009", "server failed to retrieve the auth context")
	MsgConnectionToWrongNode                = pde("PD030011", "the TLS identity of the node '%s' does not match the expected node '%s'")
	MsgPEMCertificateInvalid                = pde("PD030012", "invalid PEM encoded x509 certificate")
	MsgInvalidTransportDetails              = pde("PD030014", "Invalid transport details for node '%s'")
	MsgConnectionFailed                     = pde("PD030015", "GRPC connection failed for endpoint '%s'")
	MsgNodeNotActive                        = pde("PD030016", "Send for node that is not active '%s'")
)
