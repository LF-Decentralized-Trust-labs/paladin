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

package testutil

// This file contains utilities to abstract the complexities of the PrivateTransaction struct for use in tests to help make them more readable
// and to reduce the amount of boilerplate code needed to create a Transaction
import (
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateTransactionBuilder_Defaults(t *testing.T) {
	builder := NewPrivateTransactionBuilderForTesting()
	tx := builder.Build()
	require.NotNil(t, tx)
	assert.NotEqual(t, "", tx.Domain)
	assert.NotEqual(t, uuid.Nil, tx.ID)
	assert.NotEqual(t, tktypes.EthAddress{}, tx.Address)

	require.NotNil(t, tx.PreAssembly)
	assert.Len(t, tx.PreAssembly.RequiredVerifiers, 4)
	assert.Len(t, tx.PreAssembly.Verifiers, 4)

	require.NotNil(t, tx.PostAssembly)
	assert.Len(t, tx.PostAssembly.AttestationPlan, 4)
	assert.Nil(t, tx.PostAssembly.RevertReason)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, tx.PostAssembly.AssemblyResult)
	assert.Len(t, tx.PostAssembly.Signatures, 1)
	assert.Len(t, tx.PostAssembly.Endorsements, 0)

}

func TestPrivateTransactionBuilder_PartiallyEndorsed(t *testing.T) {
	builder := NewPrivateTransactionBuilderForTesting().NumberOfEndorsements(2)
	tx := builder.Build()
	assert.Len(t, tx.PostAssembly.Endorsements, 2)
}

func TestPrivateTransactionBuilder_FullyEndorsed(t *testing.T) {
	builder := NewPrivateTransactionBuilderForTesting().NumberOfEndorsements(3)
	tx := builder.Build()
	assert.Len(t, tx.PostAssembly.Endorsements, 3)
}

func TestPrivateTransactionBuilderList_SameSenderAndAddress(t *testing.T) {
	builders := NewPrivateTransactionBuilderListForTesting(3).Sender("alice@node1")

	txns := builders.BuildSparse()
	require.NotNil(t, txns)
	assert.Len(t, txns, 3)
	for _, tx := range txns {
		assert.NotEqual(t, "", tx.Domain)
		assert.NotEqual(t, uuid.Nil, tx.ID)
		assert.NotEqual(t, tktypes.EthAddress{}, tx.Address)

		require.NotNil(t, tx.PreAssembly)
		assert.Len(t, tx.PreAssembly.RequiredVerifiers, 4)
		assert.Len(t, tx.PreAssembly.Verifiers, 4)

		assert.Nil(t, tx.PostAssembly)
	}
}
