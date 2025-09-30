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

package coordinationtest

import (
	_ "embed"

	"context"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func transactionReceiptCondition(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client, isDeploy bool) func() bool {
	//for the given transaction ID, return a function that can be used in an assert.Eventually to check if the transaction has a receipt
	return func() bool {
		txFull := pldapi.TransactionFull{}
		err := rpcClient.CallRPC(ctx, &txFull, "ptx_getTransactionFull", txID)
		require.NoError(t, err)
		require.False(t, (txFull.Receipt != nil && txFull.Receipt.Success == false), "Have transaction receipt but not successful")
		return txFull.Receipt != nil && (!isDeploy || (txFull.Receipt.ContractAddress != nil && *txFull.Receipt.ContractAddress != pldtypes.EthAddress{}))
	}
}

func transactionLatencyThreshold(t *testing.T) time.Duration {
	// normally we would expect a transaction to be confirmed within a couple of seconds but
	// if we are in a debug session, we want to give it much longer
	threshold := 2 * time.Second

	deadline, ok := t.Deadline()
	if !ok {
		//there was no -timeout flag, default to a long time because this is most likely a debug session
		threshold = time.Hour
	} else {
		timeRemaining := time.Until(deadline)

		//Need to leave some time to ensure that polling assertions fail before the test itself timesout
		//otherwise we don't see diagnostic info for things like GoExit called by mocks etc
		timeRemaining = timeRemaining - 100*time.Millisecond

		if timeRemaining < threshold {
			threshold = timeRemaining - 100*time.Millisecond
		}
	}
	t.Logf("Using transaction latency threshold of %v", threshold)

	return threshold
}
