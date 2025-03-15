// Copyright © 2025 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package persistence

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionOk(t *testing.T) {
	ctx := context.Background()

	p, mdb := newMockGormPSQLPersistence(t)

	preCommitCalled := false
	finalizerCalled := false
	postCommitCalled := false

	mdb.ExpectBegin()
	mdb.ExpectExec("INSERT.*a_table").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectExec("INSERT.*b_table").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectCommit()

	err := p.Transaction(ctx, func(ctx context.Context, tx DBTX) error {
		require.True(t, tx.FullTransaction())
		err := tx.DB().Exec("INSERT INPUT a_table (col1) VALUES ('abc');").Error
		require.NoError(t, err)
		tx.AddPreCommit(func(ctx context.Context, preCommitTX DBTX) error {
			preCommitCalled = true
			err := preCommitTX.DB().Exec("INSERT INPUT b_table (col1) VALUES ('def');").Error
			require.Same(t, tx, preCommitTX)
			require.NoError(t, err)
			return nil
		})
		tx.AddFinalizer(func(ctx context.Context, err error) {
			require.Nil(t, err)
			finalizerCalled = true
		})
		tx.AddPostCommit(func(ctx context.Context) {
			postCommitCalled = true
		})
		return nil
	})
	require.NoError(t, err)

	require.True(t, preCommitCalled)
	require.True(t, finalizerCalled)
	require.True(t, postCommitCalled)

	require.NoError(t, mdb.ExpectationsWereMet())
}

func TestTransactionPreCommitErr(t *testing.T) {
	ctx := context.Background()

	p, mdb := newMockGormPSQLPersistence(t)

	preCommitCalled := false
	finalizerCalled := false
	postCommitCalled := false
	postRollbackCalled := false

	mdb.ExpectBegin()
	mdb.ExpectExec("INSERT.*a_table").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectRollback()

	err := p.Transaction(ctx, func(ctx context.Context, tx DBTX) error {
		err := tx.DB().Exec("INSERT INPUT a_table (col1) VALUES ('abc');").Error
		require.NoError(t, err)
		tx.AddPreCommit(func(ctx context.Context, preCommitTX DBTX) error {
			preCommitCalled = true
			return fmt.Errorf("pop")
		})
		tx.AddFinalizer(func(ctx context.Context, err error) {
			require.Regexp(t, "pop", err)
			finalizerCalled = true
		})
		tx.AddPostCommit(func(ctx context.Context) {
			postCommitCalled = true
		})
		tx.AddPostRollback(func(txCtx context.Context, err error) error {
			postRollbackCalled = true
			return fmt.Errorf("%s-popped", err)
		})
		return nil
	})
	require.Regexp(t, "pop-popped", err)

	require.True(t, preCommitCalled)
	require.True(t, finalizerCalled)
	require.False(t, postCommitCalled) // post-commit should not be called
	require.True(t, postRollbackCalled)

	require.NoError(t, mdb.ExpectationsWereMet())
}

func TestTransactionPanic(t *testing.T) {
	ctx := context.Background()

	p, mdb := newMockGormPSQLPersistence(t)

	preCommitCalled := false
	finalizerCalled := false
	postCommitCalled := false
	postRollbackCalled := false

	mdb.ExpectBegin()
	mdb.ExpectRollback()

	assert.Panics(t, func() {
		err := p.Transaction(ctx, func(ctx context.Context, tx DBTX) error {
			tx.AddPreCommit(func(ctx context.Context, preCommitTX DBTX) error {
				preCommitCalled = true
				return nil
			})
			tx.AddFinalizer(func(ctx context.Context, err error) {
				require.Regexp(t, "pop", err)
				finalizerCalled = true
			})
			tx.AddPostCommit(func(ctx context.Context) {
				postCommitCalled = true
			})
			tx.AddPostRollback(func(txCtx context.Context, err error) error {
				postRollbackCalled = true
				return err
			})
			panic("pop")
		})
		require.Regexp(t, "pop", err)
	})

	require.True(t, finalizerCalled)
	require.False(t, preCommitCalled)
	require.False(t, postCommitCalled)   // post-commit should not be called
	require.False(t, postRollbackCalled) // not on panic

	require.NoError(t, mdb.ExpectationsWereMet())
}

func TestTransactionSingletons(t *testing.T) {

	type testSingletonKey1 struct{}
	type testSingletonKey2 struct{}

	value := 100
	newVal := func(ctx context.Context) any {
		v := value
		value++
		return v
	}

	p, mdb := newMockGormPSQLPersistence(t)
	mdb.ExpectBegin()
	mdb.ExpectCommit()
	err := p.Transaction(context.Background(), func(ctx context.Context, tx DBTX) error {

		s1 := tx.Singleton(testSingletonKey1{}, newVal)
		require.Equal(t, 100, s1)

		s2 := tx.Singleton(testSingletonKey2{}, newVal)
		require.Equal(t, 101, s2)

		s3 := tx.Singleton(testSingletonKey1{}, newVal)
		require.Equal(t, 100, s3)

		s4 := tx.Singleton(testSingletonKey2{}, newVal)
		require.Equal(t, 101, s4)

		return nil

	})
	require.NoError(t, err)

}

func TestNOTXFailures(t *testing.T) {
	p, _ := newMockGormPSQLPersistence(t)

	require.NotNil(t, p.NOTX().DB())
	require.False(t, p.NOTX().FullTransaction())

	assert.Panics(t, func() {
		p.NOTX().AddPreCommit(func(ctx context.Context, tx DBTX) error { return nil })
	})
	assert.Panics(t, func() {
		p.NOTX().AddPostCommit(func(ctx context.Context) {})
	})
	assert.Panics(t, func() {
		p.NOTX().AddPostRollback(func(txCtx context.Context, err error) error { return nil })
	})
	assert.Panics(t, func() {
		p.NOTX().AddFinalizer(func(ctx context.Context, err error) {})
	})
	assert.Panics(t, func() {
		p.NOTX().Singleton("", nil)
	})

}
