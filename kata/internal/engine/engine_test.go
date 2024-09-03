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
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
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
	assert.Equal(t, "Kata Engine", engine.Name())

	mDomainStateInterface := componentmocks.NewDomainStateInterface(t)

	mDomainSmartContract := componentmocks.NewDomainSmartContract(t)

	initialised := make(chan struct{}, 1)
	mDomainSmartContract.On("InitTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		initialised <- struct{}{}
	}).Return(nil)

	assembled := make(chan struct{}, 1)
	mDomainSmartContract.On("AssembleTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)

		tx.PostAssembly = &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			InputStates: []*components.FullState{
				{
					ID:     types.Bytes32(types.RandBytes(32)),
					Schema: types.Bytes32(types.RandBytes(32)),
					Data:   types.JSONString("foo"),
				},
			},
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "notary",
					AttestationType: prototk.AttestationType_ENDORSE,
					//Algorithm:       api.SignerAlgorithm_ED25519,
					Parties: []string{
						"domain1/contract1/notary",
					},
				},
			},
		}
		assembled <- struct{}{}

	}).Return(nil)

	mDomainMgr := componentmocks.NewDomainManager(t)
	mDomainMgr.On("GetSmartContractByAddress", ctx, *domainAddress).Once().Return(mDomainSmartContract, nil)

	mStateStore := componentmocks.NewStateStore(t)
	//TODO do we need this?
	mStateStore.On("RunInDomainContext", mock.Anything, mock.AnythingOfType("statestore.DomainContextFunction")).Run(func(args mock.Arguments) {
		fn := args.Get(1).(statestore.DomainContextFunction)
		err := fn(ctx, mDomainStateInterface)
		assert.NoError(t, err)
	}).Maybe().Return(nil)

	mComponents.On("StateStore").Return(mStateStore).Maybe()
	mComponents.On("DomainManager").Return(mDomainMgr).Maybe()

	err := engine.Start()
	assert.NoError(t, err)
	txID, err := engine.HandleNewTx(ctx, &components.PrivateTransaction{})
	// no input domain should err
	assert.Regexp(t, "PD011800", err)
	assert.Empty(t, txID)
	txID, err = engine.HandleNewTx(ctx, &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: domainAddressString,
		},
	})
	assert.NoError(t, err)
	require.NotNil(t, txID)

	//poll until the transaction is initialised
	select {
	case <-time.After(1000 * time.Millisecond):
		assert.Fail(t, "Timed out waiting for transaction to be initialised")
	case <-initialised:
		break
	}

	//poll until the transaction is assembled
	select {
	case <-time.After(1000 * time.Millisecond):
		assert.Fail(t, "Timed out waiting for transaction to be assembled")
	case <-assembled:
		break
	}

}

func newEngineForTesting(t *testing.T) (Engine, *componentmocks.AllComponents) {
	mockAllComponents := componentmocks.NewAllComponents(t)
	e := NewEngine()
	r, err := e.Init(mockAllComponents)
	assert.Nil(t, r)
	assert.NoError(t, err)
	return e, mockAllComponents

}
