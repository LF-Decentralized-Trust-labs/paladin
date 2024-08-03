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
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

// SigningModule provides functions for the protobuf request/reply functions from the proto interface defined
// in signing_module.
// This module can be wrapped and loaded into the core Paladin runtime as an embedded module called directly
// on the comms bus, or wrapped in a remote process connected over gRPC.
type SigningModule interface {
	Resolve(ctx context.Context, req *proto.ResolveKeyRequest) (res *proto.ResolveKeyResponse, err error)
	Sign(ctx context.Context, req *proto.SignRequest) (res *proto.SignResponse, err error)
	List(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error)
}

type Extension interface {
	// Return nil if keystore type is not known, or error if initialization fails
	KeyStore(keystoreType string) (store KeyStore, err error)
}

type hdDerivation struct {
	sm                    *signingModule
	bip44DirectResolution bool
	bip44HardenedSegments int
	bip44Prefix           string
	hdKeyChain            *hdkeychain.ExtendedKey
}

type signingModule struct {
	keyStore          KeyStore
	disableKeyListing bool
	disableKeyLoading bool
	hd                *hdDerivation
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
func NewSigningModule(ctx context.Context, config *Config, extensions ...Extension) (_ SigningModule, err error) {
	sm := &signingModule{}

	keyStoreType := strings.ToLower(config.KeyStore.Type)
	switch keyStoreType {
	case "", KeyStoreTypeFilesystem:
		if sm.keyStore, err = newFilesystemStore(ctx, &config.KeyStore.FileSystem); err != nil {
			return nil, err
		}
	case KeyStoreTypeStatic:
		if sm.keyStore, err = newStaticKeyStore(ctx, &config.KeyStore.Static); err != nil {
			return nil, err
		}
	default:
		for _, ext := range extensions {
			store, err := ext.KeyStore(keyStoreType)
			if err != nil {
				return nil, err
			}
			if store != nil {
				sm.keyStore = store
				break
			}
		}
		if sm.keyStore == nil {
			return nil, i18n.NewError(ctx, msgs.MsgSigningUnsupportedKeyStoreType, config.KeyStore.Type)
		}
	}

	switch config.KeyDerivation.Type {
	case "", KeyDerivationTypeDirect:
	case KeyDerivationTypeBIP32:
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
	return sm, nil
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
			{Algorithm: Algorithm_ECDSA_SECP256K1, Identifier: addr.String()},
		},
	}, nil
}

// We use the ethereum convention of R,S,V for compact packing (mentioned because Golang tends to prefer V,R,S)
func compactRSV(sig *secp256k1.SignatureData) []byte {
	signatureBytes := make([]byte, 65)
	sig.R.FillBytes(signatureBytes[0:32])
	sig.S.FillBytes(signatureBytes[32:64])
	signatureBytes[64] = byte(sig.V.Int64())
	return signatureBytes
}

func (sm *signingModule) signKeystoreSECP256K1(ctx context.Context, req *proto.SignRequest, keyStoreSigner KeyStoreSigner_secp256k1) (res *proto.SignResponse, err error) {
	sig, err := keyStoreSigner.Sign_secp256k1(ctx, req.KeyHandle, req.Payload)
	if err != nil {
		return nil, err
	}
	return &proto.SignResponse{
		Payload: compactRSV(sig),
	}, nil
}

func (sm *signingModule) getKeyLenForInMemorySigning(ctx context.Context, algorithms []string) (int, error) {
	keyLen := 0
	for _, algo := range algorithms {
		switch strings.ToLower(algo) {
		case Algorithm_ECDSA_SECP256K1:
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
	switch strings.ToLower(req.Algorithm) {
	case Algorithm_ECDSA_SECP256K1:
		kp, _ := secp256k1.NewSecp256k1KeyPair(privateKey)
		sig, err := kp.SignDirect(req.Payload)
		if err == nil {
			return &proto.SignResponse{Payload: compactRSV(sig)}, nil
		}
	default:
		err = i18n.NewError(ctx, msgs.MsgSigningUnsupportedAlgoForInMemorySigning, req.Algorithm)
	}
	return nil, err
}

func (sm *signingModule) publicKeyIdentifiersForAlgorithms(ctx context.Context, keyHandle string, privateKey []byte, algorithms []string) (*proto.ResolveKeyResponse, error) {
	var identifiers []*proto.PublicKeyIdentifier
	for _, algo := range algorithms {
		switch strings.ToLower(algo) {
		case Algorithm_ECDSA_SECP256K1:
			addr, _ := secp256k1.NewSecp256k1KeyPair(privateKey)
			identifiers = append(identifiers, &proto.PublicKeyIdentifier{
				Algorithm:  Algorithm_ECDSA_SECP256K1,
				Identifier: addr.Address.String(),
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
	if sm.hd != nil {
		return sm.hd.resolveHDWalletKey(ctx, req)
	}
	if len(req.Algorithms) == 1 && req.Algorithms[0] == Algorithm_ECDSA_SECP256K1 {
		keyStoreSigner, ok := sm.keyStore.(KeyStoreSigner_secp256k1)
		if ok {
			return sm.resolveKeystoreSECP256K1(ctx, req, keyStoreSigner)
		}
	}
	// We are going to use the key store to load/decrypt a key into our volatile memory
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
	if req.Algorithm == Algorithm_ECDSA_SECP256K1 {
		keyStoreSigner, ok := sm.keyStore.(KeyStoreSigner_secp256k1)
		if ok {
			return sm.signKeystoreSECP256K1(ctx, req, keyStoreSigner)
		}
	}
	// We are going to use the key store to load/decrypt a key into our volatile memory
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
	listableStore, isListable := sm.keyStore.(KeyStoreListable)
	if !isListable || sm.disableKeyListing {
		return nil, i18n.NewError(ctx, msgs.MsgSigningKeyListingNotSupported)
	}
	return listableStore.ListKeys(ctx, req)
}
