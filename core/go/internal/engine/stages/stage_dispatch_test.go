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

package stages

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/transactionstore"
	"github.com/kaleido-io/paladin/core/mocks/enginemocks"
	"github.com/stretchr/testify/assert"
)

func TestDispatchStageMatchTrue(t *testing.T) {
	ctx := context.Background()
	tx := components.PrivateTransaction{
		Signer: "not empty",
	}

	tsg := &transactionstore.TransactionWrapper{
		PrivateTransaction: &tx,
	}

	ds := &DispatchStage{}
	assert.Equal(t, "dispatch", ds.Name())

	mSFS := &enginemocks.StageFoundationService{}

	// expect transaction to not match dispatch stage, when tx payload is not prepared.
	assert.True(t, ds.MatchStage(ctx, tsg, mSFS))

}

func TestDispatchStageMatchFalse(t *testing.T) {
	ctx := context.Background()
	tx := components.PrivateTransaction{
		Signer: "",
	}

	tsg := &transactionstore.TransactionWrapper{
		PrivateTransaction: &tx,
	}

	ds := &DispatchStage{}
	assert.Equal(t, "dispatch", ds.Name())

	mSFS := &enginemocks.StageFoundationService{}

	// expect transaction to not match dispatch stage, when tx payload is not prepared.
	assert.False(t, ds.MatchStage(ctx, tsg, mSFS))

}

func TestDispatchStagePreReqCheck(t *testing.T) {
	ctx := context.Background()
	tx := components.PrivateTransaction{
		Signer: "",
	}

	tsg := &transactionstore.TransactionWrapper{
		PrivateTransaction: &tx,
	}

	ds := &DispatchStage{}
	assert.Equal(t, "dispatch", ds.Name())

	mSFS := &enginemocks.StageFoundationService{}

	// expect transaction to not match dispatch stage, when tx payload is not prepared.
	prereqs := ds.GetIncompletePreReqTxIDs(ctx, tsg, mSFS)
	assert.Nil(t, prereqs)
}
