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

package signerapi

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

type KeyStoreFactory[C ExtensibleConfig] interface {
	NewKeyStore(ctx context.Context, conf C) (KeyStore, error)
}

// All cryptographic storage needs to support master key encryption, by which the bytes
// can be decrypted and loaded into volatile memory for use, and then discarded.
//
// The implementation is not required to know how to generate or validate such data, just how
// to securely store and retrieve it using only the information contained in the returned
// keyHandle. If the implementation finds it does not exist, it can invoke the callback function to generate
// a new suitable random string to encrypt and store.
type KeyStore interface {
	FindOrCreateLoadableKey(ctx context.Context, req *prototk.ResolveKeyRequest, newKeyMaterial func() ([]byte, error)) (keyMaterial []byte, keyHandle string, err error)
	LoadKeyMaterial(ctx context.Context, keyHandle string) ([]byte, error)
	Close()
}

// Some cryptographic stores are capable of listing their contents in a natural order.
//
// It is a friendly behavior particularly at development/exploration time to be able to present
// these keys back as key mappings automatically, simply picking a name for them that
// is intuitive based on the store in the backend.
//
// The backend store is not responsible for any fancy query/sort capabilities - as the listing
// is only used to build the key mapping entries into Paladin.
// The only requirements are:
// 1) that there is a natural order
// 2) that there is a finite list (thus this is NOT supported by the HD Wallet derivation scheme if used in the signing module on top of a key store)
// 3) that when presented with the nextPtr from the last call, the listing can continue to list keys after that point (according to the natural order)
//
// This behavior can be explicitly disabled in the configuration for any store type.
type KeyStoreListable interface {
	ListKeys(ctx context.Context, req *prototk.ListKeysRequest) (res *prototk.ListKeysResponse, err error)
}

// Some cryptographic storage systems, in particular Hardware Security Modules (HSMs) and Cloud HSM systems,
// support signing directly with certain curves.
//
// This is an advanced feature of key store implementations, and hence kept on a separate interface.
//
// Configuring a signing module in this way provides enhanced security. However, it disables use of any
// of the in-memory signing capabilities of the signing module - such as ZKP proof generation using in-memory keys.
//
// Note that even HSM based key storage systems support storage and retrieval of a key (by encrypting and
// decrypting that key on storage and retrieval). So a signing-capable system (such as a HSM or Cloud HSM)
// might still be used with this in-memory module by installing the code in this repo close to the HSM.
//
// See the Paladin architecture docs for more details.
type KeyStoreSigner interface {
	FindOrCreateInStoreSigningKey(ctx context.Context, req *prototk.ResolveKeyRequest) (res *prototk.ResolveKeyResponse, err error)
	SignWithinKeystore(ctx context.Context, req *prototk.SignWithKeyRequest) (res *prototk.SignWithKeyResponse, err error)
}
