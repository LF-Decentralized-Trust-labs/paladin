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
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/stretchr/testify/require"
)

func TestNewDomainPrefixRouterOk(t *testing.T) {
	ctx := context.Background()

	signerFactory := &testInMemorySignerFactory{
		signer: &testMemSigner{
			sign: func(ctx context.Context, algorithm, payloadType string, privateKey, payload []byte) ([]byte, error) {
				return []byte("signed"), nil
			},
			getVerifier: func(ctx context.Context, algorithm, verifierType string, privateKey []byte) (string, error) {
				return "verifier", nil
			},
			getMinimumKeyLen: func(ctx context.Context, algorithm string) (int, error) {
				return 32, nil
			},
		},
	}

	dpr := NewDomainPrefixRouter(map[string]signerapi.InMemorySignerFactory[*signerapi.ConfigNoExt]{
		"test": signerFactory,
	})

	signer, err := dpr.NewSigner(ctx, &signerapi.ConfigNoExt{})
	require.NoError(t, err)
	require.NotNil(t, dpr.GetSigner("test"))

	signed, err := signer.Sign(ctx, "domain:test:anything", "test:example", []byte("key"), []byte("payload"))
	require.NoError(t, err)
	require.Equal(t, "signed", string(signed))

	verifier, err := signer.GetVerifier(ctx, "domain:test:anything", "test:example", []byte("key"))
	require.NoError(t, err)
	require.Equal(t, "verifier", string(verifier))

	keyLen, err := signer.GetMinimumKeyLen(ctx, "domain:test:anything")
	require.NoError(t, err)
	require.Equal(t, 32, keyLen)

}

func TestNewDomainPrefixRouterErrors(t *testing.T) {
	ctx := context.Background()

	signerFactory := &testInMemorySignerFactory{
		signer: &testMemSigner{},
		err:    fmt.Errorf("pop"),
	}

	dpr := NewDomainPrefixRouter(map[string]signerapi.InMemorySignerFactory[*signerapi.ConfigNoExt]{
		"test": signerFactory,
	})

	_, err := dpr.NewSigner(ctx, &signerapi.ConfigNoExt{})
	require.Regexp(t, "pop", err)

	signerFactory.err = nil
	signer, err := dpr.NewSigner(ctx, &signerapi.ConfigNoExt{})
	require.NoError(t, err)

	_, err = signer.Sign(ctx, "edcsa:secp256k1", "test:example", []byte("key"), []byte("payload"))
	require.Regexp(t, "PD020826", err)

	_, err = signer.Sign(ctx, "domain:wrong:anything", "test:example", []byte("key"), []byte("payload"))
	require.Regexp(t, "PD020827", err)

	_, err = signer.GetVerifier(ctx, "domain:wrong:anything", "test:example", []byte("key"))
	require.Regexp(t, "PD020827", err)

	_, err = signer.GetMinimumKeyLen(ctx, "domain:wrong:anything")
	require.Regexp(t, "PD020827", err)

}
