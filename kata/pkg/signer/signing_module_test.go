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
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/stretchr/testify/assert"
)

type testExtension struct {
	keyStore func(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error)
}

func (te *testExtension) KeyStore(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) {
	return te.keyStore(ctx, config)
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
		keyStore: func(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) {
			assert.Equal(t, "ext-store", config.Type)
			return nil, fmt.Errorf("pop")
		},
	}

	_, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.Regexp(t, "pop", err)

}

func TestKeystoreTypeUnknown(t *testing.T) {

	te := &testExtension{
		keyStore: func(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) { return nil, nil },
	}
	_, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: "unknown",
		},
	}, te)
	assert.Regexp(t, "PD011407", err)

}

func TestKeyDerivationTypeUnknown(t *testing.T) {

	ctx := context.Background()
	_, err := NewSigningModule(ctx, &api.Config{
		KeyDerivation: api.KeyDerivationConfig{
			Type: "unknown",
		},
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeStatic,
		},
	})
	assert.Regexp(t, "PD011419", err)

}

func TestExtensionKeyStoreListOK(t *testing.T) {

	testRes := &proto.ListKeysResponse{
		Items: []*proto.ListKeyEntry{
			{
				Name:      "key 23456",
				KeyHandle: "key23456",
				Identifiers: []*proto.PublicKeyIdentifier{
					{Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES, Identifier: "0x93e5a15ce57564278575ff7182b5b3746251e781"},
				},
			},
		},
		Next: "key12345",
	}
	tk := &testKeyStoreAll{
		listKeys: func(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error) {
			assert.Equal(t, int32(10), req.Limit)
			assert.Equal(t, "key12345", req.Continue)
			return testRes, nil
		},
	}
	te := &testExtension{
		keyStore: func(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) {
			assert.Equal(t, "ext-store", config.Type)
			return tk, nil
		},
	}

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.NoError(t, err)

	res, err := sm.List(context.Background(), &proto.ListKeysRequest{
		Limit:    10,
		Continue: "key12345",
	})
	assert.NoError(t, err)
	assert.Equal(t, testRes, res)

	sm.(*signingModule).disableKeyListing = true
	_, err = sm.List(context.Background(), &proto.ListKeysRequest{
		Limit:    10,
		Continue: "key12345",
	})
	assert.Regexp(t, "PD011415", err)

}

func TestExtensionKeyStoreListFail(t *testing.T) {

	tk := &testKeyStoreAll{
		listKeys: func(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error) {
			return nil, fmt.Errorf("pop")
		},
	}
	te := &testExtension{
		keyStore: func(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) {
			assert.Equal(t, "ext-store", config.Type)
			return tk, nil
		},
	}

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.NoError(t, err)

	_, err = sm.List(context.Background(), &proto.ListKeysRequest{
		Limit:    10,
		Continue: "key12345",
	})
	assert.Regexp(t, "pop", err)

}

func TestExtensionKeyStoreResolveSignSECP256K1OK(t *testing.T) {

	tk := &testKeyStoreAll{
		findOrCreateKey_secp256k1: func(ctx context.Context, req *proto.ResolveKeyRequest) (addr *ethtypes.Address0xHex, keyHandle string, err error) {
			assert.Equal(t, "key1", req.Name)
			return ethtypes.MustNewAddress("0x98A356e0814382587D42B62Bd97871ee59D10b69"), "0x98a356e0814382587d42b62bd97871ee59d10b69", nil
		},
		sign_secp256k1: func(ctx context.Context, keyHandle string, payload []byte) (*secp256k1.SignatureData, error) {
			assert.Equal(t, "key1", keyHandle)
			assert.Equal(t, "something to sign", (string)(payload))
			return &secp256k1.SignatureData{V: big.NewInt(1), R: big.NewInt(2), S: big.NewInt(3)}, nil
		},
	}
	te := &testExtension{
		keyStore: func(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) {
			assert.Equal(t, "ext-store", config.Type)
			return tk, nil
		},
	}

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.NoError(t, err)

	resResolve, err := sm.Resolve(context.Background(), &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "key1",
	})
	assert.NoError(t, err)
	assert.Equal(t, "0x98a356e0814382587d42b62bd97871ee59d10b69", resResolve.Identifiers[0].Identifier)

	resSign, err := sm.Sign(context.Background(), &proto.SignRequest{
		KeyHandle: "key1",
		Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		Payload:   ([]byte)("something to sign"),
	})
	assert.NoError(t, err)
	// R, S, V compact encoding
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000301", hex.EncodeToString(resSign.Payload))
}

