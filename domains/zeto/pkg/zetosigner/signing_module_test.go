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

package zetosigner

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/signer"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newZetoSigningModule(t *testing.T) (context.Context, signer.SigningModule, func()) {
	ctx := context.Background()
	sm, err := signer.NewSigningModule(ctx, &SnarkProverConfig{
		ConfigNoExt: signerapi.ConfigNoExt{
			KeyDerivation: pldconf.KeyDerivationConfig{
				Type: pldconf.KeyDerivationTypeBIP32,
			},
			KeyStore: pldconf.KeyStoreConfig{
				Type: pldconf.KeyStoreTypeStatic,
				Static: pldconf.StaticKeyStoreConfig{
					Keys: map[string]pldconf.StaticKeyEntryConfig{
						"seed": {
							Encoding: "hex",
							Inline:   tktypes.RandHex(32),
						},
					},
				},
			},
		},
	}, &signerapi.Extensions[*SnarkProverConfig]{
		InMemorySignerFactories: map[string]signerapi.InMemorySignerFactory[*SnarkProverConfig]{
			"domain": NewZetoOnlyDomainRouter(),
		},
	})
	require.NoError(t, err)

	return ctx, sm, func() { sm.Close() }
}

func TestZKPSigningModuleKeyResolution(t *testing.T) {
	ctx, sm, done := newZetoSigningModule(t)
	defer done()

	resp1, err := sm.Resolve(ctx, &signerapi.ResolveKeyRequest{
		RequiredIdentifiers: []*signerapi.PublicKeyIdentifierType{
			{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
			{Algorithm: AlgoDomainZetoSnarkBJJ("zeto"), VerifierType: IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
		},
		Name: "blueKey",
		Path: []*signerapi.ResolveKeyPathSegment{
			{Name: "alice"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, len(resp1.Identifiers))
}
