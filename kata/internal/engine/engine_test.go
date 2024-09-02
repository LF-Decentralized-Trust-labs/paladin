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
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/components"
	engineTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	pbEngine "github.com/kaleido-io/paladin/kata/pkg/proto/engine"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Attempt to assert the behaviour of the Engine as a whole component in isolation from the rest of the system
// Tests in this file do not mock anything else in this package or sub packages but does mock other components and managers in paladin as per their interfaces

func TestEngineInit(t *testing.T) {

	engine, mocks, _ := newEngineForTesting(t)
	assert.Equal(t, "Kata Engine", engine.EngineName())
	initResult, err := engine.Init(mocks.allComponents)
	assert.NoError(t, err)
	assert.NotNil(t, initResult)
}

func TestEngineSimpleTransaction(t *testing.T) {
	ctx := context.Background()

	engine, mocks, domainAddress := newEngineForTesting(t)
	assert.Equal(t, "Kata Engine", engine.EngineName())

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
	assert.Equal(t, "Kata Engine", engine.EngineName())

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
	}).Once().Return(nil)

	mocks.domainSmartContract.On("AssembleTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		tx := args.Get(1).(*components.PrivateTransaction)

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

	}).Once().Return(nil)

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

	//wait for both transactions to send the endorsement request
	<-sentEndorsementRequest
	<-sentEndorsementRequest

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