func TestExtensionKeyStoreResolveSECP256K1Fail(t *testing.T) {

	tk := &testKeyStoreAll{
		findOrCreateKey_secp256k1: func(ctx context.Context, req *proto.ResolveKeyRequest) (addr *ethtypes.Address0xHex, keyHandle string, err error) {
			return nil, "", fmt.Errorf("pop")
		},
	}
	te := &testExtension{
		keyStore: func(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) {
			assert.Equal(t, "ext-store", config.Type)
			return tk, nil
		},
	}

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.NoError(t, err)

	_, err = sm.Resolve(context.Background(), &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
	})
	assert.Regexp(t, "pop", err)

}

func TestExtensionKeyStoreSignSECP256K1Fail(t *testing.T) {

	tk := &testKeyStoreAll{
		sign_secp256k1: func(ctx context.Context, keyHandle string, payload []byte) (*secp256k1.SignatureData, error) {
			return nil, fmt.Errorf("pop")
		},
	}
	te := &testExtension{
		keyStore: func(ctx context.Context, config *api.StoreConfig) (store api.KeyStore, err error) {
			assert.Equal(t, "ext-store", config.Type)
			return tk, nil
		},
	}

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: "ext-store",
		},
	}, te)
	assert.NoError(t, err)

	_, err = sm.Sign(context.Background(), &proto.SignRequest{
		KeyHandle: "key1",
		Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		Payload:   ([]byte)("something to sign"),
	})
	assert.Regexp(t, "pop", err)

}

func TestSignInMemoryFailBadKey(t *testing.T) {

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeStatic,
		},
	})
	assert.NoError(t, err)

	_, err = sm.Sign(context.Background(), &proto.SignRequest{
		KeyHandle: "key1",
		Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		Payload:   ([]byte)("something to sign"),
	})
	assert.Regexp(t, "PD011418", err)

}

func TestResolveSignWithNewKeyCreation(t *testing.T) {

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeFilesystem,
			FileSystem: api.FileSystemConfig{
				Path: confutil.P(t.TempDir()),
			},
		},
	})
	assert.NoError(t, err)

	resolveRes, err := sm.Resolve(context.Background(), &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "key1",
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, resolveRes.KeyHandle)
	assert.Equal(t, "key1", resolveRes.KeyHandle)
	assert.Equal(t, api.Algorithm_ECDSA_SECP256K1_PLAINBYTES, resolveRes.Identifiers[0].Algorithm)
	assert.NotEmpty(t, resolveRes.Identifiers[0].Identifier)

	signRes, err := sm.Sign(context.Background(), &proto.SignRequest{
		KeyHandle: resolveRes.KeyHandle,
		Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		Payload:   ([]byte)("sign me"),
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, signRes.Payload)

}

func TestResolveUnsupportedAlgo(t *testing.T) {

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeFilesystem,
			FileSystem: api.FileSystemConfig{
				Path: confutil.P(t.TempDir()),
			},
		},
	})
	assert.NoError(t, err)

	_, err = sm.Resolve(context.Background(), &proto.ResolveKeyRequest{
		Algorithms: []string{"wrong"},
		Name:       "key1",
	})
	assert.Regexp(t, "PD011410.*wrong", err)

}

func TestResolveMissingAlgo(t *testing.T) {

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeFilesystem,
			FileSystem: api.FileSystemConfig{
				Path: confutil.P(t.TempDir()),
			},
		},
	})
	assert.NoError(t, err)

	_, err = sm.Resolve(context.Background(), &proto.ResolveKeyRequest{
		Name: "key1",
	})
	assert.Regexp(t, "PD011411", err)

}

