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

package ethclient

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
)

type simpleKeyManager struct {
	signer     signer.SigningModule
	lock       sync.Mutex
	rootFolder *keyFolder
}

type keyFolder struct {
	Name     string
	Index    uint64
	Children uint64
	Keys     map[string]*keyMapping
	Folders  map[string]*keyFolder
}

type keyMapping struct {
	Name        string
	Index       uint64
	KeyHandle   string
	Identifiers map[string]string
}

// Super simple in-memory placeholder for Key Manager, which wraps a single signer, and does not
// have any persistence of the folders and key mappings that are created.
// TODO: Supersede with full key manager once it is in place
func NewSimpleTestKeyManager(ctx context.Context, signerConfig *signerapi.ConfigNoExt, extensions ...*signerapi.Extensions[*signerapi.ConfigNoExt]) (KeyManager, error) {
	signer, err := signer.NewSigningModule(ctx, signerConfig, extensions...)
	if err != nil {
		return nil, err
	}
	return &simpleKeyManager{
		signer:     signer,
		rootFolder: &keyFolder{},
	}, nil
}

func (km *simpleKeyManager) AddInMemorySigner(prefix string, signer signerapi.InMemorySigner) {
	km.signer.AddInMemorySigner(prefix, signer)
}

func (km *simpleKeyManager) ResolveKey(ctx context.Context, identifier, algorithm, verifierType string) (keyHandle, verifier string, err error) {
	km.lock.Lock()
	defer km.lock.Unlock()

	resolveRequest := &prototk.ResolveKeyRequest{
		Attributes: make(map[string]string),
		RequiredIdentifiers: []*prototk.PublicKeyIdentifierType{
			{
				Algorithm:    algorithm,
				VerifierType: verifierType,
			},
		},
	}
	loc := km.rootFolder
	segments := strings.Split(identifier, "/")
	for i := 0; i < len(segments)-1; i++ {
		folderName := segments[i]
		if loc.Folders == nil {
			loc.Folders = make(map[string]*keyFolder)
		}
		folder := loc.Folders[folderName]
		if folder == nil {
			folder = &keyFolder{
				Name:  folderName,
				Index: loc.Children,
			}
			loc.Folders[folderName] = folder
			loc.Children++ // increment for folders optimistically (and keys pessimistically below)
		}
		loc = folder
		resolveRequest.Path = append(resolveRequest.Path, &prototk.ResolveKeyPathSegment{
			Name:  folder.Name,
			Index: folder.Index,
		})
	}
	keyName := segments[len(segments)-1]
	if loc.Keys == nil {
		loc.Keys = make(map[string]*keyMapping)
	}
	key := loc.Keys[keyName]
	algoAndVerifierType := strings.ToLower(fmt.Sprintf("%s/%s", algorithm, verifierType))
	if key == nil || key.Identifiers[algoAndVerifierType] == "" {
		resolveRequest.Name = keyName
		// resolve either a new key, or a new identifier for an existing key
		if key == nil {
			resolveRequest.Index = loc.Children
		} else {
			resolveRequest.Index = key.Index
		}
		resolved, err := km.signer.Resolve(ctx, resolveRequest)
		if err != nil {
			return "", "", err
		}
		// ok - we're good - update our record
		if key == nil {
			key = &keyMapping{
				Name:        keyName,
				Index:       loc.Children,
				KeyHandle:   resolved.KeyHandle,
				Identifiers: make(map[string]string),
			}
			// we're now ready to take the count from the parent
			loc.Children++
			loc.Keys[key.Name] = key
		} else if resolved.KeyHandle != key.KeyHandle {
			return "", "", i18n.NewError(ctx, msgs.MsgEthClientKeyMismatch, identifier, key.KeyHandle, resolved.KeyHandle)
		}
		for _, v := range resolved.Identifiers {
			key.Identifiers[strings.ToLower(fmt.Sprintf("%s/%s", v.Algorithm, v.VerifierType))] = v.Verifier
		}
	}
	// Double check we have the identifier we need
	return key.KeyHandle, key.Identifiers[algoAndVerifierType], nil
}

func (km *simpleKeyManager) Sign(ctx context.Context, req *prototk.SignWithKeyRequest) (res *prototk.SignWithKeyResponse, err error) {
	return km.signer.Sign(ctx, req)
}

func (km *simpleKeyManager) Close() {
	km.signer.Close()
}