func TestEngineMiniLoad(t *testing.T) {
	ctx := context.Background()
	log.SetLevel("debug")

	engine, mocks, domainAddress := newEngineForTesting(t)
	assert.Equal(t, "Kata Engine", engine.EngineName())

	domainAddressString := domainAddress.String()

	//500 is the maximum we can do in this test for now until either
	//a) implement config to allow us to define MaxConcurrentTransactions
	//b) implement ( or mock) transaction dispatch processing all the way to confirmation
	numTransactions := 500

	dependenciesByTransactionID := make(map[string][]string) // populated during assembly stage
	nonceByTransactionID := make(map[string]uint64)          // populated when dispatch event recieved and used later to check that the nonce order matchs the dependency order

	unclaimedPendingStatesToMintingTransaction := make(map[types.Bytes32]string)

	mocks.domainSmartContract.On("InitTransaction", ctx, mock.Anything).Return(nil)

	r := rand.New(rand.NewSource(42))
	failEarly := make(chan string, 1)

	assembleConcurrency := 0
	mocks.domainSmartContract.On("AssembleTransaction", ctx, mock.Anything).Run(func(args mock.Arguments) {
		//assert that we are not assembling more than 1 transaction at a time
		if assembleConcurrency > 0 {
			failEarly <- "Assembling more than one transaction at a time"
		}
		require.Equal(t, assembleConcurrency, 0, "Assembling more than one transaction at a time")

		assembleConcurrency++
		defer func() { assembleConcurrency-- }()

		// chose a number of dependencies at random 0, 1, 2, 3
		// for each dependency, chose a different unclaimed pending state to spend
		tx := args.Get(1).(*components.PrivateTransaction)

		var inputStates []*components.FullState
		numDependencies := min(r.Intn(4), len(unclaimedPendingStatesToMintingTransaction))
		dependencies := make([]string, numDependencies)
		for i := 0; i < numDependencies; i++ {
			// chose a random unclaimed pending state to spend
			stateIndex := r.Intn(len(unclaimedPendingStatesToMintingTransaction))

			keys := make([]types.Bytes32, len(unclaimedPendingStatesToMintingTransaction))
			keyIndex := 0
			for keyName := range unclaimedPendingStatesToMintingTransaction {

				keys[keyIndex] = keyName
				keyIndex++
			}
			stateID := keys[stateIndex]
			inputStates = append(inputStates, &components.FullState{
				ID: stateID,
			})

			log.L(ctx).Infof("input state %s, numDependencies %d i %d", stateID, numDependencies, i)
			dependencies[i] = unclaimedPendingStatesToMintingTransaction[stateID]
			delete(unclaimedPendingStatesToMintingTransaction, stateID)
		}
		dependenciesByTransactionID[tx.ID.String()] = dependencies

		numOutputStates := r.Intn(4)
		outputStates := make([]*components.FullState, numOutputStates)
		for i := 0; i < numOutputStates; i++ {
			stateID := types.Bytes32(types.RandBytes(32))
			outputStates[i] = &components.FullState{
				ID: stateID,
			}
			unclaimedPendingStatesToMintingTransaction[stateID] = tx.ID.String()
		}

		tx.PostAssembly = &components.TransactionPostAssembly{

			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			OutputStates:   outputStates,
			InputStates:    inputStates,
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
	}).Return(nil)

	endorsementRequests := make(chan string, 10)
	//mocks.transportManager.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.AnythingOfType("github.com/kaleido-io/paladin/kata/internal/components.TransportMessage")).Run(func(args mock.Arguments) {
	mocks.transportManager.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		transportMessage := args.Get(1)
		switch transportMessage := transportMessage.(type) {
		case components.TransportMessage:

			payloadBytes := transportMessage.Payload
			stageEvent := new(engineTypes.StageEvent)
			err := json.Unmarshal(payloadBytes, stageEvent)
			assert.NoError(t, err)

			endorsementRequests <- stageEvent.TxID
		default:
			assert.Fail(t, "Unexpected message type")
		}
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

	expectedNonce := uint64(0)

	numDispatched := 0
	allDispatched := make(chan bool, 1)
	engine.Subscribe(ctx, func(event engineTypes.EngineEvent) {
		numDispatched++
		switch event := event.(type) {
		case *engineTypes.TransactionDispatchedEvent:
			assert.Equal(t, expectedNonce, event.Nonce)
			expectedNonce++
			nonceByTransactionID[event.TransactionID] = event.Nonce
		}
		if numDispatched == numTransactions {
			allDispatched <- true
		}
	})

	err := engine.Start()
	assert.NoError(t, err)

	for i := 0; i < numTransactions; i++ {
		tx := &components.PrivateTransaction{
			ID: uuid.New(),
			Inputs: &components.TransactionInputs{
				Domain: domainAddressString,
				From:   "Alice",
			},
		}
		txID, err := engine.HandleNewTx(ctx, tx)
		assert.NoError(t, err)
		require.NotNil(t, txID)
	}

	// whenever a new endorsement request comes in, endorse it after a random delay
	go func() {
		for {
			txID := <-endorsementRequests
			go func() {
				time.Sleep(time.Duration(r.Intn(1000)) * time.Millisecond)
				attestationResult := prototk.AttestationResult{
					Name:            "notary",
					AttestationType: prototk.AttestationType_ENDORSE,
					Payload:         types.RandBytes(32),
				}

				attestationResultAny, err := anypb.New(&attestationResult)
				assert.NoError(t, err)

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
			}()
		}
	}()

	deadline, ok := t.Deadline()
	if !ok {
		//there was no -timeout flag, default to 10 seconds
		deadline = time.Now().Add(10 * time.Second)
	}
	haveAllDispatched := false
out:
	for {
		select {
		case <-time.After(time.Until(deadline)):
			log.L(ctx).Errorf("Timed out waiting for all transactions to be dispatched")
			assert.Fail(t, "Timed out waiting for all transactions to be dispatched")
			break out
		case <-allDispatched:
			haveAllDispatched = true
			break out
		case reason := <-failEarly:
			require.Fail(t, reason)
		}
	}

	if haveAllDispatched {
		//check that they were dispatched a valid order ( i.e. no transaction was dispatched before its dependencies)
		for txId, nonce := range nonceByTransactionID {
			dependencies := dependenciesByTransactionID[txId]
			for _, depTxID := range dependencies {
				depNonce, ok := nonceByTransactionID[depTxID]
				assert.True(t, ok)
				assert.True(t, depNonce < nonce)
			}
		}
	}

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
	assert.NotNil(t, r)
	assert.NoError(t, err)
	return e, mocks, domainAddress

}
