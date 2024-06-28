// Copyright © 2024 Kaleido, Inc.
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

package manager

import (
	"context"
	"testing"

	"github.com/hyperledger/firefly-transaction-manager/pkg/apitypes"
	"github.com/stretchr/testify/assert"
)

func TestBlockchainTxManager_Init(t *testing.T) {
	manager := NewBlockchainTxManager(context.Background())

	err := manager.Init(context.Background())

	assert.NoError(t, err, "Init should not return an error")
}

func TestBlockchainTxManager_HandleNewTransaction(t *testing.T) {
	manager := NewBlockchainTxManager(context.Background())

	txRequest := &apitypes.TransactionRequest{}

	mtx, submissionRejected, err := manager.HandleNewTransaction(context.Background(), txRequest)

	assert.Error(t, err, "HandleNewTransaction should return an error")
	assert.Nil(t, mtx, "ManagedTX should be nil on error")
	assert.False(t, submissionRejected, "Submission should not be rejected on error")
}
