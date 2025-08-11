/*
 * Copyright © 2025 Kaleido, Inc.
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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newZetoSigningModule(t *testing.T) (context.Context, signer.SigningModule, func()) {
	ctx := context.Background()
	sm, err := signer.NewSigningModule(ctx, &zetosignerapi.SnarkProverConfig{
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
							Inline:   pldtypes.RandHex(32),
						},
					},
				},
			},
		},
	}, &signerapi.Extensions[*zetosignerapi.SnarkProverConfig]{
		InMemorySignerFactories: map[string]signerapi.InMemorySignerFactory[*zetosignerapi.SnarkProverConfig]{
			"domain": NewZetoOnlyDomainRouter(),
		},
	})
	require.NoError(t, err)

	return ctx, sm, func() { sm.Close() }
}

func TestZKPSigningModuleKeyResolution(t *testing.T) {
	ctx, sm, done := newZetoSigningModule(t)
	defer done()

	resp1, err := sm.Resolve(ctx, &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{
			{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
			{Algorithm: zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X},
		},
		Name: "blueKey",
		Path: []*prototk.ResolveKeyPathSegment{
			{Name: "alice"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, len(resp1.Identifiers))
}
