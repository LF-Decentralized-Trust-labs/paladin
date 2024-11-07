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

package keymanager

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestKRCTimeoutWaitingForLock(t *testing.T) {

	ctx, km, _, done := newTestDBKeyManagerWithWallets(t, hdWalletConfig("wallet1", ""))
	defer done()

	readyToTry := make(chan struct{})
	waitDone := make(chan struct{})
	krc1 := km.NewKeyResolutionContext(ctx)
	go func() {

		committed := false
		defer func() {
			krc1.Close(committed)
		}()
		err := km.p.DB().Transaction(func(tx *gorm.DB) error {
			mapping1, err := krc1.KeyResolver(tx).ResolveKey("key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			require.NoError(t, err)
			require.NotEmpty(t, mapping1.Verifier.Verifier)
			close(readyToTry)
			<-waitDone
			return nil
		})
		require.NoError(t, err)
		committed = true
	}()

	// Wait until we know we are blocked
	<-readyToTry
	cancelled, cancelCtx := context.WithCancel(ctx)
	cancelCtx()
	krc2 := km.NewKeyResolutionContext(cancelled)
	kr2 := krc2.KeyResolver(km.p.DB()).(*keyResolver)
	err := km.takeAllocationLock(kr2)
	assert.Regexp(t, "PD010301", err)

	close(waitDone)

	// Double unlock is a warned no-op
	km.unlockAllocation(kr2)

}

func mockSetupForUnresolvedKeyEmptyDB(mc *mockComponents) {
	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))    // on root path to see it is not resolved
	mc.db.ExpectExec("INSERT.*key_paths").WillReturnResult(sqlmock.NewResult(0, 1))       // adding the new root
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))    // on whole path to see it is not resolved
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))    // on root to find the next index to add
	mc.db.ExpectExec("INSERT.*key_paths").WillReturnResult(sqlmock.NewResult(0, 1))       // adding the new leaf
	mc.db.ExpectQuery("SELECT.*key_mappings").WillReturnRows(sqlmock.NewRows([]string{})) // checking existing key mapping
	mc.db.ExpectExec("INSERT.*key_mapping").WillReturnResult(driver.ResultNoRows)         // adding the new leaf
	mc.db.ExpectExec("INSERT.*key_verifiers").WillReturnResult(driver.ResultNoRows)       // adding the verifier
}

func TestKRCCommitError(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, false,
		&pldconf.KeyManagerConfig{Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")}},
	)
	defer done()

	mockSetupForUnresolvedKeyEmptyDB(mc)
	mc.db.ExpectCommit().WillReturnError(fmt.Errorf("pop")) // mock a commit error

	krc1 := km.NewKeyResolutionContextLazyDB(ctx)
	_, err := krc1.KeyResolverLazyDB().ResolveKey("key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	err = krc1.Commit()
	require.Regexp(t, "pop", err)

	require.NoError(t, mc.db.ExpectationsWereMet())

}

func TestKRCPreCommitNoWork(t *testing.T) {
	ctx, km, _, done := newTestKeyManager(t, false,
		&pldconf.KeyManagerConfig{Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")}},
	)
	defer done()

	krc1 := km.NewKeyResolutionContext(ctx)
	err := krc1.PreCommit()
	require.NoError(t, err)

}

func TestKRCGetDBTXNotLazy(t *testing.T) {
	ctx, km, _, done := newTestKeyManager(t, false,
		&pldconf.KeyManagerConfig{Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")}},
	)
	defer done()

	krc1 := km.NewKeyResolutionContext(ctx)
	_, err := krc1.(*keyResolutionContext).getDBTX()
	assert.Regexp(t, "PD010514", err)

}

func TestKRCResolveEmptyPathSegment(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, false,
		&pldconf.KeyManagerConfig{Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")}},
	)
	defer done()

	krc1 := km.NewKeyResolutionContextLazyDB(ctx)
	kr := krc1.KeyResolverLazyDB().(*keyResolver)
	kr.resolvedPaths[""] = &resolvedDBPath{}
	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{})) // "a" - not found
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{})) // root to get index for "a"
	mc.db.ExpectExec("INSERT.*key_paths").WillReturnResult(sqlmock.NewResult(0, 1))    // add "a"

	_, err := kr.ResolveKey("a..wrong", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	assert.Regexp(t, "PD010500", err)

}

func TestKRCNoCreate(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, false,
		&pldconf.KeyManagerConfig{Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")}},
	)
	defer done()

	krc1 := km.NewKeyResolutionContextLazyDB(ctx)
	kr := krc1.KeyResolverLazyDB().(*keyResolver)
	kr.resolvedPaths[""] = &resolvedDBPath{}
	// mc.db.ExpectBegin()
	// mc.db.ExpectQuery("SELECT.*key_verifiers").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))

	_, err := km.ReverseKeyLookup(ctx, mc.c.Persistence().DB(), "a", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	assert.Regexp(t, "PD010500", err)

}