func TestInMemorySignFailures(t *testing.T) {

	sm, err := NewSigningModule(context.Background(), &api.Config{
		KeyStore: api.StoreConfig{
			Type: api.KeyStoreTypeStatic,
			Static: api.StaticKeyStorageConfig{
				Keys: map[string]api.StaticKeyEntryConfig{
					"key1": {
						Encoding: "hex",
						Inline:   "0x00",
					},
				},
			},
		},
	})
	assert.NoError(t, err)

	resolveRes, err := sm.Resolve(context.Background(), &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "key1",
	})
	assert.NoError(t, err)

	_, err = sm.Sign(context.Background(), &proto.SignRequest{
		KeyHandle: resolveRes.KeyHandle,
		Payload:   ([]byte)("something to sign"),
	})
	assert.Regexp(t, "PD011410", err)

	_, err = sm.Resolve(context.Background(), &proto.ResolveKeyRequest{
		Algorithms: []string{"wrong"},
		Name:       "key1",
	})
	assert.Regexp(t, "PD011410", err)

	sm.(*signingModule).disableKeyLoading = true

	_, err = sm.Resolve(context.Background(), &proto.ResolveKeyRequest{
		Algorithms: []string{api.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Name:       "key1",
	})
	assert.Regexp(t, "PD011409", err)

	_, err = sm.Sign(context.Background(), &proto.SignRequest{
		KeyHandle: resolveRes.KeyHandle,
		Payload:   ([]byte)("something to sign"),
	})
	assert.Regexp(t, "PD011409", err)
}

func TestDecodeCompactRSVBadLen(t *testing.T) {
	_, err := DecodeCompactRSV(context.Background(), make([]byte, 64))
	assert.Regexp(t, "PD011420", err)
}

// func TestZetoKeystoreExtension(t *testing.T) {
// 	ctx := context.Background()
// 	zke := NewZkpSignerExtension()
// 	ks, err := zke.KeyStore(ctx, &api.StoreConfig{
// 		Type: ZkpKeyStoreSigner,
// 		FileSystem: api.FileSystemConfig{
// 			Path: confutil.P(t.TempDir()),
// 		},
// 		SnarkProver: api.SnarkProverConfig{
// 			CircuitsDir:    "/Users/jimzhang/workspace.zkp/confidential-utxo/zkp/js/lib/",
// 			ProvingKeysDir: "/Users/jimzhang/Documents/zkp/proving-keys",
// 		},
// 	})
// 	assert.NoError(t, err)

// 	keyHex := "627d15ca47363fb118997679bc8941d1ae16a034dc8ae96c938e3997e3d6ca98"
// 	keyBytes, _ := hex.DecodeString(keyHex)
// 	privKeyBytes := [32]byte{}
// 	copy(privKeyBytes[:], keyBytes)
// 	key0 := key.NewKeyEntryFromPrivateKeyBytes(privKeyBytes)

// 	req := pb.ResolveKeyRequest{
// 		Name: "42",
// 		Path: []*pb.ResolveKeyPathSegment{
// 			{Name: "bob"},
// 			{Name: "blue"},
// 		},
// 	}
// 	newKeyFunc := func() ([]byte, error) { return key0.PrivateKey[:], nil }
// 	_, keyHandle, err := ks.FindOrCreateLoadableKey(context.Background(), &req, newKeyFunc)
// 	assert.NoError(t, err)
// 	_, err = ks.LoadKeyMaterial(context.Background(), keyHandle)
// 	assert.NoError(t, err)
// }

// func TestZetoKeystoreExtensionMissingKeystoreConfig(t *testing.T) {
// 	ctx := context.Background()
// 	zke := NewZkpSignerExtension()
// 	_, err := zke.KeyStore(ctx, &api.StoreConfig{Type: ZkpKeyStoreSigner})
// 	assert.EqualError(t, err, "key store config is required")
// }

// func TestZKPSigningModuleUsingFileSystemStore(t *testing.T) {
// 	ctx, fs, dir := newTestFilesystemStore(t)

// 	// // create a BJJ key in the filesystem store
// 	alice := NewKeypair()
// 	bob := NewKeypair()

