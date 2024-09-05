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
	"crypto/rand"
	"strings"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/core/pkg/signer/api"
	sepc256k1Signer "github.com/kaleido-io/paladin/core/pkg/signer/in-memory/secp256k1"
	zkpSigner "github.com/kaleido-io/paladin/core/pkg/signer/in-memory/snark"
	"github.com/kaleido-io/paladin/core/pkg/signer/keystore"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
)

// SigningModule provides functions for the protobuf request/reply functions from the proto interface defined
// in signing_module.
// This module can be wrapped and loaded into the core Paladin runtime as an embedded module called directly
// on the comms bus, or wrapped in a remote process connected over gRPC.
type SigningModule interface {
	Resolve(ctx context.Context, req *proto.ResolveKeyRequest) (res *proto.ResolveKeyResponse, err error)
	Sign(ctx context.Context, req *proto.SignRequest) (res *proto.SignResponse, err error)
	List(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error)
	Close()
}

type hdDerivation struct {
	sm                    *signingModule
	bip44DirectResolution bool
	bip44HardenedSegments int
	bip44Prefix           string
	hdKeyChain            *hdkeychain.ExtendedKey
}

type signingModule struct {
	keyStore          api.KeyStore
	disableKeyListing bool
	disableKeyLoading bool
	hd                *hdDerivation
	inMemorySigners   map[string]api.InMemorySigner
}

// We allow this same code to be used (un-modified) with set of initialization functions passed
// in for additional keystores (and potentially other types of extension in the future).
//
// This "pkg/signer" go code module is the building block to build your sophisticated remote
// signer on top of only needing to implement the specifics to your particular system.
//
// Note that the interface is protobuf, so you can also use this code as inspiration to build
// your own signing module in a different language (like Java), but be aware that if you wish
// to support ZKP proof generation based tokens you will need to consider the ability to
// host and execute WASM code.
//
// At the same time, it is package inside of Paladin and runs with a default set of key storage
// technologies (which can themselves be remote) inside the paladin runtime.
//
// Check out the architecture docs for more information about why the modular structure is that way,
// and for important concepts like "key handles" (the response of this module) and
// and "key mappings" (the lookup reference table managed in front of this module by the Paladin
// runtime).
//
// The design is such that all built-in behaviors should be both:
// 1. Easy to re-use if they are valuable with your extension
// 2. Easy to disable in the Config object passed in, if you do not want to have them enabled
func NewSigningModule(ctx context.Context, config *api.Config, extensions ...api.Extension) (_ SigningModule, err error) {
	sm := &signingModule{}

	keyStoreType := strings.ToLower(config.KeyStore.Type)
	switch keyStoreType {
	case "", api.KeyStoreTypeFilesystem:
		sm.keyStore, err = keystore.NewFilesystemStore(ctx, config.KeyStore.FileSystem)
	case api.KeyStoreTypeStatic:
		sm.keyStore, err = keystore.NewStaticKeyStore(ctx, config.KeyStore.Static)
	default:
		for _, ext := range extensions {
			store, err := ext.KeyStore(ctx, &config.KeyStore)
			if err != nil {
				return nil, err
			}
			if store != nil {
				sm.keyStore = store
				break
			}
		}
		if sm.keyStore == nil {
			err = i18n.NewError(ctx, msgs.MsgSigningUnsupportedKeyStoreType, config.KeyStore.Type)
		}
	}
	if err != nil {
		return nil, err
	}

	switch config.KeyDerivation.Type {
	case "", api.KeyDerivationTypeDirect:
	case api.KeyDerivationTypeBIP32:
		// This is fundamentally incompatible with a request to disable loading key materials into memory
		if config.KeyStore.DisableKeyLoading {
			return nil, i18n.NewError(ctx, msgs.MsgSigningHierarchicalRequiresLoading)
		}
		if err := sm.initHDWallet(ctx, &config.KeyDerivation); err != nil {
			return nil, err
		}
	default:
		return nil, i18n.NewError(ctx, msgs.MsgSigningUnsupportedKeyDerivationType, config.KeyDerivation.Type)
	}

	// Settings that disable behaviors, whether technically supported by the key store or not
	sm.disableKeyListing = config.KeyStore.DisableKeyListing
	sm.disableKeyLoading = config.KeyStore.DisableKeyLoading

	// Register any in-memory signers
	sm.inMemorySigners = make(map[string]api.InMemorySigner)
	sepc256k1Signer.Register(sm.inMemorySigners)
	err = zkpSigner.Register(ctx, config.KeyStore.SnarkProver, sm.inMemorySigners)

	return sm, err
}

