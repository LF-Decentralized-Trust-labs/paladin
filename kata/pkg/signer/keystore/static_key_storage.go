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

package keystore

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"os"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type staticStore struct {
	keys map[string][]byte
}

// This is a trivial implementation of a keystore, that loads unencrypted keys
// Supports the following encodings:
// 1. None (the string is the key)
// 2. Hex
// 3. Base64
//
// Note that special characters in key names must be URL path encoded in the
// YAML keys, and "/" characters (rather than object nesting) is used
// in the YAML config.
//
// The keys themselves can be in files, so as well as very simple testing
// with keys in-line in the config, this helps use a file based Kubernetes
// secret for a mnemonic seed phrase for example at the root of a HD wallet.
func NewStaticKeyStore(ctx context.Context, conf api.StaticKeyStorageConfig) (_ api.KeyStore, err error) {
	ils := &staticStore{
		keys: make(map[string][]byte),
	}
	for keyHandle, keyEntry := range conf.Keys {
		var keyData []byte
		if keyEntry.Filename != "" {
			if keyData, err = os.ReadFile(string(keyEntry.Filename)); err != nil {
				log.L(ctx).Errorf("Failed to load file %s: %s", keyEntry.Filename, err)
				return nil, i18n.NewError(ctx, msgs.MsgSigningStaticKeyInvalid, keyHandle)
			}
		} else if keyEntry.Inline != "" {
			keyData = []byte(keyEntry.Inline)
		}
		if len(keyData) > 0 && keyEntry.Trim {
			keyData = ([]byte)(strings.TrimSpace((string)(keyData)))
		}
		// If we didn't get either, or what we did get is zero length - we fail startup
		if len(keyData) == 0 {
			return nil, i18n.NewError(ctx, msgs.MsgSigningStaticKeyInvalid, keyHandle)
		}
		switch keyEntry.Encoding {
		case api.StaticKeyEntryEncodingNONE:
		case api.StaticKeyEntryEncodingHEX:
			if keyData, err = hex.DecodeString(strings.TrimPrefix(string(keyData), "0x")); err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgSigningStaticKeyInvalid, keyHandle)
			}
		case api.StaticKeyEntryEncodingBase64:
			if keyData, err = base64.StdEncoding.DecodeString(string(keyData)); err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgSigningStaticKeyInvalid, keyHandle)
			}
		default:
			return nil, i18n.NewError(ctx, msgs.MsgSigningStaticBadEncoding, keyHandle, keyEntry.Encoding)
		}
		ils.keys[keyHandle] = keyData
	}
	return ils, nil
}

func (ils *staticStore) FindOrCreateLoadableKey(ctx context.Context, req *proto.ResolveKeyRequest, newKeyMaterial func() ([]byte, error)) (keyMaterial []byte, keyHandle string, err error) {
	for _, segment := range req.Path {
		if len(segment.Name) == 0 {
			return nil, "", i18n.NewError(ctx, msgs.MsgSigningModuleBadKeyHandle)
		}
		keyHandle += url.PathEscape(segment.Name)
		keyHandle += "/"
	}
	if len(req.Name) == 0 {
		return nil, "", i18n.NewError(ctx, msgs.MsgSigningModuleBadKeyHandle)
	}
	keyHandle += url.PathEscape(req.Name)
	key, err := ils.LoadKeyMaterial(ctx, keyHandle)
	if err != nil {
		return nil, "", err
	}
	return key, keyHandle, nil
}

func (ils *staticStore) LoadKeyMaterial(ctx context.Context, keyHandle string) ([]byte, error) {
	log.L(ctx).Debugf("Resolving key %s", keyHandle)
	key, ok := ils.keys[keyHandle]
	if !ok {
		return nil, i18n.NewError(ctx, msgs.MsgSigningKeyCannotBeResolved)
	}
	return key, nil
}

func (ils *staticStore) Close() {

}
