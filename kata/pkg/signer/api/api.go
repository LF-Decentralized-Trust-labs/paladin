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

package api

import (
	"context"

	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

// All cryptographic storage needs to support master key encryption, by which the bytes
// can be decrypted an loaded into volatile memory for use, and then discarded.
//
// The implementation is not required to know how to generate or validate such data, just now
// to securely store and retrieve it using only the information contained in the returned
// keyHandle. If the implementation finds it does not exist, it can invoke the callback function to generate
// a new suitable random string to encrypt and store.
type KeyStore interface {
	FindOrCreateLoadableKey(ctx context.Context, req *proto.ResolveKeyRequest, newKeyMaterial func() ([]byte, error)) (keyMaterial []byte, keyHandle string, err error)
	LoadKeyMaterial(ctx context.Context, keyHandle string) ([]byte, error)
}

type Extension interface {
	// Return nil if keystore type is not known, or error if initialization fails
	KeyStore(ctx context.Context, config *StoreConfig) (store KeyStore, err error)
}
