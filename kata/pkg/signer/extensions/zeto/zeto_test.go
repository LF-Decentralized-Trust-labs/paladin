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

package extensions

import (
	"context"
	"encoding/hex"
	"math/big"
	"os"
	"path"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/types"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/proto/zeto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/signer/keystore"
	"google.golang.org/protobuf/proto"
)

type User struct {
	PrivateKey       *babyjub.PrivateKey
	PublicKey        *babyjub.PublicKey
	PrivateKeyBigInt *big.Int
}

// generate a new BabyJub keypair
func NewKeypair() *User {
	// generate babyJubjub private key randomly
	babyJubjubPrivKey := babyjub.NewRandPrivKey()
	// generate public key from private key
	babyJubjubPubKey := babyJubjubPrivKey.Public()
	// convert the private key to big.Int for use inside circuits
	privKeyBigInt := babyjub.SkToBigInt(&babyJubjubPrivKey)

	return &User{
		PrivateKey:       &babyJubjubPrivKey,
		PublicKey:        babyJubjubPubKey,
		PrivateKeyBigInt: privKeyBigInt,
	}
}

func newTestFilesystemStore(t *testing.T) (context.Context, api.KeyStore, string) {
	ctx := context.Background()

	dirPath := t.TempDir()
	store, err := keystore.NewFilesystemStore(ctx, &api.FileSystemConfig{
		Path: confutil.P(dirPath),
	})
	assert.NoError(t, err)

	return ctx, store, dirPath
}

func newTestStaticStore(t *testing.T, keys map[string]api.StaticKeyEntryConfig) (context.Context, api.KeyStore) {
	ctx := context.Background()

	store, err := keystore.NewStaticKeyStore(ctx, &api.StaticKeyStorageConfig{
		Keys: keys,
	})
	assert.NoError(t, err)

	return ctx, store
}

func TestFileSystemStoreCreateBJJ(t *testing.T) {
	ctx, fs, _ := newTestFilesystemStore(t)

	key0 := babyjub.NewRandPrivKey()

	keyBytes, keyHandle, err := fs.FindOrCreateLoadableKey(ctx, &pb.ResolveKeyRequest{
		Path: []*pb.KeyPathSegment{
			{Name: "bob"},
			{Name: "blue"},
			{Name: "42"},
		},
	}, func() ([]byte, error) { return key0[:], nil })
	assert.NoError(t, err)

	assert.Equal(t, keyBytes, key0[:])
	assert.Equal(t, "bob/blue/42", keyHandle)

	keyBytes, err = fs.LoadKeyMaterial(ctx, keyHandle)
	assert.NoError(t, err)
	assert.Equal(t, keyBytes, key0[:])

	privKeyBytes := [32]byte{}
	copy(privKeyBytes[:], keyBytes)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(privKeyBytes)
	assert.NotZero(t, keyEntry.PrivateKey)
	assert.NotZero(t, keyEntry.PublicKey)
	assert.NotZero(t, keyEntry.PrivateKeyForZkp)
}

func TestStaticStoreFileFileWithTrim(t *testing.T) {
	keyData := types.RandHex(32)
	keyFile := path.Join(t.TempDir(), "my.key")
	err := os.WriteFile(keyFile, []byte(keyData+"\n"), 0644)
	assert.NoError(t, err)

	ctx, store := newTestStaticStore(t, map[string]api.StaticKeyEntryConfig{
		"myKey": {
			Encoding: "none",
			Filename: keyFile,
			Trim:     true,
		},
	})

	loadedKey, err := store.LoadKeyMaterial(ctx, "myKey")
	assert.NoError(t, err)
	assert.Equal(t, ([]byte)(keyData), loadedKey)

	var keyBytes [32]byte
	copy(keyBytes[:], keyData)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(keyBytes)
	assert.NotZero(t, keyEntry.PrivateKey)
	assert.NotZero(t, keyEntry.PublicKey)
	assert.NotZero(t, keyEntry.PrivateKeyForZkp)
}

