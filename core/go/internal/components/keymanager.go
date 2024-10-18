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

package components

import (
	"context"

	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

type KeyResolutionContext interface {
	KeyResolver(dbTX *gorm.DB) KeyResolver // Defers passing the DB TX in until it's begun
	PreCommit() error                      // MUST be called for successful TX inside the DB TX
	Close(committed bool)                  // MUST be called outside the DB TX
}
type KeyResolutionContextLazyDB interface {
	KeyResolverLazyDB() KeyResolver // Defers starting the DB TX in until it's begun
	Commit() error
	Rollback()
}

type KeyResolver interface {
	ResolveKey(identifier, algorithm, verifierType string) (mapping *pldapi.KeyMappingAndVerifier, err error)
}

type KeyManager interface {
	ManagerLifecycle

	// Note resolving a key is a persistent activity that requires a database transaction to be managed by the caller.
	// To avoid deadlock when resolving multiple keys in the same DB transaction, the caller is responsible for using the same
	// resolution context for all calls that occur within the same DB tx.
	NewKeyResolutionContext(ctx context.Context) KeyResolutionContext

	// Key resolution context where we sort the database transaction for you
	NewKeyResolutionContextLazyDB(ctx context.Context) KeyResolutionContextLazyDB

	// Convenience function in code where there isn't already a database transaction, and we're happy to create a
	// new one just to scope the lookup (cannot be called safely within a containing DB transaction)
	ResolveKeyNewDatabaseTX(ctx context.Context, identifier, algorithm, verifierType string) (resolvedKey *pldapi.KeyMappingAndVerifier, err error)

	// Convenience to resolve a whole set in one new DB transaction
	ResolveBatchNewDatabaseTX(ctx context.Context, algorithm, verifierType string, identifiers []string) (resolvedKey []*pldapi.KeyMappingAndVerifier, err error)

	// Convenience when all you want is the EthAddress, and to know the reverse lookup will later be possible
	ResolveEthAddressNewDatabaseTX(ctx context.Context, identifier string) (ethAddress *tktypes.EthAddress, err error)

	// Convenience when all you want is the EthAddress, and to know the reverse lookup will later be possible
	ResolveEthAddressBatchNewDatabaseTX(ctx context.Context, identifiers []string) (ethAddresses []*tktypes.EthAddress, err error)

	// Domains register their signers during PostCommit
	AddInMemorySigner(prefix string, signer signerapi.InMemorySigner)

	ReverseKeyLookup(ctx context.Context, dbTX *gorm.DB, algorithm, verifierType, verifier string) (mapping *pldapi.KeyMappingAndVerifier, err error)

	Sign(ctx context.Context, mapping *pldapi.KeyMappingAndVerifier, payloadType string, payload []byte) ([]byte, error)
}
