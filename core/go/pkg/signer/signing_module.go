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
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/core/pkg/signer/keystores"
	"github.com/kaleido-io/paladin/core/pkg/signer/signerapi"
	"github.com/kaleido-io/paladin/core/pkg/signer/signers"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
)

// SigningModule provides functions for the protobuf request/reply functions from the proto interface defined
// in signing_module.
// This module can be wrapped and loaded into the core Paladin runtime as an embedded module called directly
// within the runtime, or wrapped in a remote process connected over a transport like HTTP, WebSockets, gRPC etc.
type SigningModule interface {
	Resolve(ctx context.Context, req *proto.ResolveKeyRequest) (res *proto.ResolveKeyResponse, err error)
	Sign(ctx context.Context, req *proto.SignRequest) (res *proto.SignResponse, err error)
	List(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error)
	Close()
}

type hdDerivation[C signerapi.ExtensibleConfig] struct {
	sm                    *signingModule[C]
	bip44DirectResolution bool
	bip44HardenedSegments int
	bip44Prefix           string
	hdKeyChain            *hdkeychain.ExtendedKey
}

type signingModule[C signerapi.ExtensibleConfig] struct {
	keyStore                signerapi.KeyStore
	keyStoreSigner          signerapi.KeyStoreSigner
	disableKeyListing       bool
	hd                      *hdDerivation[C]
	signingImplementations  map[string]signerapi.InMemorySigner
	keyStoreImplementations map[string]signerapi.KeyStoreFactory[C]
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
func NewSigningModule[C signerapi.ExtensibleConfig](ctx context.Context, conf C, extensions ...*signerapi.Extensions[C]) (_ SigningModule, err error) {

	ecdsaSigner, _ := signers.NewECDSASignerFactory[C]().NewSigner(ctx, conf) // this factory has no errors as it does not parse any config
	sm := &signingModule[C]{
		signingImplementations: map[string]signerapi.InMemorySigner{
			algorithms.Prefix_ECDSA: ecdsaSigner,
		},
	}
	keyStoreImplementations := map[string]signerapi.KeyStoreFactory[C]{
		signerapi.KeyStoreTypeFilesystem: keystores.NewFilesystemStoreFactory[C](),
		signerapi.KeyStoreTypeStatic:     keystores.NewStaticStoreFactory[C](),
	}

	for _, e := range extensions {
		// We construct ALL of the supplied signers as any can be used dynamically
		for name, imsf := range e.InMemorySignerFactories {
			sm.signingImplementations[name], err = imsf.NewSigner(ctx, conf)
			if err != nil {
				return nil, err
			}
		}
		// We only construct a single storage system, so here we just put them in a map
		// to construct the one picked in the configuration in the next block
		for name, ksf := range e.KeyStoreFactories {
			keyStoreImplementations[name] = ksf
		}
	}

	// Now we have all the possible factories mapped, we load the one keystore type we actually use
	ksConf := conf.KeyStoreConfig()
	keyStoreType := strings.ToLower(ksConf.Type)
	ksf := sm.keyStoreImplementations[keyStoreType]
	if ksf == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSigningUnsupportedKeyStoreType, ksConf.Type)
	}
	sm.keyStore, err = ksf.NewKeyStore(ctx, conf)
	if err != nil {
		return nil, err
	}

	// Check if we'be been asked to delegate signing directly to the key storage system
	// (disabling ALL in memory signing modules)
	if ksConf.KeyStoreSigning {
		var supportsSigning bool
		sm.keyStoreSigner, supportsSigning = sm.keyStore.(signerapi.KeyStoreSigner)
		if !supportsSigning {
			return nil, i18n.NewError(ctx, msgs.MsgSigningKeyStoreNoInStoreSingingSupport, ksConf.Type)
		}
	}

	kdConf := conf.KeyDerivationConfig()
	switch kdConf.Type {
	case "", signerapi.KeyDerivationTypeDirect:
	case signerapi.KeyDerivationTypeBIP32:
		// This is fundamentally incompatible with a request to disable loading key materials into memory
		if ksConf.KeyStoreSigning {
			return nil, i18n.NewError(ctx, msgs.MsgSigningHierarchicalRequiresLoading)
		}
		if err := sm.initHDWallet(ctx, kdConf); err != nil {
			return nil, err
		}
	default:
		return nil, i18n.NewError(ctx, msgs.MsgSigningUnsupportedKeyDerivationType, kdConf.Type)
	}

	// Settings that disable behaviors, whether technically supported by the key store or not
	sm.disableKeyListing = ksConf.DisableKeyListing

	return sm, err
}

func (sm *signingModule[C]) getSignerForAlgorithm(ctx context.Context, algorithm string) (signerapi.InMemorySigner, error) {
	lookupPrefix := strings.ToLower(strings.SplitN(algorithm, ":", 2)[0])
	signer := sm.signingImplementations[lookupPrefix]
	if signer == nil {
		// No signer registered for this algorithm prefix
		return nil, i18n.NewError(ctx, msgs.MsgSigningUnsupportedAlgoForInMemorySigning, algorithm)
	}
	return signer, nil
}

