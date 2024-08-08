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

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

type testbedKeyFolder struct {
	Name     string                       `json:"name,omitempty"`
	Index    uint64                       `json:"index"`
	Children uint64                       `json:"children"`
	Keys     map[string]*testbedKey       `json:"keys,omitempty"`
	Folders  map[string]*testbedKeyFolder `json:"folders,omitempty"`
}

type testbedKey struct {
	Name        string            `json:"name"`
	Index       uint64            `json:"index"`
	KeyHandle   string            `json:"keyHandle"`
	Identifiers map[string]string `json:"identifiers"`
}

// Super simple in-memory placeholder for Key Manager
func (tb *testbed) resolveKey(ctx context.Context, identifier string, algorithm string) (keyHandle, verifier string, err error) {
	tb.keyLock.Lock()
	defer tb.keyLock.Unlock()

	resolvePath := []*proto.KeyPathSegment{}
	loc := tb.keyMap
	segments := strings.Split(identifier, "/")
	for i := 0; i < len(segments)-1; i++ {
		folderName := segments[i]
		if loc.Folders == nil {
			loc.Folders = make(map[string]*testbedKeyFolder)
		}
		folder := loc.Folders[folderName]
		if folder == nil {
			folder = &testbedKeyFolder{
				Name:  folderName,
				Index: loc.Children,
			}
			loc.Folders[folderName] = folder
			loc.Children++ // increment for folders optimistically (and keys pessimistically below)
		}
		loc = folder
		resolvePath = append(resolvePath, &proto.KeyPathSegment{
			Name:       folder.Name,
			Index:      folder.Index,
			Attributes: make(map[string]string), // none in testbed
		})
	}
	keyName := segments[len(segments)-1]
	if loc.Keys == nil {
		loc.Keys = make(map[string]*testbedKey)
	}
	key := loc.Keys[keyName]
	if key == nil || key.Identifiers[algorithm] == "" {
		// resolve either a new key, or a new identifier for an existing key
		resolvePath = append(resolvePath, &proto.KeyPathSegment{
			Name:       keyName,
			Index:      loc.Children,
			Attributes: make(map[string]string), // none in testbed
		})
		resolved, err := tb.signer.Resolve(ctx, &proto.ResolveKeyRequest{
			Algorithms: []string{algorithm},
			Path:       resolvePath,
		})
		if err != nil {
			return "", "", err
		}
		// ok - we're good - update our record
		if key == nil {
			key = &testbedKey{
				Name:        keyName,
				Index:       loc.Children,
				KeyHandle:   resolved.KeyHandle,
				Identifiers: make(map[string]string),
			}
			// we're now ready to take the count from the parent
			loc.Children++
			loc.Keys[key.Name] = key
		} else if resolved.KeyHandle != key.KeyHandle {
			return "", "", fmt.Errorf("resolved %q to different key handle expected=%q received=%q", identifier, key.KeyHandle, resolved.KeyHandle)
		}
		for _, v := range resolved.Identifiers {
			key.Identifiers[v.Algorithm] = v.Identifier
		}
	}
	// Double check we have the identifier we need
	verifier = key.Identifiers[algorithm]
	if verifier == "" {
		return "", "", fmt.Errorf("key verifier not established for algorithm %s", algorithm)
	}
	return key.KeyHandle, verifier, nil
}

func (tb *testbed) keystoreInfo() []*testbedKey {
	tb.keyLock.Lock()
	defer tb.keyLock.Unlock()
	return tb.recurseKeystoreInfo(tb.keyMap, "")
}

func (tb *testbed) recurseKeystoreInfo(loc *testbedKeyFolder, prefix string) []*testbedKey {
	var keys []*testbedKey
	keyNames := make([]string, 0, len(loc.Keys))
	for name := range loc.Keys {
		keyNames = append(keyNames, name)
	}
	for _, name := range keyNames {
		keyCopy := *loc.Keys[name]
		keyCopy.Name = prefix + keyCopy.Name
		keys = append(keys, &keyCopy)
	}
	folderNames := make([]string, 0, len(loc.Folders))
	for name := range loc.Folders {
		folderNames = append(folderNames, name)
	}
	for _, name := range folderNames {
		folder := loc.Folders[name]
		childKeys := tb.recurseKeystoreInfo(folder, prefix+folder.Name+"/")
		keys = append(keys, childKeys...)
	}
	return keys
}
