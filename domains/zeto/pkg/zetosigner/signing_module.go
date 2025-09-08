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

package zetosigner

import (
	"context"

	zetosigner "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
)

// This is a direct implementation of the extension point for signing in the Paladin signing toolkit.
// In a standalone process, this allows signing to be performed directly (separate from the Paladin runtime).
//
// TODO: As part of the Paladin domain inside of the Paladin runtime, this can be enabled as a domain
//
//	signer to call over to the Zeto domain over gRPC and request signing within the Paladin process itself.
func NewZetoSignerFactory() signerapi.InMemorySignerFactory[*zetosignerapi.SnarkProverConfig] {
	return &zetoSignerFactory{}
}

// A domain router that only knows about zeto (if that's the only domain you want to manage in your remote code deployment)
func NewZetoOnlyDomainRouter() signerapi.InMemorySignerFactory[*zetosignerapi.SnarkProverConfig] {
	return signer.NewDomainPrefixRouter(map[string]signerapi.InMemorySignerFactory[*zetosignerapi.SnarkProverConfig]{
		"zeto": NewZetoSignerFactory(),
	})
}

type zetoSignerFactory struct{}

func (zsf *zetoSignerFactory) NewSigner(ctx context.Context, conf *zetosignerapi.SnarkProverConfig) (signerapi.InMemorySigner, error) {
	return zetosigner.NewSnarkProver(conf)
}
