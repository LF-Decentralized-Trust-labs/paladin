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

package keymanager

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var signingModuleID uuid.UUID

type testPlugin struct {
	plugintk.SigningModuleAPIBase
	initialized atomic.Bool
	sm          *signingModule
}

func (tp *testPlugin) Initialized() {
	tp.initialized.Store(true)
}

func newTestPlugin(signingModuleFuncs *plugintk.SigningModuleAPIFunctions) *testPlugin {
	return &testPlugin{
		SigningModuleAPIBase: plugintk.SigningModuleAPIBase{
			Functions: signingModuleFuncs,
		},
	}
}

func newTestSigningModule(t *testing.T, realDB bool, extraSetup ...func(mc *mockComponents, conf *pldconf.KeyManagerConfig)) (context.Context, *keyManager, *testPlugin, *mockComponents, func()) {
	conf := &pldconf.KeyManagerConfig{
		SigningModules: map[string]*pldconf.SigningModuleConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
	}

	ctx, km, mc, done := newTestKeyManager(t, realDB, conf, nil)

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.SigningModuleAPIFunctions{
		ConfigureSigningModule: func(ctx context.Context, csmr *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
			assert.Equal(t, "test1", csmr.Name)
			assert.JSONEq(t, `{"some":"conf"}`, csmr.ConfigJson)
			return &prototk.ConfigureSigningModuleResponse{}, nil
		},
	}

	registerTestSigningModule(t, km, tp)
	return ctx, km, tp, mc, done
}

func registerTestSigningModule(t *testing.T, km *keyManager, tp *testPlugin) {
	signingModuleID = uuid.New()
	_, err := km.SigningModuleRegistered("test1", signingModuleID, tp)
	require.NoError(t, err)

	sm := km.signingModulesByName["test1"]
	assert.NotNil(t, sm)
	tp.sm = sm
	tp.sm.initRetry.UTSetMaxAttempts(1)
	<-tp.sm.initDone
}

func TestDoubleRegisterReplaces(t *testing.T) {
	_, km, tp0, _, done := newTestSigningModule(t, false)
	defer done()
	assert.Nil(t, tp0.sm.initError.Load())
	assert.True(t, tp0.initialized.Load())

	// Register again
	tp1 := newTestPlugin(nil)
	tp1.Functions = tp0.Functions
	registerTestSigningModule(t, km, tp1)
	assert.Nil(t, tp1.sm.initError.Load())
	assert.True(t, tp1.initialized.Load())

	// Check we get the second from all the maps
	byName := km.signingModulesByName[tp1.sm.name]
	assert.Same(t, tp1.sm, byName)
	byUUID := km.signingModulesByID[tp1.sm.id]
	assert.Same(t, tp1.sm, byUUID)
}

func TestHandleResolveOk(t *testing.T) {
	ctx, _, tp, _, done := newTestSigningModule(t, false)
	defer done()

	resolveKeyReq := &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "key1",
	}

	tp.Functions.ResolveKey = func(ctx context.Context, rkr *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error) {
		assert.Equal(t, "key1", rkr.Name)
		return &prototk.ResolveKeyResponse{
			KeyHandle: "key_handle_1",
			Identifiers: []*prototk.PublicKeyIdentifier{
				{
					Algorithm:    rkr.RequiredIdentifiers[0].Algorithm,
					VerifierType: rkr.RequiredIdentifiers[0].VerifierType,
					Verifier:     "0x98a356e0814382587d42b62bd97871ee59d10b69",
				},
			},
		}, nil
	}

	res, err := tp.sm.Resolve(ctx, resolveKeyReq)
	require.NoError(t, err)
	assert.Equal(t, "key_handle_1", res.KeyHandle)
}

