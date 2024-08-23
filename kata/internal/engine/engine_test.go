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

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/kata/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Attempt to assert the behaviour of the Engine as a whole component in isolation from the rest of the system
// Tests in this file do not mock anything else in this package or sub packages but does mock other components and managers in paladin as per their interfaces

func TestEngine(t *testing.T) {
	ctx := context.Background()

	domainAddress := types.MustEthAddress(types.RandHex(20))
	domainAddressString := domainAddress.String()

	engine, mComponents := newEngineForTesting(t)
	mDomainStateInterface := componentmocks.NewDomainStateInterface(t)
	mDomainAPI := &componentmocks.DomainSmartContract{}
	mDomainAPI = componentmocks.NewDomainSmartContract(t)
	mDomainAPI.On("InitTransaction", ctx, mock.Anything).Return(nil)
	mDomainMgr := &componentmocks.DomainManager{}
	mDomainMgr.On("GetSmartContractByAddress", ctx, *domainAddress).Once().Return(mDomainAPI, nil)
	mStateStore := &componentmocks.StateStore{}
	mComponents.On("StateStore").Once().Return(mStateStore)
	mComponents.On("DomainManager").Once().Return(mDomainMgr).Maybe()
	mStateStore.On("RunInDomainContext", mock.Anything, mock.AnythingOfType("statestore.DomainContextFunction")).Run(func(args mock.Arguments) {
		fn := args.Get(1).(statestore.DomainContextFunction)
		fn(ctx, mDomainStateInterface)
	}).Once().Return(nil)
	assert.Equal(t, "Kata Engine", engine.Name())

	txID, err := engine.HandleNewTx(ctx, &components.PrivateTransaction{})
	// no input domain should err
	assert.Regexp(t, "PD011700", err)
	assert.Empty(t, txID)
	txID, err = engine.HandleNewTx(ctx, &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: domainAddressString,
		},
	})

	//poll until the transaction is processed
	for {
		status, err := engine.GetTxStatus(ctx, txID)
		require.NoError(t, err)
		if status.Status == "completed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	assert.NoError(t, err)
	require.NotNil(t, txID)

}

func newEngineForTesting(t *testing.T) (Engine, *componentmocks.AllComponents) {
	mockAllComponents := componentmocks.NewAllComponents(t)
	e := NewEngine()
	r, err := e.Init(mockAllComponents)
	assert.Nil(t, r)
	assert.NoError(t, err)
	return e, mockAllComponents

}
