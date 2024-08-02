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

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
)

type testExtension struct {
	keyStore func(keystoreType string) (store KeyStore, err error)
}

func (te *testExtension) KeyStore(keystoreType string) (store KeyStore, err error) {
	return te.keyStore(keystoreType)
}

type testKeyStoreAll struct {
	findOrCreateLoadableKey   func(ctx context.Context, req *proto.ResolveKeyRequest, newKeyMaterial func() ([]byte, error)) (keyMaterial []byte, keyHandle string, err error)
	loadKeyMaterial           func(ctx context.Context, keyHandle string) ([]byte, error)
	findOrCreateKey_secp256k1 func(ctx context.Context, req *proto.ResolveKeyRequest) (addr *ethtypes.Address0xHex, keyHandle string, err error)
	sign_secp256k1            func(ctx context.Context, keyHandle string, payload []byte) (*secp256k1.SignatureData, error)
	listKeys                  func(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error)
}

func (tk *testKeyStoreAll) FindOrCreateLoadableKey(ctx context.Context, req *proto.ResolveKeyRequest, newKeyMaterial func() ([]byte, error)) (keyMaterial []byte, keyHandle string, err error) {
	return tk.findOrCreateLoadableKey(ctx, req, newKeyMaterial)
}

func (tk *testKeyStoreAll) LoadKeyMaterial(ctx context.Context, keyHandle string) ([]byte, error) {
	return tk.loadKeyMaterial(ctx, keyHandle)
}

func (tk *testKeyStoreAll) FindOrCreateKey_secp256k1(ctx context.Context, req *proto.ResolveKeyRequest) (addr *ethtypes.Address0xHex, keyHandle string, err error) {
	return tk.findOrCreateKey_secp256k1(ctx, req)
}

func (tk *testKeyStoreAll) Sign_secp256k1(ctx context.Context, keyHandle string, payload []byte) (*secp256k1.SignatureData, error) {
	return tk.sign_secp256k1(ctx, keyHandle, payload)
}

func (tk *testKeyStoreAll) ListKeys(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error) {
	return tk.listKeys(ctx, req)
}

func TestExtensionInitFail(t *testing.T) {

	te := &testExtension{
		keyStore: func(keystoreType string) (store KeyStore, err error) {
			assert.Equal(t, "ext-store", keystoreType)
			return nil, fmt.Errorf("pop")
		},
	}

	_, err := NewSigningModule(context.Background(), &Config{
		KeyStore: StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.Regexp(t, "pop", err)

}

func TestKeystoreUnknown(t *testing.T) {

	te := &testExtension{
		keyStore: func(keystoreType string) (store KeyStore, err error) { return nil, nil },
	}
	_, err := NewSigningModule(context.Background(), &Config{
		KeyStore: StoreConfig{
			Type: "unknown",
		},
	}, te)
	assert.Regexp(t, "PD011407", err)

}

func TestExtensionKeyStoreListOK(t *testing.T) {

	testRes := &proto.ListKeysResponse{
		Items: []*proto.ListKeyHandleEntry{
			{
				TopLevelName: "key 23456",
				KeyHandle:    "key23456",
				Identifiers: []*proto.PublicKeyIdentifier{
					{Algorithm: Algorithm_ECDSA_SECP256K1, Identifier: "0x93e5a15ce57564278575ff7182b5b3746251e781"},
				},
			},
		},
	}
	tk := &testKeyStoreAll{
		listKeys: func(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error) {
			assert.Equal(t, int32(10), req.Limit)
			assert.Equal(t, "key12345", req.AfterKeyHandle)
			return testRes, nil
		},
	}
	te := &testExtension{
		keyStore: func(keystoreType string) (store KeyStore, err error) {
			assert.Equal(t, "ext-store", keystoreType)
			return tk, nil
		},
	}

	sm, err := NewSigningModule(context.Background(), &Config{
		KeyStore: StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.NoError(t, err)

	res, err := sm.List(context.Background(), &proto.ListKeysRequest{
		Limit:          10,
		AfterKeyHandle: "key12345",
	})
	assert.NoError(t, err)
	assert.Equal(t, testRes, res)

}

func TestExtensionKeyStoreListFail(t *testing.T) {

	tk := &testKeyStoreAll{
		listKeys: func(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error) {
			return nil, fmt.Errorf("pop")
		},
	}
	te := &testExtension{
		keyStore: func(keystoreType string) (store KeyStore, err error) {
			assert.Equal(t, "ext-store", keystoreType)
			return tk, nil
		},
	}

	sm, err := NewSigningModule(context.Background(), &Config{
		KeyStore: StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.NoError(t, err)

	_, err = sm.List(context.Background(), &proto.ListKeysRequest{
		Limit:          10,
		AfterKeyHandle: "key12345",
	})
	assert.Regexp(t, "pop", err)

}
