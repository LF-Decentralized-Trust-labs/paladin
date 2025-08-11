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

package keystores

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
)

type staticStoreFactory[C signerapi.ExtensibleConfig] struct{}

type staticStore struct {
	keys map[string][]byte
}

func NewStaticStoreFactory[C signerapi.ExtensibleConfig]() signerapi.KeyStoreFactory[C] {
	return &staticStoreFactory[C]{}
}

// This is a trivial implementation of a keystore, that loads unencrypted keys
// Supports the following encodings:
// 1. None (the string is the key)
// 2. Hex
// 3. Base64
//
// Note that special characters in key names must be URL path encoded in the
// YAML keys, and "/" characters (rather than object nesting) is used
// in the YAML pldconf.
//
// The keys themselves can be in files, so as well as very simple testing
// with keys in-line in the config, this helps use a file based Kubernetes
// secret for a mnemonic seed phrase for example at the root of a HD wallet.
func (fsf *staticStoreFactory[C]) NewKeyStore(ctx context.Context, eConf C) (_ signerapi.KeyStore, err error) {
	conf := &eConf.KeyStoreConfig().Static

	keyMap := conf.Keys
	if keyMap == nil {
		keyMap = make(map[string]pldconf.StaticKeyEntryConfig)
	}
	ils := &staticStore{
		keys: make(map[string][]byte),
	}
	if conf.File != "" {
		if err = ils.loadFileIntoKeyMap(ctx, conf.File, keyMap); err != nil {
			return nil, err
		}
	}
	for keyHandle, keyEntry := range keyMap {
		var keyData []byte
		if keyEntry.Filename != "" {
			if keyData, err = os.ReadFile(string(keyEntry.Filename)); err != nil {
				log.L(ctx).Errorf("Failed to load file %s: %s", keyEntry.Filename, err)
				return nil, i18n.NewError(ctx, pldmsgs.MsgSigningStaticKeyInvalid, keyHandle)
			}
		} else if keyEntry.Inline != "" {
			keyData = []byte(keyEntry.Inline)
		}
		if len(keyData) > 0 && keyEntry.Trim {
			keyData = ([]byte)(strings.TrimSpace((string)(keyData)))
		}
		// If we didn't get either, or what we did get is zero length - we fail startup
		if len(keyData) == 0 {
			return nil, i18n.NewError(ctx, pldmsgs.MsgSigningStaticKeyInvalid, keyHandle)
		}
		switch keyEntry.Encoding {
		case pldconf.StaticKeyEntryEncodingNONE:
		case pldconf.StaticKeyEntryEncodingHEX:
			if keyData, err = hex.DecodeString(strings.TrimPrefix(string(keyData), "0x")); err != nil {
				return nil, i18n.NewError(ctx, pldmsgs.MsgSigningStaticKeyInvalid, keyHandle)
			}
		case pldconf.StaticKeyEntryEncodingBase64:
			if keyData, err = base64.StdEncoding.DecodeString(string(keyData)); err != nil {
				return nil, i18n.NewError(ctx, pldmsgs.MsgSigningStaticKeyInvalid, keyHandle)
			}
		default:
			return nil, i18n.NewError(ctx, pldmsgs.MsgSigningStaticBadEncoding, keyHandle, keyEntry.Encoding)
		}
		ils.keys[keyHandle] = keyData
	}
	return ils, nil
}

func (ils *staticStore) loadFileIntoKeyMap(ctx context.Context, filename string, keyMap map[string]pldconf.StaticKeyEntryConfig) error {
	var fileKeyMap map[string]pldconf.StaticKeyEntryConfig
	b, err := os.ReadFile(filename)
	if err == nil {
		err = yaml.Unmarshal(b, &fileKeyMap)
	}
	if err != nil {
		return i18n.WrapError(ctx, err, pldmsgs.MsgSigningFailedToLoadStaticKeyFile)
	}
	for k, v := range fileKeyMap {
		keyMap[k] = v
	}
	return nil
}

func (ils *staticStore) FindOrCreateLoadableKey(ctx context.Context, req *prototk.ResolveKeyRequest, newKeyMaterial func() ([]byte, error)) (keyMaterial []byte, keyHandle string, err error) {
	for _, segment := range req.Path {
		if len(segment.Name) == 0 {
			return nil, "", i18n.NewError(ctx, pldmsgs.MsgSigningModuleBadKeyHandle)
		}
		keyHandle += url.PathEscape(segment.Name)
		keyHandle += "."
	}
	if len(req.Name) == 0 {
		return nil, "", i18n.NewError(ctx, pldmsgs.MsgSigningModuleBadKeyHandle)
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
		return nil, i18n.NewError(ctx, pldmsgs.MsgSigningKeyCannotBeResolved)
	}
	return key, nil
}

func (ils *staticStore) Close() {

}
