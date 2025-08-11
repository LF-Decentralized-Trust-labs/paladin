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

package components

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/google/uuid"
)

type KeyResolver interface {
	ResolveKey(ctx context.Context, identifier, algorithm, verifierType string) (mapping *pldapi.KeyMappingAndVerifier, err error)
}

type KeyManagerToSigningModule interface {
	plugintk.SigningModuleAPI
	Initialized()
}

type KeyManager interface {
	ManagerLifecycle

	// plugin signing modules management
	ConfiguredSigningModules() map[string]*pldconf.PluginConfig
	GetSigningModule(ctx context.Context, name string) (signer.SigningModule, error)
	SigningModuleRegistered(name string, id uuid.UUID, toSigningModule KeyManagerToSigningModule) (fromSigningModule plugintk.SigningModuleCallbacks, err error)

	// Note resolving a key is a persistent activity that requires a database transaction to be managed by the caller.
	// To avoid deadlock when resolving multiple keys in the same DB transaction, the caller is responsible for using the same
	// resolution context for all calls that occur within the same DB tx.
	//
	// IMPORTANT: An attempt to use a NOTX() pseudo transaction with this call will panic
	KeyResolverForDBTX(dbTX persistence.DBTX) KeyResolver

	// Convenience function in code where there isn't already a database transaction, and we're happy to create a
	// new one just to scope the lookup (cannot be called safely within a containing DB transaction)
	ResolveKeyNewDatabaseTX(ctx context.Context, identifier, algorithm, verifierType string) (resolvedKey *pldapi.KeyMappingAndVerifier, err error)

	// Convenience to resolve a whole set in one new DB transaction
	ResolveBatchNewDatabaseTX(ctx context.Context, algorithm, verifierType string, identifiers []string) (resolvedKey []*pldapi.KeyMappingAndVerifier, err error)

	// Convenience when all you want is the EthAddress, and to know the reverse lookup will later be possible
	ResolveEthAddressNewDatabaseTX(ctx context.Context, identifier string) (ethAddress *pldtypes.EthAddress, err error)

	// Convenience when all you want is the EthAddress, and to know the reverse lookup will later be possible
	ResolveEthAddressBatchNewDatabaseTX(ctx context.Context, identifiers []string) (ethAddresses []*pldtypes.EthAddress, err error)

	// Domains register their signers during PostCommit
	AddInMemorySigner(prefix string, signer signerapi.InMemorySigner)

	ReverseKeyLookup(ctx context.Context, dbTX persistence.DBTX, algorithm, verifierType, verifier string) (mapping *pldapi.KeyMappingAndVerifier, err error)

	Sign(ctx context.Context, mapping *pldapi.KeyMappingAndVerifier, payloadType string, payload []byte) ([]byte, error)
}
