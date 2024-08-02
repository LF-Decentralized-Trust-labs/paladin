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
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/tyler-smith/go-bip39"
)

var (
	BIP32BaseDerivationPath = []uint32{0x80000000 + 44, 0x80000000 + 60}
)

func (sm *signingModule) loadHDWalletSeed(ctx context.Context, conf *KeyDerivationConfig) (err error) {
	seedKeyPath := KeyDerivationDefaults.SeedKeyPath
	if len(conf.SeedKeyPath) > 0 {
		seedKeyPath = conf.SeedKeyPath
	}
	seedReq := &proto.ResolveKeyRequest{}
	for _, p := range seedKeyPath {
		seedReq.Path = append(seedReq.Path, &proto.KeyPathSegment{
			Name:       p.Name,
			Index:      p.Index,
			Attributes: p.Attributes,
		})
	}
	// Note we don't have any way to store the resolved keyHandle, so we resolve it every time we start
	seed, _, err := sm.keyStore.FindOrCreateLoadableKey(ctx, seedReq, sm.new32ByteRandom)
	if err != nil {
		return err
	}
	// Now we might have a 32byte value, or something like a BIP-39 mnemonic that has been saved
	// by a human/automation into a secrets repository
	if len(seed) != 32 {
		seed, err = bip39.NewSeedWithErrorChecking(string(seed), "")
		if err != nil {
			return i18n.NewError(ctx, msgs.MsgHDSeedMustBe32BytesOrMnemonic)
		}
	}
	sm.hdKeyChain, err = hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	return err
}

func (sm *signingModule) resolveHDWalletKey(ctx context.Context, req *proto.ResolveKeyRequest) (res *proto.ResolveKeyResponse, err error) {
	keyHandle := "m/44'/0'"
	for i, s := range req.Path {
		hardenedTag := ""
		hardenedAttr := s.Attributes["bip32_hardened"]
		if i == 0 && s.Attributes["bip32_root"] == "true" {
			// Clear the BIP44 standard m/44'/0' prefix
			// Only applies on root key/folder
			keyHandle = "m"
		}
		if i == 0 && hardenedAttr != "false" || hardenedAttr == "true" {
			hardenedTag = "'"
		}
		keyHandle += fmt.Sprintf("/%d%s", s.Index, hardenedTag)
	}
	privateKey, err := sm.loadHDWalletPrivateKey(ctx, keyHandle)
	if err != nil {
		return nil, err
	}
	return sm.publicKeyIdentifiersForAlgorithms(ctx, keyHandle, privateKey, req.Algorithms)
}

func (sm *signingModule) loadHDWalletPrivateKey(ctx context.Context, keyHandle string) (privateKey []byte, err error) {
	segments := strings.Split(keyHandle, "/")
	if len(segments) < 2 || segments[0] != "m" {
		return nil, i18n.NewError(ctx, msgs.MsgSigningModuleBadKeyFile)
	}
	pos := sm.hdKeyChain
	for _, s := range segments[1:] {
		number, isHardened := strings.CutSuffix(s, "'")
		derivation, err := strconv.ParseUint(number, 10, 32)
		if err == nil {
			if isHardened {
				derivation += 0x80000000
			}
			pos, err = pos.Derive(uint32(derivation))
		}
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgSigningModuleBadKeyFile)
		}
	}
	ecPrivKey, err := pos.ECPrivKey()
	if err == nil {
		privateKey = ecPrivKey.Serialize()
	}
	return privateKey, err
}