func TestHandleResolveError(t *testing.T) {
	ctx, _, tp, _, done := newTestSigningModule(t, false)
	defer done()

	resolveKeyReq := &prototk.ResolveKeyRequest{
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{{Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS}},
		Name:                "key1",
	}

	tp.Functions.ResolveKey = func(ctx context.Context, rkr *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error) {
		assert.Equal(t, "key1", rkr.Name)
		return nil, fmt.Errorf("pop")
	}

	_, err := tp.sm.Resolve(ctx, resolveKeyReq)
	require.Regexp(t, "pop", err)
}

func TestHandleSignOk(t *testing.T) {
	ctx, _, tp, _, done := newTestSigningModule(t, false)
	defer done()

	signWithKeyRequest := &prototk.SignWithKeyRequest{
		KeyHandle:   "key_handle_1",
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
	}

	tp.Functions.Sign = func(ctx context.Context, swkr *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
		assert.Equal(t, "key_handle_1", swkr.KeyHandle)
		return &prototk.SignWithKeyResponse{
			Payload: ([]byte)("signed data"),
		}, nil
	}

	res, err := tp.sm.Sign(ctx, signWithKeyRequest)
	require.NoError(t, err)
	assert.Equal(t, ([]byte)("signed data"), res.Payload)
}

func TestHandleSignError(t *testing.T) {
	ctx, _, tp, _, done := newTestSigningModule(t, false)
	defer done()

	signWithKeyRequest := &prototk.SignWithKeyRequest{
		KeyHandle:   "key_handle_1",
		Algorithm:   algorithms.ECDSA_SECP256K1,
		PayloadType: signpayloads.OPAQUE_TO_RSV,
		Payload:     ([]byte)("some data"),
	}

	tp.Functions.Sign = func(ctx context.Context, swkr *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
		assert.Equal(t, "key_handle_1", swkr.KeyHandle)
		return nil, fmt.Errorf("pop")
	}

	_, err := tp.sm.Sign(ctx, signWithKeyRequest)
	require.Regexp(t, "pop", err)
}

func TestHandleListKeysOk(t *testing.T) {
	ctx, _, tp, _, done := newTestSigningModule(t, false)
	defer done()

	listKeysRequest := &prototk.ListKeysRequest{
		Continue: "key12345",
		Limit:    int32(10),
	}

	tp.Functions.ListKeys = func(ctx context.Context, lkr *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error) {
		assert.Equal(t, int32(10), lkr.Limit)
		return &prototk.ListKeysResponse{
			Items: []*prototk.ListKeyEntry{
				{
					Name:      "key 23456",
					KeyHandle: "key23456",
					Identifiers: []*prototk.PublicKeyIdentifier{
						{Algorithm: algorithms.ECDSA_SECP256K1, Verifier: "0x93e5a15ce57564278575ff7182b5b3746251e781"},
					},
				},
			},
			Next: "key23456",
		}, nil
	}

	res, err := tp.sm.List(ctx, listKeysRequest)
	require.NoError(t, err)
	assert.Equal(t, 1, len(res.Items))
}

func TestHandleListKeysError(t *testing.T) {
	ctx, _, tp, _, done := newTestSigningModule(t, false)
	defer done()

	listKeysRequest := &prototk.ListKeysRequest{
		Continue: "key12345",
		Limit:    int32(10),
	}

	tp.Functions.ListKeys = func(ctx context.Context, lkr *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error) {
		assert.Equal(t, int32(10), lkr.Limit)
		return nil, fmt.Errorf("pop")
	}

	_, err := tp.sm.List(ctx, listKeysRequest)
	require.Regexp(t, "pop", err)
}

func TestHandleAddInMemorySignerOk(t *testing.T) {
	_, _, tp, _, done := newTestSigningModule(t, false)
	defer done()

	s := &testSigner{}

	tp.sm.AddInMemorySigner("test-signer", s)
}

func TestHandleCloseOk(t *testing.T) {
	_, _, tp, _, done := newTestSigningModule(t, false)
	defer done()

	tp.Functions.Close = func(ctx context.Context, cr *prototk.CloseRequest) (*prototk.CloseResponse, error) {
		require.NotNil(t, cr)
		return &prototk.CloseResponse{}, nil
	}

	tp.sm.Close()
}