func (sm *signingModule) newKeyForAlgorithms(ctx context.Context, algorithms []string) ([]byte, error) {
	keyLen, err := sm.getKeyLenForInMemorySigning(ctx, algorithms)
	if err != nil {
		return nil, err
	}
	buff := make([]byte, keyLen)
	_, err = rand.Read(buff)
	return buff, err
}

func (sm *signingModule) resolveKeystoreSECP256K1(ctx context.Context, req *proto.ResolveKeyRequest, keyStoreSigner KeyStoreSigner_secp256k1) (res *proto.ResolveKeyResponse, err error) {

	addr, keyHandle, err := keyStoreSigner.FindOrCreateKey_secp256k1(ctx, req)
	if err != nil {
		return nil, err
	}
	return &proto.ResolveKeyResponse{
		KeyHandle: keyHandle,
		Identifiers: []*proto.PublicKeyIdentifier{
			{Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES, Identifier: addr.String()},
		},
	}, nil
}

func (sm *signingModule) signKeystoreSECP256K1(ctx context.Context, req *proto.SignRequest, keyStoreSigner KeyStoreSigner_secp256k1) (res *proto.SignResponse, err error) {
	sig, err := keyStoreSigner.Sign_secp256k1(ctx, req.KeyHandle, req.Payload)
	if err != nil {
		return nil, err
	}
	return &proto.SignResponse{
		Payload: sig.CompactRSV(),
	}, nil
}

func (sm *signingModule) getKeyLenForInMemorySigning(ctx context.Context, requiredAlgorithms []string) (int, error) {
	keyLen := 0
	for _, algo := range requiredAlgorithms {
		switch strings.ToLower(algo) {
		case algorithms.ECDSA_SECP256K1_PLAINBYTES, algorithms.ZKP_BABYJUBJUB_PLAINBYTES:
			keyLen = 32
		default:
			return -1, i18n.NewError(ctx, msgs.MsgSigningUnsupportedAlgoForInMemorySigning, algo)
		}
	}
	if keyLen <= 0 {
		return -1, i18n.NewError(ctx, msgs.MsgSigningMustSpecifyAlgorithms)
	}
	return keyLen, nil
}

func (sm *signingModule) signInMemory(ctx context.Context, privateKey []byte, req *proto.SignRequest) (res *proto.SignResponse, err error) {
	algo := strings.ToLower(req.Algorithm)
	signer, ok := sm.inMemorySigners[algo]
	if !ok {
		return nil, i18n.NewError(ctx, msgs.MsgSigningUnsupportedAlgoForInMemorySigning, req.Algorithm)
	}
	return signer.Sign(ctx, privateKey, req)
}