// 	_, aliceKeyHandle, err := fs.FindOrCreateLoadableKey(ctx, &pb.ResolveKeyRequest{
// 		Name: "blueKey",
// 		Path: []*pb.ResolveKeyPathSegment{
// 			{Name: "alice"},
// 		},
// 	}, func() ([]byte, error) { return alice.PrivateKey[:], nil })
// 	assert.NoError(t, err)

// 	_, bobKeyHandle, err := fs.FindOrCreateLoadableKey(ctx, &pb.ResolveKeyRequest{
// 		Name: "redKey",
// 		Path: []*pb.ResolveKeyPathSegment{
// 			{Name: "bob"},
// 		},
// 	}, func() ([]byte, error) { return bob.PrivateKey[:], nil })
// 	assert.NoError(t, err)

// 	sm, err := signer.NewSigningModule(ctx, &api.Config{
// 		KeyStore: api.StoreConfig{
// 			Type:       ZkpKeyStoreSigner,
// 			FileSystem: api.FileSystemConfig{Path: confutil.P(dir)},
// 			SnarkProver: api.SnarkProverConfig{
// 				CircuitsDir:    "/Users/jimzhang/workspace.zkp/confidential-utxo/zkp/js/lib/",
// 				ProvingKeysDir: "/Users/jimzhang/Documents/zkp/proving-keys",
// 			},
// 		},
// 	}, NewZkpSignerExtension())
// 	assert.NoError(t, err)
// 	assert.NotZero(t, sm)

// 	_, err = sm.Resolve(ctx, &pb.ResolveKeyRequest{
// 		MustExist:  true,
// 		Algorithms: []string{signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES, signer.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES},
// 		Name:       "blue",
// 		Path: []*pb.ResolveKeyPathSegment{
// 			{Name: "bob"},
// 		},
// 	})
// 	assert.EqualError(t, err, "PD011406: Key 'bob/blue' does not exist")

// 	resp, err := sm.Resolve(ctx, &pb.ResolveKeyRequest{
// 		MustExist:  true,
// 		Algorithms: []string{signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES, signer.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES},
// 		Name:       "blueKey",
// 		Path: []*pb.ResolveKeyPathSegment{
// 			{Name: "alice"},
// 		},
// 	})
// 	assert.NoError(t, err)
// 	assert.Equal(t, 2, len(resp.Identifiers))

// 	inputValues := []*big.Int{big.NewInt(30), big.NewInt(40)}
// 	outputValues := []*big.Int{big.NewInt(32), big.NewInt(38)}

// 	salt1 := utxo.NewSalt()
// 	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
// 	salt2 := utxo.NewSalt()
// 	input2, _ := poseidon.Hash([]*big.Int{inputValues[1], salt2, alice.PublicKey.X, alice.PublicKey.Y})
// 	inputCommitments := []string{input1.Text(16), input2.Text(16)}

// 	inputValueInts := []uint64{inputValues[0].Uint64(), inputValues[1].Uint64()}
// 	inputSalts := []string{salt1.Text(16), salt2.Text(16)}
// 	outputValueInts := []uint64{outputValues[0].Uint64(), outputValues[1].Uint64()}

// 	req := zeto.ProvingRequest{
// 		CircuitId: "anon",
// 		Common: &zeto.ProvingRequestCommon{
// 			InputCommitments: inputCommitments,
// 			InputValues:      inputValueInts,
// 			InputSalts:       inputSalts,
// 			InputOwner:       aliceKeyHandle,
// 			OutputValues:     outputValueInts,
// 			OutputOwners:     []string{aliceKeyHandle, bobKeyHandle},
// 		},
// 	}
// 	payload, err := proto.Marshal(&req)
// 	assert.NoError(t, err)

// 	resSign, err := sm.Sign(ctx, &pb.SignRequest{
// 		KeyHandle: resp.KeyHandle,
// 		Algorithm: signer.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES,
// 		Payload:   payload,
// 	})
// 	assert.NoError(t, err)
// 	assert.NotZero(t, resSign.Payload)
// }
