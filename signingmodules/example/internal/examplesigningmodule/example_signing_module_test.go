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

package examplesigningmodule

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCallbacks struct{}

func TestPluginLifecycle(t *testing.T) {
	pb := NewPlugin(context.Background())
	assert.NotNil(t, pb)
}

func TestBadConfigJSON(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{!!!!`,
	})
	assert.Regexp(t, "PD070001", err)
}

func TestMissingConfigJSON(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"foo":{"bar": "baz"}}`,
	})
	assert.Regexp(t, "PD070001", err)
}

func TestInvalidConfigJSON(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"signer":{"keyDerivation": { "type": "invalid_type"}, "keyStore": { "type": "static", "static": { "keys": { "seed": { "encoding": "none", "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"}}}}}}`,
	})
	assert.Regexp(t, "PD070001", err)
}

func TestGoodConfigJSON(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	res, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"signer":{"keyDerivation": { "type": "bip32"}, "keyStore": { "type": "static", "static": { "keys": { "seed": { "encoding": "none", "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"}}}}}}`,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestResolveKeyOk(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"signer":{"keyDerivation": { "type": "bip32"}, "keyStore": { "type": "static", "static": { "keys": { "seed": { "encoding": "none", "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"}}}}}}`,
	})
	require.NoError(t, err)

	res, err := signingModule.ResolveKey(signingModule.bgCtx, &prototk.ResolveKeyRequest{
		Name: "testKey",
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{
			{
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "m/44'/60'/0'", res.KeyHandle)
	assert.Equal(t, "0x1a6c36b04874844e3a11b81f28e08802d086c4df", res.Identifiers[0].Verifier)
}

func TestResolveKeyError(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"signer":{"keyDerivation": { "type": "bip32"}, "keyStore": { "type": "static", "static": { "keys": { "seed": { "encoding": "none", "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"}}}}}}`,
	})
	require.NoError(t, err)

	_, err = signingModule.ResolveKey(signingModule.bgCtx, &prototk.ResolveKeyRequest{
		Name: "testKey",
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{
			{
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: "invalid_verifier",
			},
		},
	})
	require.Regexp(t, "PD020823", err)
}

func TestSignOk(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"signer":{"keyDerivation": { "type": "bip32"}, "keyStore": { "type": "static", "static": { "keys": { "seed": { "encoding": "none", "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"}}}}}}`,
	})
	require.NoError(t, err)

	res, err := signingModule.Sign(signingModule.bgCtx, &prototk.SignWithKeyRequest{
		KeyHandle:   "m/44'/60'/0'",
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Payload)
}

func TestSignError(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"signer":{"keyDerivation": { "type": "bip32"}, "keyStore": { "type": "static", "static": { "keys": { "seed": { "encoding": "none", "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"}}}}}}`,
	})
	require.NoError(t, err)

	_, err = signingModule.Sign(signingModule.bgCtx, &prototk.SignWithKeyRequest{
		KeyHandle:   "m/44'/60'/0'",
		Algorithm:   "invalid_algorithm",
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
	})
	require.Regexp(t, "PD020810", err)
}

func TestListError(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"signer":{"keyDerivation": { "type": "bip32"}, "keyStore": { "type": "static", "static": { "keys": { "seed": { "encoding": "none", "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"}}}}}}`,
	})
	require.NoError(t, err)

	_, err = signingModule.ListKeys(signingModule.bgCtx, &prototk.ListKeysRequest{})
	require.Regexp(t, "PD020815", err)
}

func TestCloseOk(t *testing.T) {
	callbacks := &testCallbacks{}
	signingModule := NewKeyManagerSigningModule(callbacks).(*exampleSigningModule)
	_, err := signingModule.ConfigureSigningModule(signingModule.bgCtx, &prototk.ConfigureSigningModuleRequest{
		Name:       "test",
		ConfigJson: `{"signer":{"keyDerivation": { "type": "bip32"}, "keyStore": { "type": "static", "static": { "keys": { "seed": { "encoding": "none", "inline": "field audit weird now route order gentle magnet plastic girl tree lake before super useful unit credit atom person crystal hair drama hole dove"}}}}}}`,
	})
	require.NoError(t, err)

	_, err = signingModule.Close(signingModule.bgCtx, &prototk.CloseRequest{})
	require.NoError(t, err)
}