func (sm *signingModule) publicKeyIdentifiersForAlgorithms(ctx context.Context, keyHandle string, privateKey []byte, requiredAlgorithms []string) (*proto.ResolveKeyResponse, error) {
	var identifiers []*proto.PublicKeyIdentifier
	for _, algo := range requiredAlgorithms {
		switch strings.ToLower(algo) {
		case algorithms.ECDSA_SECP256K1_PLAINBYTES:
			addr := secp256k1.KeyPairFromBytes(privateKey)
			identifiers = append(identifiers, &proto.PublicKeyIdentifier{
				Algorithm:  algorithms.ECDSA_SECP256K1_PLAINBYTES,
				Identifier: addr.Address.String(),
			})
		case algorithms.ZKP_BABYJUBJUB_PLAINBYTES:
			var privKeyBytes [32]byte
			copy(privKeyBytes[:], privateKey)
			keyEntry := key.NewKeyEntryFromPrivateKeyBytes(privKeyBytes)
			identifiers = append(identifiers, &proto.PublicKeyIdentifier{
				Algorithm:  algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
				Identifier: keyEntry.PublicKey.String(),
			})
		default:
			return nil, i18n.NewError(ctx, msgs.MsgSigningUnsupportedAlgoForInMemorySigning, algo)
		}
	}
	return &proto.ResolveKeyResponse{
		KeyHandle:   keyHandle,
		Identifiers: identifiers,
	}, nil
}

func (sm *signingModule) Resolve(ctx context.Context, req *proto.ResolveKeyRequest) (res *proto.ResolveKeyResponse, err error) {
	if len(req.Name) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgSigningKeyCannotBeEmpty)
	}
	if sm.hd != nil {
		return sm.hd.resolveHDWalletKey(ctx, req)
	}
	if len(req.Algorithms) == 1 && req.Algorithms[0] == algorithms.ECDSA_SECP256K1_PLAINBYTES {
		keyStoreSigner, ok := sm.keyStore.(KeyStoreSigner_secp256k1)
		if ok {
			// found a key store signer configured which does not expose private key materials
			// but encapsulates the signing logic. delegate further handling to the signer
			return sm.resolveKeystoreSECP256K1(ctx, req, keyStoreSigner)
		}
	}

	// No key store signer for the requested algorithm - we need to
	// load/decrypt a key into our volatile memory
	if sm.disableKeyLoading {
		return nil, i18n.NewError(ctx, msgs.MsgSigningStoreRequiresKeyLoadingForAlgo, strings.Join(req.Algorithms, ","))
	}
	privateKey, keyHandle, err := sm.keyStore.FindOrCreateLoadableKey(ctx, req, func() ([]byte, error) {
		return sm.newKeyForAlgorithms(ctx, req.Algorithms)
	})
	if err != nil {
		return nil, err
	}
	return sm.publicKeyIdentifiersForAlgorithms(ctx, keyHandle, privateKey, req.Algorithms)
}

func (sm *signingModule) Sign(ctx context.Context, req *proto.SignRequest) (res *proto.SignResponse, err error) {
	if sm.hd != nil {
		return sm.hd.signHDWalletKey(ctx, req)
	}
	if req.Algorithm == algorithms.ECDSA_SECP256K1_PLAINBYTES {
		keyStoreSigner, ok := sm.keyStore.(KeyStoreSigner_secp256k1)
		if ok {
			return sm.signKeystoreSECP256K1(ctx, req, keyStoreSigner)
		}
	}

	// No key store signer for the requested algorithm - we need to sign in memory
	// by asking the key store to load/decrypt a key into our volatile memory
	if sm.disableKeyLoading {
		return nil, i18n.NewError(ctx, msgs.MsgSigningStoreRequiresKeyLoadingForAlgo, req.Algorithm)
	}
	privateKey, err := sm.keyStore.LoadKeyMaterial(ctx, req.KeyHandle)
	if err != nil {
		return nil, err
	}
	return sm.signInMemory(ctx, privateKey, req)
}

func (sm *signingModule) List(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error) {
	listableStore, isListable := sm.keyStore.(api.KeyStoreListable)
	if !isListable || sm.disableKeyListing {
		return nil, i18n.NewError(ctx, msgs.MsgSigningKeyListingNotSupported)
	}
	return listableStore.ListKeys(ctx, req)
}

func (sm *signingModule) Close() {
	sm.keyStore.Close()
}
