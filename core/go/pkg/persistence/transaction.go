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

	"gorm.io/gorm"
)

type singletonVal struct {
	key   any
	value any
	next  *singletonVal
}

// Wrapper around a database transaction context, which provides lifecycle functions beyond those of GORM.
type DBTX interface {
	// Access the Gorm DB object for the DB/transaction
	DB() *gorm.DB
	// False means you cannot call pre-commit/post-commit, because this is just a NOTX() transaction. Used by code that wants to allow a mini-transaction to be created ad-hoc, but only when there's no transaction in flight.
	FullTransaction() bool
	// Functions to be run at the end of the transaction, before it has committed. An error from these will cause a rollback of the transaction itself
	AddPreCommit(func(txCtx context.Context, tx DBTX) error)
	// Only called after a transaction is successfully committed - useful for triggering other actions that are conditional on new data
	AddPostCommit(func(txCtx context.Context))
	// Called in all cases (including panic cases) AFTER the transaction commits, to release resources. An error indicates the transaction rolled back. Can be used as a post-commit too by checking err==nil.
	AddFinalizer(func(txCtx context.Context, err error))
	// This is for if after a rollback error (not panic), you wish to adjust the error returned based on performing new DB activity (as you cannot do any DB activity in the rolled back TXN)
	AddPostRollback(func(txCtx context.Context, err error) error)
	// Management of singleton component interfaces, using a value key (similar to contexts)
	Singleton(key any, new func(txCtx context.Context) any) any
}

type transaction struct {
	txCtx         context.Context
	gdb           *gorm.DB
	preCommits    []func(txCtx context.Context, tx DBTX) error
	postCommits   []func(txCtx context.Context)
	finalizers    []func(txCtx context.Context, err error)
	postRollbacks []func(txCtx context.Context, err error) error
	singletons    *singletonVal
}

func (t *transaction) DB() *gorm.DB {
	return t.gdb
}

func (t *transaction) AddPreCommit(fn func(txCtx context.Context, tx DBTX) error) {
	t.preCommits = append(t.preCommits, fn)
}

func (t *transaction) AddPostCommit(fn func(txCtx context.Context)) {
	t.postCommits = append(t.postCommits, fn)
}

func (t *transaction) AddFinalizer(fn func(txCtx context.Context, err error)) {
	t.finalizers = append(t.finalizers, fn)
}

func (t *transaction) AddPostRollback(fn func(txCtx context.Context, err error) error) {
	t.postRollbacks = append(t.postRollbacks, fn)
}

func (t *transaction) Singleton(key any, new func(ctx context.Context) any) any {
	v := t.singletons
	for v != nil {
		if v.key == key {
			return v.value
		}
		v = v.next
	}
	newValue := new(t.txCtx)
	newRoot := &singletonVal{next: t.singletons, key: key, value: newValue}
	t.singletons = newRoot
	return newValue
}

func (t *transaction) FullTransaction() bool {
	return true
}

func newNOTX(gdb *gorm.DB) DBTX {
	return &noTransaction{gdb: gdb}
}

type noTransaction struct {
	gdb *gorm.DB
}

func (t *noTransaction) DB() *gorm.DB {
	return t.gdb
}

func (t *noTransaction) AddPreCommit(fn func(txCtx context.Context, tx DBTX) error) {
	panic("pre-commit used outside of transaction context")
}

func (t *noTransaction) AddPostCommit(fn func(txCtx context.Context)) {
	panic("post-commit used outside of transaction context")
}

func (t *noTransaction) AddPostRollback(fn func(txCtx context.Context, err error) error) {
	panic("post-rollback used outside of transaction context")
}

func (t *noTransaction) AddFinalizer(fn func(txCtx context.Context, err error)) {
	panic("finalizer used outside of transaction context")
}

func (t *noTransaction) Singleton(key any, new func(txCtx context.Context) any) any {
	panic("singleton components used outside of transaction context")
}

func (t *noTransaction) FullTransaction() bool {
	return false
}
