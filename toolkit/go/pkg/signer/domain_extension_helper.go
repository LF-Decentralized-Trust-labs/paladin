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
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
)

// As well as running inside of Paladin, domains with signing tech (like Zeto) can provide their signing
// engine runtime packaged along with this signing code - to run remote from Paladin in an even
// more secure network segment close to a key store technology (such as HSM)
//
// In this case they still need to resolve "domain:[name]" to their particular signer inside that runtime.
//
// This utility provides a simple wrapper with that common logic.
//
// It's also helpful for unit testing the signer in isolation from a full Paladin build (see Zeto for an example)
//
// The config passed to this router needs to be the combined configuration object that
// contains all the different features the domains require.
type DomainPrefixRouter[C signerapi.ExtensibleConfig] interface {
	signerapi.InMemorySignerFactory[C]
	GetSigner(domain string) signerapi.InMemorySigner // nil if not initialized, or not in map
}

type domainPrefixRouter[C signerapi.ExtensibleConfig] struct {
	domainFactories map[string]signerapi.InMemorySignerFactory[C]
	domainSigners   map[string]signerapi.InMemorySigner
}

func NewDomainPrefixRouter[C signerapi.ExtensibleConfig](domainFactories map[string]signerapi.InMemorySignerFactory[C]) DomainPrefixRouter[C] {
	return &domainPrefixRouter[C]{
		domainFactories: domainFactories,
		domainSigners:   make(map[string]signerapi.InMemorySigner),
	}
}

func (d *domainPrefixRouter[C]) GetSigner(domain string) signerapi.InMemorySigner {
	return d.domainSigners[domain]
}

func (d *domainPrefixRouter[C]) NewSigner(ctx context.Context, conf C) (_ signerapi.InMemorySigner, err error) {
	for domain, dsf := range d.domainFactories {
		if d.domainSigners[domain], err = dsf.NewSigner(ctx, conf); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (d *domainPrefixRouter[C]) getDomainSignerForAlgorithm(ctx context.Context, algorithm string) (signerapi.InMemorySigner, error) {
	algoNoDomain, ok := strings.CutPrefix(strings.ToLower(algorithm), "domain:")
	if !ok {
		return nil, i18n.NewError(ctx, pldmsgs.MsgSigningInvalidDomainAlgorithmNoPrefix, algorithm)
	}
	domain := strings.SplitN(algoNoDomain, ":", 2)[0]
	s, ok := d.domainSigners[domain]
	if !ok {
		return nil, i18n.NewError(ctx, pldmsgs.MsgSigningNoDomainRegisteredWithModule, domain)
	}
	return s, nil
}

func (d *domainPrefixRouter[C]) GetMinimumKeyLen(ctx context.Context, algorithm string) (keyLen int, err error) {
	s, err := d.getDomainSignerForAlgorithm(ctx, algorithm)
	if err == nil {
		keyLen, err = s.GetMinimumKeyLen(ctx, algorithm)
	}
	return
}

// GetVerifier implements signerapi.InMemorySigner.
func (d *domainPrefixRouter[C]) GetVerifier(ctx context.Context, algorithm string, verifierType string, privateKey []byte) (verifier string, err error) {
	s, err := d.getDomainSignerForAlgorithm(ctx, algorithm)
	if err == nil {
		verifier, err = s.GetVerifier(ctx, algorithm, verifierType, privateKey)
	}
	return
}

// Sign implements signerapi.InMemorySigner.
func (d *domainPrefixRouter[C]) Sign(ctx context.Context, algorithm string, payloadType string, privateKey []byte, payload []byte) (signed []byte, err error) {
	s, err := d.getDomainSignerForAlgorithm(ctx, algorithm)
	if err == nil {
		signed, err = s.Sign(ctx, algorithm, payloadType, privateKey, payload)
	}
	return
}
