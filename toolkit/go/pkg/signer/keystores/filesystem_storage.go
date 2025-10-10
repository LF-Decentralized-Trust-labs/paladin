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
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/hyperledger/firefly-signer/pkg/keystorev3"
)

type filesystemStoreFactory[C signerapi.ExtensibleConfig] struct{}

type filesystemStore struct {
	cache    cache.Cache[string, keystorev3.WalletFile]
	path     string
	fileMode os.FileMode
	dirMode  os.FileMode
}

func NewFilesystemStoreFactory[C signerapi.ExtensibleConfig]() signerapi.KeyStoreFactory[C] {
	return &filesystemStoreFactory[C]{}
}

func (fsf *filesystemStoreFactory[C]) NewKeyStore(ctx context.Context, eConf C) (fss signerapi.KeyStore, err error) {
	conf := &eConf.KeyStoreConfig().FileSystem

	// Determine the path
	var pathInfo fs.FileInfo
	path, err := filepath.Abs(confutil.StringNotEmpty(conf.Path, *pldconf.FileSystemDefaults.Path))
	if err == nil {
		pathInfo, err = os.Stat(path)
	}
	if err != nil || !pathInfo.IsDir() {
		return nil, i18n.WrapError(ctx, err, pldmsgs.MsgSigningModuleBadPathError, *pldconf.FileSystemDefaults.Path)
	}
	return &filesystemStore{
		cache:    cache.NewCache[string, keystorev3.WalletFile](&conf.Cache, &pldconf.FileSystemDefaults.Cache),
		fileMode: confutil.UnixFileMode(conf.FileMode, *pldconf.FileSystemDefaults.FileMode),
		dirMode:  confutil.UnixFileMode(conf.DirMode, *pldconf.FileSystemDefaults.DirMode),
		path:     path,
	}, nil
}

func (fss *filesystemStore) validateFilePathKeyHandle(ctx context.Context, keyHandle string, forCreate bool) (absPath string, err error) {

	fullPath := fss.path
	segments := strings.Split(keyHandle, "/")
	for i, segment := range segments {
		isDir := i < (len(segments) - 1)

		// We use a file-or-directory prefix for two reasons:
		// - To avoid filesystem clashes between "something.key/another" and "something"
		// - Belt an braces to ensure we never use a ".anything" path segment
		if isDir {
			segment = "_" + segment
		} else {
			segment = "-" + segment
		}

		fullPath = path.Join(fullPath, segment)
		if forCreate {
			fsInfo, err := os.Stat(fullPath)
			if os.IsNotExist(err) {
				err = nil
				if isDir {
					err = os.Mkdir(fullPath, fss.dirMode)
				}
			} else {
				if (!isDir && fsInfo.IsDir()) || (isDir && !fsInfo.IsDir()) {
					err = i18n.NewError(ctx, pldmsgs.MsgSigningModuleKeyHandleClash)
				}
			}
			if err != nil {
				return "", err
			}
		}
	}
	return fullPath, nil

}

func (fss *filesystemStore) createWalletFile(ctx context.Context, keyFilePath, passwordFilePath string, newKeyMaterial func() ([]byte, error)) (keystorev3.WalletFile, error) {

	privateKey, err := newKeyMaterial()
	if err != nil {
		return nil, err
	}
	password := pldtypes.RandHex(32)
	wf := keystorev3.NewWalletFileCustomBytesStandard(password, privateKey)

	// Address is not part of the V3 standard, per
	// https://github.com/ethereum/wiki/wiki/Web3-Secret-Storage-Definition#alterations-from-version-1
	//
	// It's also very misleading in Paladin, as there's no assurance the private key material we're storing in the file
	// will be used for SECP256K1 cryptography (BabyJubJub being an example) - or even that it's 32bytes in length
	// (BIP39 mnemonics being a simple example).
	//
	// So we use the feature from https://github.com/hyperledger/firefly-signer/pull/70 to remove it entirely
	wf.Metadata()["address"] = nil

	err = os.WriteFile(passwordFilePath, []byte(password), fss.fileMode)
	if err == nil {
		err = os.WriteFile(keyFilePath, wf.JSON(), fss.fileMode)
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, pldmsgs.MsgSigningModuleFSError)
	}
	return wf, nil
}

func (fss *filesystemStore) getOrCreateWalletFile(ctx context.Context, keyHandle string, newKeyMaterialFactory func() ([]byte, error)) (keystorev3.WalletFile, error) {

	absPathPrefix, err := fss.validateFilePathKeyHandle(ctx, keyHandle, newKeyMaterialFactory != nil)
	if err != nil {
		return nil, err
	}

	cached, _ := fss.cache.Get(keyHandle)
	if cached != nil {
		return cached, nil
	}
	keyFilePath := fmt.Sprintf("%s.key", absPathPrefix)
	passwordFilePath := fmt.Sprintf("%s.pwd", absPathPrefix)

	_, checkNotExist := os.Stat(keyFilePath)
	if os.IsNotExist(checkNotExist) {
		if newKeyMaterialFactory != nil {
			// We need to create it
			wf, err := fss.createWalletFile(ctx, keyFilePath, passwordFilePath, newKeyMaterialFactory)
			if err == nil {
				fss.cache.Set(keyHandle, wf)
			}
			return wf, err
		} else {
			return nil, i18n.NewError(ctx, pldmsgs.MsgSigningModuleKeyNotExist, keyHandle)
		}
	}
	// we need to read it
	wf, err := fss.readWalletFile(ctx, keyFilePath, passwordFilePath)
	if err == nil {
		fss.cache.Set(keyHandle, wf)
	}
	return wf, err
}

func (fss *filesystemStore) readWalletFile(ctx context.Context, keyFilePath, passwordFilePath string) (keystorev3.WalletFile, error) {

	keyData, err := os.ReadFile(keyFilePath)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, pldmsgs.MsgSigningModuleBadKeyFile, keyFilePath)
	}

	passData, err := os.ReadFile(passwordFilePath)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, pldmsgs.MsgSigningModuleBadPassFile, passwordFilePath)
	}

	return keystorev3.ReadWalletFile(keyData, passData)
}

func (fss *filesystemStore) FindOrCreateLoadableKey(ctx context.Context, req *prototk.ResolveKeyRequest, newKeyMaterial func() ([]byte, error)) (keyMaterial []byte, keyHandle string, err error) {
	for _, segment := range req.Path {
		if len(segment.Name) == 0 {
			return nil, "", i18n.NewError(ctx, pldmsgs.MsgSigningModuleBadKeyHandle)
		}
		keyHandle += url.PathEscape(segment.Name)
		keyHandle += "/"
	}
	if len(req.Name) == 0 {
		return nil, "", i18n.NewError(ctx, pldmsgs.MsgSigningModuleBadKeyHandle)
	}
	keyHandle += url.PathEscape(req.Name)
	wf, err := fss.getOrCreateWalletFile(ctx, keyHandle, newKeyMaterial)
	if err != nil {
		return nil, "", err
	}
	return wf.PrivateKey(), keyHandle, nil
}

func (fss *filesystemStore) LoadKeyMaterial(ctx context.Context, keyHandle string) ([]byte, error) {
	wf, err := fss.getOrCreateWalletFile(ctx, keyHandle, nil)
	if err != nil {
		return nil, err
	}
	return wf.PrivateKey(), nil
}

func (fss *filesystemStore) Close() {

}
