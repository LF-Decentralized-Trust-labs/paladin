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
	pbEngine "github.com/kaleido-io/paladin/kata/pkg/proto/engine"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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

	mTransportManager := componentmocks.NewTransportManager(t)
	sentEndorsementRequest := make(chan struct{}, 1)
	mTransportManager.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		sentEndorsementRequest <- struct{}{}
	}).Return(nil).Maybe()

	onMessage := func(ctx context.Context, message components.TransportMessage) error {
		assert.Fail(t, "onMessage has not been set")
		return nil
	}
	// mock Recieve(component string, onMessage func(ctx context.Context, message TransportMessage) error) error
	mTransportManager.On("RegisterReceiver", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		onMessage = args.Get(0).(func(ctx context.Context, message components.TransportMessage) error)

	}).Return(nil).Maybe()

	mStateStore := componentmocks.NewStateStore(t)
	//TODO do we need this?
	mStateStore.On("RunInDomainContext", mock.Anything, mock.AnythingOfType("statestore.DomainContextFunction")).Run(func(args mock.Arguments) {
		fn := args.Get(1).(statestore.DomainContextFunction)
		err := fn(ctx, mDomainStateInterface)
		assert.NoError(t, err)
	}).Maybe().Return(nil)

	mComponents.On("StateStore").Return(mStateStore).Maybe()
	mComponents.On("DomainManager").Return(mDomainMgr).Maybe()
	mComponents.On("TransportManager").Return(mTransportManager).Maybe()

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
	case <-time.After(100000 * time.Millisecond):
		assert.Fail(t, "Timed out waiting for transaction to be initialised")
	case <-initialised:
		break
	}

	//poll until the transaction is assembled
	select {
	case <-time.After(100000 * time.Millisecond):
		assert.Fail(t, "Timed out waiting for transaction to be assembled")
	case <-assembled:
		break
	}

	//poll until the endorsement request has been sent
	select {
	case <-time.After(100000 * time.Millisecond):
		assert.Fail(t, "Timed out waiting for endorsement request to be sent")
	case <-sentEndorsementRequest:
		break
	}

	attestationResult := prototk.AttestationResult{
		Name:            "notary",
		AttestationType: prototk.AttestationType_ENDORSE,
		Payload:         types.RandBytes(32),
	}

	attestationResultAny, err := anypb.New(&attestationResult)
	assert.NoError(t, err)

	//for now, while endorsement is a stage, we will send the endorsement back as a stage message
	engineMessage := pbEngine.StageMessage{
		ContractAddress: domainAddressString,
		TransactionId:   txID,
		Data:            attestationResultAny,
		Stage:           "attestation",
	}

	engineMessageBytes, err := proto.Marshal(&engineMessage)
	assert.NoError(t, err)

	//now send the endorsement back
	err = onMessage(ctx, components.TransportMessage{
		MessageType: "endorsement",
		Payload:     engineMessageBytes,
	})
	assert.NoError(t, err)

	timeout := time.After(2 * time.Second)
	tick := time.Tick(100 * time.Millisecond)

	status := func() string {
		for {
			select {
			case <-timeout:
				// Timeout reached, exit the loop
				assert.Fail(t, "Timed out waiting for transaction to be endorsed")
				return "timeout"
			case <-tick:
				s, err := engine.GetTxStatus(ctx, domainAddressString, txID)
				if s.Status == "endorsed" {
					return s.Status
				}
				assert.NoError(t, err)
			}
		}
	}()

	assert.Equal(t, status, "endorsed")

}

func newEngineForTesting(t *testing.T) (Engine, *componentmocks.AllComponents) {
	mockAllComponents := componentmocks.NewAllComponents(t)
	e := NewEngine()
	r, err := e.Init(mockAllComponents)
	assert.Nil(t, r)
	assert.NoError(t, err)
	return e, mockAllComponents

}
