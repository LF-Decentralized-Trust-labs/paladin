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

func TestEngineSimpleTransaction(t *testing.T) {
	ctx := context.Background()

	engine, mocks, domainAddress := newEngineForTesting(t)
	assert.Equal(t, "Kata Engine", engine.Name())

	domainAddressString := domainAddress.String()

	initialised := make(chan struct{}, 1)
	mocks.domainSmartContract.On("InitTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		initialised <- struct{}{}
	}).Return(nil)

	assembled := make(chan struct{}, 1)
	mocks.domainSmartContract.On("AssembleTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
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

	sentEndorsementRequest := make(chan struct{}, 1)
	mocks.transportManager.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		sentEndorsementRequest <- struct{}{}
	}).Return(nil).Maybe()

	onMessage := func(ctx context.Context, message components.TransportMessage) error {
		assert.Fail(t, "onMessage has not been set")
		return nil
	}
	// mock Recieve(component string, onMessage func(ctx context.Context, message TransportMessage) error) error
	mocks.transportManager.On("RegisterReceiver", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		onMessage = args.Get(0).(func(ctx context.Context, message components.TransportMessage) error)

	}).Return(nil).Maybe()

	//TODO do we need this?
	mocks.stateStore.On("RunInDomainContext", mock.Anything, mock.AnythingOfType("statestore.DomainContextFunction")).Run(func(args mock.Arguments) {
		fn := args.Get(1).(statestore.DomainContextFunction)
		err := fn(ctx, mocks.domainStateInterface)
		assert.NoError(t, err)
	}).Maybe().Return(nil)

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

	//poll until the endorsement request has been sent
	select {
	case <-time.After(1000 * time.Millisecond):
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
				s, err := engine.GetTxStatus(ctx, domainAddressString, txID)
				require.NoError(t, err)
				return s.Status
			case <-tick:
				s, err := engine.GetTxStatus(ctx, domainAddressString, txID)
				if s.Status == "dispatch" {
					return s.Status
				}
				assert.NoError(t, err)
			}
		}
	}()

	assert.Equal(t, "dispatch", status)

}

func TestEngineDependantTransaction(t *testing.T) {
	ctx := context.Background()

	engine, mocks, domainAddress := newEngineForTesting(t)
	assert.Equal(t, "Kata Engine", engine.Name())

	domainAddressString := domainAddress.String()

	mocks.domainSmartContract.On("InitTransaction", ctx, mock.Anything).Return(nil)
	states := []*components.FullState{
		{
			ID:     types.Bytes32(types.RandBytes(32)),
			Schema: types.Bytes32(types.RandBytes(32)),
			Data:   types.JSONString("foo"),
		},
	}

	tx1 := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: domainAddressString,
			From:   "Alice",
		},
	}

	tx2 := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain: domainAddressString,
			From:   "Bob",
		},
	}

	mocks.domainSmartContract.On("AssembleTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)
		switch tx.Inputs.From {
		case "Alice":
			tx.PostAssembly = &components.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_OK,
				OutputStates:   states,
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
		case "Bob":
			tx.PostAssembly = &components.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_OK,
				InputStates:    states,
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
		}

	}).Return(nil)

	sentEndorsementRequest := make(chan struct{}, 1)
	mocks.transportManager.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		sentEndorsementRequest <- struct{}{}
	}).Return(nil).Maybe()

	onMessage := func(ctx context.Context, message components.TransportMessage) error {
		assert.Fail(t, "onMessage has not been set")
		return nil
	}
	// mock Recieve(component string, onMessage func(ctx context.Context, message TransportMessage) error) error
	mocks.transportManager.On("RegisterReceiver", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		onMessage = args.Get(0).(func(ctx context.Context, message components.TransportMessage) error)

	}).Return(nil).Maybe()

	//TODO do we need this?
	mocks.stateStore.On("RunInDomainContext", mock.Anything, mock.AnythingOfType("statestore.DomainContextFunction")).Run(func(args mock.Arguments) {
		fn := args.Get(1).(statestore.DomainContextFunction)
		err := fn(ctx, mocks.domainStateInterface)
		assert.NoError(t, err)
	}).Maybe().Return(nil)

	err := engine.Start()
	assert.NoError(t, err)

	tx1ID, err := engine.HandleNewTx(ctx, tx1)
	assert.NoError(t, err)
	require.NotNil(t, tx1ID)

	tx2ID, err := engine.HandleNewTx(ctx, tx2)
	assert.NoError(t, err)
	require.NotNil(t, tx2ID)

	// Neither transaction should be dispatched yet
	s, err := engine.GetTxStatus(ctx, domainAddressString, tx1ID)
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	s, err = engine.GetTxStatus(ctx, domainAddressString, tx2ID)
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	attestationResult := prototk.AttestationResult{
		Name:            "notary",
		AttestationType: prototk.AttestationType_ENDORSE,
		Payload:         types.RandBytes(32),
	}

	attestationResultAny, err := anypb.New(&attestationResult)
	assert.NoError(t, err)

	// endorse transaction 2 before 1 and check that 2 is not dispatched before 1
	engineMessage := pbEngine.StageMessage{
		ContractAddress: domainAddressString,
		TransactionId:   tx2ID,
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

	//unless the tests are running in short mode, wait a second to ensure that the transaction is not dispatched
	if !testing.Short() {
		time.Sleep(1 * time.Second)
	}
	s, err = engine.GetTxStatus(ctx, domainAddressString, tx1ID)
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	s, err = engine.GetTxStatus(ctx, domainAddressString, tx2ID)
	require.NoError(t, err)
	assert.NotEqual(t, "dispatch", s.Status)

	// endorse transaction 1 and check that both it and 2 are dispatched
	engineMessage = pbEngine.StageMessage{
		ContractAddress: domainAddressString,
		TransactionId:   tx1ID,
		Data:            attestationResultAny,
		Stage:           "attestation",
	}

	engineMessageBytes, err = proto.Marshal(&engineMessage)
	assert.NoError(t, err)

	//now send the endorsement back
	err = onMessage(ctx, components.TransportMessage{
		MessageType: "endorsement",
		Payload:     engineMessageBytes,
	})
	assert.NoError(t, err)

	status := pollForStatus(ctx, t, "dispatch", engine, domainAddressString, tx1ID, 2*time.Second)
	assert.Equal(t, "dispatch", status)

	status = pollForStatus(ctx, t, "dispatch", engine, domainAddressString, tx2ID, 2*time.Second)
	assert.Equal(t, "dispatch", status)

	//TODO assert that transaction 1 got dispatched before 2

}