func TestZetoKeystoreExtension(t *testing.T) {
	ctx := context.Background()
	zke := NewZkpSignerExtension()
	ks, err := zke.KeyStore(ctx, &api.StoreConfig{
		Type: ZkpKeyStoreSigner,
		FileSystem: &api.FileSystemConfig{
			Path: confutil.P(t.TempDir()),
		},
		ZkpProver: &api.ZkpProverConfig{
			CircuitsDir:    "/Users/jimzhang/workspace.zkp/confidential-utxo/zkp/js/lib/",
			ProvingKeysDir: "/Users/jimzhang/Documents/zkp/proving-keys",
		},
	})
	assert.NoError(t, err)

	keyHex := "627d15ca47363fb118997679bc8941d1ae16a034dc8ae96c938e3997e3d6ca98"
	keyBytes, _ := hex.DecodeString(keyHex)
	privKeyBytes := [32]byte{}
	copy(privKeyBytes[:], keyBytes)
	key0 := key.NewKeyEntryFromPrivateKeyBytes(privKeyBytes)

	req := pb.ResolveKeyRequest{
		Path: []*pb.KeyPathSegment{
			{Name: "bob"},
			{Name: "blue"},
		},
	}
	newKeyFunc := func() ([]byte, error) { return key0.PrivateKey[:], nil }
	_, keyHandle, err := ks.FindOrCreateLoadableKey(context.Background(), &req, newKeyFunc)
	assert.NoError(t, err)
	_, err = ks.LoadKeyMaterial(context.Background(), keyHandle)
	assert.NoError(t, err)
}

func TestZetoKeystoreExtensionMissingKeystoreConfig(t *testing.T) {
	ctx := context.Background()
	zke := NewZkpSignerExtension()
	_, err := zke.KeyStore(ctx, &api.StoreConfig{Type: ZkpKeyStoreSigner})
	assert.EqualError(t, err, "key store config is required")
}

func TestZKPSigningModuleUsingFileSystemStore(t *testing.T) {
	ctx, fs, dir := newTestFilesystemStore(t)

	// // create a BJJ key in the filesystem store
	alice := NewKeypair()
	bob := NewKeypair()

	_, aliceKeyHandle, err := fs.FindOrCreateLoadableKey(ctx, &pb.ResolveKeyRequest{
		Path: []*pb.KeyPathSegment{
			{Name: "alice"},
			{Name: "blueKey"},
		},
	}, func() ([]byte, error) { return alice.PrivateKey[:], nil })
	assert.NoError(t, err)

	_, bobKeyHandle, err := fs.FindOrCreateLoadableKey(ctx, &pb.ResolveKeyRequest{
		Path: []*pb.KeyPathSegment{
			{Name: "bob"},
			{Name: "redKey"},
		},
	}, func() ([]byte, error) { return bob.PrivateKey[:], nil })
	assert.NoError(t, err)

	sm, err := signer.NewSigningModule(ctx, &api.Config{
		KeyStore: api.StoreConfig{
			Type:       ZkpKeyStoreSigner,
			FileSystem: &api.FileSystemConfig{Path: confutil.P(dir)},
			ZkpProver: &api.ZkpProverConfig{
				CircuitsDir:    "/Users/jimzhang/workspace.zkp/confidential-utxo/zkp/js/lib/",
				ProvingKeysDir: "/Users/jimzhang/Documents/zkp/proving-keys",
			},
		},
	}, NewZkpSignerExtension())
	assert.NoError(t, err)
	assert.NotZero(t, sm)

	_, err = sm.Resolve(ctx, &pb.ResolveKeyRequest{
		MustExist:  true,
		Algorithms: []string{signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES, signer.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES},
		Path: []*pb.KeyPathSegment{
			{Name: "bob"},
			{Name: "blue"},
		},
	})
	assert.EqualError(t, err, "PD011406: Key 'bob/blue' does not exist")

	resp, err := sm.Resolve(ctx, &pb.ResolveKeyRequest{
		MustExist:  true,
		Algorithms: []string{signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES, signer.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES},
		Path: []*pb.KeyPathSegment{
			{Name: "alice"},
			{Name: "blueKey"},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(resp.Identifiers))

	inputValues := []*big.Int{big.NewInt(30), big.NewInt(40)}
	outputValues := []*big.Int{big.NewInt(32), big.NewInt(38)}

	salt1 := utxo.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := utxo.NewSalt()
	input2, _ := poseidon.Hash([]*big.Int{inputValues[1], salt2, alice.PublicKey.X, alice.PublicKey.Y})
	inputCommitments := []string{input1.Text(16), input2.Text(16)}

	inputValueInts := []uint64{inputValues[0].Uint64(), inputValues[1].Uint64()}
	inputSalts := []string{salt1.Text(16), salt2.Text(16)}
	outputValueInts := []uint64{outputValues[0].Uint64(), outputValues[1].Uint64()}

	req := zeto.ProvingRequest{
		CircuitId: "anon",
		Common: &zeto.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputValues:      inputValueInts,
			InputSalts:       inputSalts,
			InputOwner:       aliceKeyHandle,
			OutputValues:     outputValueInts,
			OutputOwners:     []string{aliceKeyHandle, bobKeyHandle},
		},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)

	resSign, err := sm.Sign(ctx, &pb.SignRequest{
		KeyHandle: resp.KeyHandle,
		Algorithm: signer.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.NoError(t, err)
	assert.NotZero(t, resSign.Payload)
}
