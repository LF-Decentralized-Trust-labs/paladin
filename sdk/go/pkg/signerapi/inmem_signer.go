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

package signerapi

import (
	"context"
)

type InMemorySignerFactory[C ExtensibleConfig] interface {
	NewSigner(ctx context.Context, conf C) (InMemorySigner, error)
}

type InMemorySigner interface {
	// Perform signing using the specified algorithm, with the specified private key
	Sign(ctx context.Context, algorithm, payloadType string, privateKey, payload []byte) ([]byte, error)
	// Translate a signing key into a verifier of the requested type
	GetVerifier(ctx context.Context, algorithm, verifierType string, privateKey []byte) (string, error)
	// Get the minimum key length required for the supplied algorithm
	// The key will be created and managed on behalf of the in memory signing using the configured key store
	GetMinimumKeyLen(ctx context.Context, algorithm string) (int, error)
}
