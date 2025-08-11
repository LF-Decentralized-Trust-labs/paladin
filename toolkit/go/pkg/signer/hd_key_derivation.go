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

package signer

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/tyler-smith/go-bip39"
)

var (
	BIP32BaseDerivationPath = []uint32{0x80000000 + 44, 0x80000000 + 60}
)

type hdWalletPathEntry struct {
	Name  string
	Index uint64
}

func configToKeyResolutionRequest(k *pldconf.StaticKeyReference) (string, *prototk.ResolveKeyRequest) {
	if k.KeyHandle != "" {
		return k.KeyHandle, nil
	}
	keyReq := &prototk.ResolveKeyRequest{
		Name:       k.Name,
		Index:      k.Index,
		Attributes: k.Attributes,
		Path:       []*prototk.ResolveKeyPathSegment{},
	}
	for _, p := range k.Path {
		keyReq.Path = append(keyReq.Path, &prototk.ResolveKeyPathSegment{
			Name:  p.Name,
			Index: p.Index,
		})
	}
	return "", keyReq
}

func (sm *signingModule[C]) initHDWallet(ctx context.Context, conf *pldconf.KeyDerivationConfig) (err error) {
	bip44Prefix := confutil.StringNotEmpty(conf.BIP44Prefix, *pldconf.KeyDerivationDefaults.BIP44Prefix)
	bip44Prefix = strings.ReplaceAll(bip44Prefix, " ", "")
	sm.hd = &hdDerivation[C]{
		sm:                    sm,
		bip44Prefix:           bip44Prefix,
		bip44DirectResolution: conf.BIP44DirectResolution,
		bip44HardenedSegments: confutil.IntMin(conf.BIP44HardenedSegments, 0, *pldconf.KeyDerivationDefaults.BIP44HardenedSegments),
	}
	seedKeyPath := pldconf.KeyDerivationDefaults.SeedKeyPath
	if conf.SeedKeyPath.Name != "" || conf.SeedKeyPath.KeyHandle != "" || len(conf.SeedKeyPath.Attributes) > 0 {
		seedKeyPath = conf.SeedKeyPath
	}
	// Note we don't have any way to store the resolved keyHandle, so we resolve it every time we start
	var seed []byte
	keyHandle, seedResolve := configToKeyResolutionRequest(&seedKeyPath)
	if keyHandle != "" {
		// We have been provided a pre-resolved key handle
		seed, err = sm.keyStore.LoadKeyMaterial(ctx, keyHandle)
	} else {
		// We need to call resolve to resolve the key material
		seed, _, err = sm.keyStore.FindOrCreateLoadableKey(ctx, seedResolve, sm.new32ByteRandomSeed)
	}
	if err != nil {
		return err
	}
	// Now we might have a 32byte value, or something like a BIP-39 mnemonic that has been saved
	// by a human/automation into a secrets repository
	if len(seed) != 32 {
		seed, err = bip39.NewSeedWithErrorChecking(string(seed), "")
		if err != nil {
			return i18n.NewError(ctx, pldmsgs.MsgSigningHDSeedMustBe32BytesOrMnemonic)
		}
	}
	sm.hd.hdKeyChain, err = hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	return err
}

func (sm *signingModule[C]) new32ByteRandomSeed() ([]byte, error) {
	buff := make([]byte, 32)
	_, err := rand.Read(buff)
	return buff, err
}

func (hd *hdDerivation[C]) flatPathList(req *prototk.ResolveKeyRequest) []hdWalletPathEntry {
	ret := make([]hdWalletPathEntry, len(req.Path)+1)
	for i, p := range req.Path {
		ret[i] = hdWalletPathEntry{Name: p.Name, Index: p.Index}
	}
	ret[len(req.Path)] = hdWalletPathEntry{
		Name:  req.Name,
		Index: req.Index,
	}
	return ret
}

