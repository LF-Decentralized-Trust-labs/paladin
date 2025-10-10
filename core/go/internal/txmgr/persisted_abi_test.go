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

package txmgr

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
)

func TestGetABIByHashError(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.getABIByHash(ctx, txm.p.NOTX(), pldtypes.RandBytes32())
	assert.Regexp(t, "pop", err)

}

func TestGetABIByHashBadData(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnRows(sqlmock.NewRows(
				[]string{"abi"},
			).AddRow(
				`{!!!! bad JSON`,
			))
		})
	defer done()

	_, err := txm.getABIByHash(ctx, txm.p.NOTX(), pldtypes.RandBytes32())
	assert.Regexp(t, "PD012217", err)

}

func TestGetABIByCache(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abis").WillReturnRows(sqlmock.NewRows(
				[]string{"abi"},
			).AddRow(
				`[]`,
			))
		})
	defer done()

	hash := pldtypes.RandBytes32()

	// 2nd time cached (only one DB mock)
	for i := 0; i < 2; i++ {
		pa, err := txm.getABIByHash(ctx, txm.p.NOTX(), hash)
		assert.NoError(t, err)
		assert.Equal(t, hash, pa.Hash)
		assert.Equal(t, abi.ABI{}, pa.ABI)
	}

}

func TestUpsertABIBadData(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.UpsertABI(ctx, txm.p.NOTX(), abi.ABI{{Inputs: abi.ParameterArray{{Type: "wrong"}}}})
	assert.Regexp(t, "PD012201", err)

}

func TestUpsertABIFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT INTO.*abis").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.storeABI(ctx, txm.p.NOTX(), abi.ABI{{Type: abi.Function, Name: "get"}})
	assert.Regexp(t, "pop", err)

}
