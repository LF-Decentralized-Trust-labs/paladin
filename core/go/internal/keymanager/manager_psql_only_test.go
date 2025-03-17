//go:build testdbpostgres
// +build testdbpostgres

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
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test is PSQL only as it requires two concurrent DB transactions (and neither the mock DB, or SQLite, support this)
func TestTimeoutWaitingForLock(t *testing.T) {

	ctx, km, _, done := newTestDBKeyManagerWithWallets(t, hdWalletConfig("wallet1", ""))
	defer done()

	readyToTry := make(chan struct{})
	waitDone := make(chan struct{})
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			mapping1, err := km.KeyResolverForDBTX(dbTX).ResolveKey(ctx, "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			require.NoError(t, err)
			require.NotEmpty(t, mapping1.Verifier.Verifier)
			close(readyToTry)
			<-waitDone
			return nil
		})
		require.NoError(t, err)
	}()

	// Wait until we know we are blocked
	<-readyToTry
	withTimeout, cancelCtx := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancelCtx()
	var kr2 *keyResolver
	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr2 = km.KeyResolverForDBTX(dbTX).(*keyResolver)
		_, err := kr2.ResolveKey(withTimeout, "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		return err
	})
	assert.Regexp(t, "PD010301", err)

	close(waitDone)

	// Double unlock is a warned no-op
	km.unlockAllocation(ctx, kr2)

	<-workerDone

}
