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

package keymanager

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/require"
)

func mockQueryRootExisting(mc *mockComponents) {
	mc.db.ExpectQuery("SELECT.*").WillReturnRows(sqlmock.NewRows([]string{
		"segment", "index", "path", "next_index", "parent",
	}).AddRow(
		"" /* root */, 0, "", 1, "",
	))
}

func mockNewFirstLevelEntryExistingRoot(mc *mockComponents) {
	mc.db.ExpectBegin()
	// Return the existing root, for the root lookup
	mockQueryRootExisting(mc)
	// Then we will find the entry
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))
	// Then we will look up root again (locked) to get the next index
	mockQueryRootExisting(mc)
	// Then we insert
	mc.db.ExpectExec("INSERT.*key_paths").WillReturnResult(driver.RowsAffected(1))
}

func TestGetOrCreateIdentifierPathEmptySegment(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mockQueryRootExisting(mc)

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		_, err := kr.getOrCreateIdentifierPath(ctx, "..wrong", false)
		return err
	})
	require.Regexp(t, "PD010500", err)

}

func TestResolvePathNoCreate(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		_, err := kr.resolvePathSegment(ctx, kr.rootPath, "", false)
		return err
	})
	require.Regexp(t, "PD010512", err)

}

func TestResolvePathInLockLookupFail(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnError(fmt.Errorf("pop"))

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		_, err := kr.resolvePathSegment(ctx, kr.rootPath, "", true)
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestResolvePathCreateFail(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.ExpectExec("INSERT.*key_paths").WillReturnError(fmt.Errorf("pop"))

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		_, err := kr.resolvePathSegment(ctx, kr.rootPath, "", true)
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestResolveKeyIdentifierLookupFail(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mockNewFirstLevelEntryExistingRoot(mc)
	mc.db.ExpectQuery("SELECT.*key_mappings").WillReturnError(fmt.Errorf("pop"))

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		_, err := kr.ResolveKey(ctx, "root1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestGetStoredVerifierFail(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_verifiers").WillReturnError(fmt.Errorf("pop"))

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		_, err := kr.getStoredVerifier(ctx, "any", algorithms.ECDSA_SECP256K1, algorithms.ECDSA_SECP256K1)
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestGetStoredVerifierNotFound(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_verifiers").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.ExpectCommit()

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		rv, err := kr.getStoredVerifier(ctx, "any", algorithms.ECDSA_SECP256K1, algorithms.ECDSA_SECP256K1)
		require.Nil(t, rv)
		return err
	})
	require.NoError(t, err)

}

func TestResolveKeyNotFound(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mockQueryRootExisting(mc)
	mc.db.ExpectQuery("SELECT.*").WillReturnRows(sqlmock.NewRows([]string{
		"segment", "index", "path", "next_index", "parent",
	}).AddRow(
		"root1", 0, "", 1, "",
	))
	mc.db.ExpectQuery("SELECT.*key_mappings").WillReturnRows(sqlmock.NewRows([]string{}))

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		rv, err := kr.resolveKey(ctx, "root1", algorithms.ECDSA_SECP256K1, algorithms.ECDSA_SECP256K1, true)
		require.Nil(t, rv)
		return err
	})
	require.Regexp(t, "PD010513.*root1", err)

}

func TestResolveNewMappingNoSuitableWalletError(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "not_this_one")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		rv, err := kr.resolveMapping(ctx,
			&pldapi.KeyMappingWithPath{
				KeyMapping: &pldapi.KeyMapping{Identifier: "root1"},
				Path:       []*pldapi.KeyPathSegment{{Name: ""}},
			},
			true,
			"root1", algorithms.ECDSA_SECP256K1, algorithms.ECDSA_SECP256K1, true)
		require.Nil(t, rv)
		return err
	})
	require.Regexp(t, "PD010501", err)

}

func TestResolveExistingMappingNoWallet(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		rv, err := kr.resolveMapping(ctx,
			&pldapi.KeyMappingWithPath{
				KeyMapping: &pldapi.KeyMapping{Identifier: "root1", Wallet: "nope"},
				Path:       []*pldapi.KeyPathSegment{{Name: ""}},
			},
			false,
			"root1", algorithms.ECDSA_SECP256K1, algorithms.ECDSA_SECP256K1, true)
		require.Nil(t, rv)
		return err
	})
	require.Regexp(t, "PD010503", err)

}

func TestResolveExistingGetStoredVerifierFail(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_verifiers").WillReturnError(fmt.Errorf("pop"))

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		rv, err := kr.resolveMapping(ctx,
			&pldapi.KeyMappingWithPath{
				KeyMapping: &pldapi.KeyMapping{Identifier: "root1", Wallet: "hdwallet1"},
				Path:       []*pldapi.KeyPathSegment{{Name: ""}},
			},
			false,
			"root1", algorithms.ECDSA_SECP256K1, algorithms.ECDSA_SECP256K1, true)
		require.Nil(t, rv)
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestResolveNewMappingWhenRequiredExisting(t *testing.T) {

	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX).(*keyResolver)
		rv, err := kr.resolveMapping(ctx,
			&pldapi.KeyMappingWithPath{
				KeyMapping: &pldapi.KeyMapping{Identifier: "root1", Wallet: "hdwallet1"},
				Path:       []*pldapi.KeyPathSegment{{Name: ""}},
			},
			true,
			"root1", algorithms.ECDSA_SECP256K1, algorithms.ECDSA_SECP256K1,
			true)
		require.Nil(t, rv)
		return err
	})
	require.Regexp(t, "PD010513", err)

}