func pollForStatus(ctx context.Context, t *testing.T, expectedStatus string, engine Engine, domainAddressString, txID string, duration time.Duration) string {
	timeout := time.After(duration)
	tick := time.Tick(100 * time.Millisecond)

	for {
		select {
		case <-timeout:
			// Timeout reached, exit the loop
			assert.Failf(t, "Timed out waiting for status %s", expectedStatus)
			s, err := engine.GetTxStatus(ctx, domainAddressString, txID)
			require.NoError(t, err)
			return s.Status
		case <-tick:
			s, err := engine.GetTxStatus(ctx, domainAddressString, txID)
			if s.Status == expectedStatus {
				return s.Status
			}
			assert.NoError(t, err)
		}
	}
}

type dependencyMocks struct {
	allComponents        *componentmocks.AllComponents
	domainStateInterface *componentmocks.DomainStateInterface
	domainSmartContract  *componentmocks.DomainSmartContract
	domainMgr            *componentmocks.DomainManager
	transportManager     *componentmocks.TransportManager
	stateStore           *componentmocks.StateStore
}

func newEngineForTesting(t *testing.T) (Engine, *dependencyMocks, *types.EthAddress) {
	domainAddress := types.MustEthAddress(types.RandHex(20))

	mocks := &dependencyMocks{
		allComponents:        componentmocks.NewAllComponents(t),
		domainStateInterface: componentmocks.NewDomainStateInterface(t),
		domainSmartContract:  componentmocks.NewDomainSmartContract(t),
		domainMgr:            componentmocks.NewDomainManager(t),
		transportManager:     componentmocks.NewTransportManager(t),
		stateStore:           componentmocks.NewStateStore(t),
	}
	mocks.allComponents.On("StateStore").Return(mocks.stateStore).Maybe()
	mocks.allComponents.On("DomainManager").Return(mocks.domainMgr).Maybe()
	mocks.allComponents.On("TransportManager").Return(mocks.transportManager).Maybe()
	mocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, *domainAddress).Maybe().Return(mocks.domainSmartContract, nil)

	e := NewEngine(uuid.Must(uuid.NewUUID()))
	r, err := e.Init(mocks.allComponents)
	assert.Nil(t, r)
	assert.NoError(t, err)
	return e, mocks, domainAddress

}