func (sm *signingModule[C]) newKeyForAlgorithms(ctx context.Context, requiredIdentifiers []*proto.PublicKeyIdentifierType) ([]byte, error) {
	var keyLen = 0
	for _, requiredIdentifier := range requiredIdentifiers {
		var algoKeyLen int
		signer, err := sm.getSignerForAlgorithm(ctx, requiredIdentifier.Algorithm)
		if err == nil {
			algoKeyLen, err = signer.GetMinimumKeyLen(ctx, requiredIdentifier.Algorithm)
		}
		if err != nil {
			return nil, err
		}
		if algoKeyLen > keyLen {
			keyLen = algoKeyLen
		}
	}
	if keyLen <= 0 {
		return nil, i18n.NewError(ctx, msgs.MsgSigningMustSpecifyAlgorithms)
	}
	// Generate random bytes for the size
	buff := make([]byte, keyLen)
	_, err := rand.Read(buff)
	return buff, err
}

func (sm *signingModule[C]) signInMemory(ctx context.Context, algorithm, payloadType string, privateKey, payload []byte) (res *proto.SignResponse, err error) {
	signer, err := sm.getSignerForAlgorithm(ctx, algorithm)
	if err != nil {
		return nil, err
	}
	resultBytes, err := signer.Sign(ctx, algorithm, payloadType, privateKey, payload)
	if err != nil {
		return nil, err
	}
	return &proto.SignResponse{
		Payload: resultBytes,
	}, nil
}

func (sm *signingModule[C]) Resolve(ctx context.Context, req *proto.ResolveKeyRequest) (res *proto.ResolveKeyResponse, err error) {

	if len(req.Name) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgSigningKeyCannotBeEmpty)
	}

	// If we are delegating resolution to the keystore (hence all our in memory signers are disabled)
	// then that's what we do in all cases. An individual signer works in one mode or the other
	if sm.keyStoreSigner != nil {
		return sm.keyStoreSigner.FindOrCreateInStoreSigningKey(ctx, req)
	}
	// If we have HD wallet derivation, then that is where we do the resolution
	if sm.hd != nil {
		return sm.hd.resolveHDWalletKey(ctx, req)
	}
	// Otherwise load up the key from the keystore into memory and build the verifiers
	privateKey, keyHandle, err := sm.keyStore.FindOrCreateLoadableKey(ctx, req, func() ([]byte, error) {
		return sm.newKeyForAlgorithms(ctx, req.RequiredIdentifiers)
	})
	if err != nil {
		return nil, err
	}
	return sm.buildResolveResponseWithIdentifiers(ctx, keyHandle, privateKey, req.RequiredIdentifiers)
}

func (sm *signingModule[C]) buildResolveResponseWithIdentifiers(ctx context.Context, keyHandle string, privateKey []byte, requiredIdentifiers []*proto.PublicKeyIdentifierType) (*proto.ResolveKeyResponse, error) {
	identifiers := make([]*proto.PublicKeyIdentifier, len(requiredIdentifiers))
	for i, required := range requiredIdentifiers {
		resolved := &proto.PublicKeyIdentifier{
			Algorithm:    required.Algorithm,
			VerifierType: required.VerifierType,
		}
		signer, err := sm.getSignerForAlgorithm(ctx, required.Algorithm)
		if err == nil {
			resolved.Verifier, err = signer.GetVerifier(ctx, required.Algorithm, required.VerifierType, privateKey)
		}
		if err != nil {
			return nil, err
		}
		identifiers[i] = resolved
	}
	return &proto.ResolveKeyResponse{
		KeyHandle:   keyHandle,
		Identifiers: identifiers,
	}, nil
}

func (sm *signingModule[C]) Sign(ctx context.Context, req *proto.SignRequest) (res *proto.SignResponse, err error) {
	// If we are delegating resolution to the keystore (hence all our in memory signers are disabled)
	// then that's what we do in all cases. An individual signer works in one mode or the other
	if sm.keyStoreSigner != nil {
		return sm.keyStoreSigner.SignWithinKeystore(ctx, req)
	}
	// If we have HD wallet derivation, then that is where we do the signing
	if sm.hd != nil {
		return sm.hd.signHDWalletKey(ctx, req)
	}
	// Otherwise load up the key from the keystore into memory and do the signing
	privateKey, err := sm.keyStore.LoadKeyMaterial(ctx, req.KeyHandle)
	if err != nil {
		return nil, err
	}
	return sm.signInMemory(ctx, req.Algorithm, req.PayloadType, privateKey, req.Payload)
}

func (sm *signingModule[C]) List(ctx context.Context, req *proto.ListKeysRequest) (res *proto.ListKeysResponse, err error) {
	listableStore, isListable := sm.keyStore.(signerapi.KeyStoreListable)
	if !isListable || sm.disableKeyListing {
		return nil, i18n.NewError(ctx, msgs.MsgSigningKeyListingNotSupported)
	}
	return listableStore.ListKeys(ctx, req)
}

func (sm *signingModule[C]) Close() {
	sm.keyStore.Close()
}