func (hd *hdDerivation[C]) resolveHDWalletKey(ctx context.Context, req *prototk.ResolveKeyRequest) (res *prototk.ResolveKeyResponse, err error) {
	keyHandle := hd.bip44Prefix
	for i, s := range hd.flatPathList(req) {
		var derivation uint64
		hardenedFlag := ""
		// We must only use the config to set whether direct derivation is used, otherwise it
		// would be possible on the API to coerce two resolutions that result in the same
		// derivation path.
		// Paladin would catch this and error, but it would still break the application that
		// hit the situation.
		//
		// So if a use case requires two different behaviors, backed by the same seed, it will be
		// necessary to configure two signing modules with different BIP44DirectResolution settings,
		// and different BIP44Prefix settings, but with the same Seed.
		if hd.bip44DirectResolution {
			// We will process the NAME as a BIP44 segment spec string directly
			numStr, isHardened := strings.CutSuffix(s.Name, "'")
			ui64, err := strconv.ParseUint(numStr, 10, 64) // we use 64 bits here, but loadHDWalletPrivateKey will handle an overflow
			if err != nil {
				return nil, i18n.NewError(ctx, pldmsgs.MsgSignerBIP44DerivationInvalid, s.Name)
			}
			if isHardened {
				hardenedFlag = "'"
			}
			derivation = ui64
		} else {
			// Otherwise we use the Paladin generated index as our derivation path, which is
			// assured to be both numeric and unique.
			//
			// Handle whether the child keys will be placed in the hardened range (indices 2^31 through 2^32-1)
			// or normal range (0 through 2^31-1) using a combination of our configuration and
			// and an option that can be specified dynamically when creating the key.
			if i < hd.bip44HardenedSegments {
				hardenedFlag = "'"
			}
			derivation = s.Index
		}
		keyHandle += fmt.Sprintf("/%d%s", derivation, hardenedFlag)
	}
	privateKey, err := hd.loadHDWalletPrivateKey(ctx, keyHandle)
	if err != nil {
		return nil, err
	}
	// Once we've used key derivation, we've just got a 32byte private key in volatile memory,
	// from the perspective of the rest of the signer module.
	return hd.sm.buildResolveResponseWithIdentifiers(ctx, keyHandle, privateKey, req.RequiredIdentifiers)
}

func (hd *hdDerivation[C]) loadHDWalletPrivateKey(ctx context.Context, keyHandle string) (privateKey []byte, err error) {
	segments := strings.Split(keyHandle, "/")
	if len(segments) < 2 || segments[0] != "m" {
		return nil, i18n.NewError(ctx, pldmsgs.MsgSignerBIP44DerivationInvalid, keyHandle)
	}
	pos := hd.hdKeyChain
	for _, s := range segments[1:] {
		number, isHardened := strings.CutSuffix(s, "'")
		derivation, err := strconv.ParseUint(number, 10, 64) // we use 64bits up until the logic below
		if err == nil {
			if derivation >= 0x80000000 {
				return nil, i18n.WrapError(ctx, err, pldmsgs.MsgSignerBIP32DerivationTooLarge, derivation)
			}
			if isHardened {
				derivation += 0x80000000
			}
			pos, err = pos.Derive(uint32(derivation))
			if err != nil {
				return nil, i18n.WrapError(ctx, err, pldmsgs.MsgSignerBIP44DerivationInvalid, s)
			}
		}
		if err != nil {
			return nil, i18n.WrapError(ctx, err, pldmsgs.MsgSignerBIP44DerivationInvalid, s)
		}
	}
	ecPrivKey, err := pos.ECPrivKey()
	if err == nil {
		pkBytes := ecPrivKey.Key.Bytes()
		privateKey = pkBytes[:]
	}
	return privateKey, err
}

func (hd *hdDerivation[C]) signHDWalletKey(ctx context.Context, req *prototk.SignWithKeyRequest) (res *prototk.SignWithKeyResponse, err error) {
	privateKey, err := hd.loadHDWalletPrivateKey(ctx, req.KeyHandle)
	if err != nil {
		return nil, err
	}
	return hd.sm.signInMemory(ctx, req.Algorithm, req.PayloadType, privateKey, req.Payload)
}
