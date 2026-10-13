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

package transaction

import (
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestBaseEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &BaseEvent{
		TransactionID: txID,
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestBaseEvent_GetEventTime(t *testing.T) {
	eventTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	event := &BaseEvent{
		BaseEvent: common.BaseEvent{
			EventTime: eventTime,
		},
	}
	assert.Equal(t, eventTime, event.GetEventTime())
}

func TestConfirmedSuccessEvent_Type(t *testing.T) {
	event := &ConfirmedSuccessEvent{}
	assert.Equal(t, Event_ConfirmedSuccess, event.Type())
}

func TestConfirmedSuccessEvent_TypeString(t *testing.T) {
	event := &ConfirmedSuccessEvent{}
	assert.Equal(t, "Event_ConfirmedSuccess", event.TypeString())
}

func TestConfirmedSuccessEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &ConfirmedSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestConfirmedRevertedEvent_Type(t *testing.T) {
	event := &ConfirmedRevertedEvent{}
	assert.Equal(t, Event_ConfirmedReverted, event.Type())
}

func TestConfirmedRevertedEvent_TypeString(t *testing.T) {
	event := &ConfirmedRevertedEvent{}
	assert.Equal(t, "Event_ConfirmedReverted", event.TypeString())
}

func TestConfirmedRevertedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	revertReason := pldtypes.HexBytes{0x01, 0x02, 0x03}
	event := &ConfirmedRevertedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		RevertReason: revertReason,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, revertReason, event.RevertReason)
}

func TestCreatedEvent_Type(t *testing.T) {
	event := &CreatedEvent{}
	assert.Equal(t, Event_Created, event.Type())
}

func TestCreatedEvent_TypeString(t *testing.T) {
	event := &CreatedEvent{}
	assert.Equal(t, "Event_Created", event.TypeString())
}

func TestCreatedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	privateTx := &components.PrivateTransaction{
		ID: txID,
	}
	event := &CreatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		PrivateTransaction: privateTx,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, privateTx, event.PrivateTransaction)
}

func TestDelegatedEvent_Type(t *testing.T) {
	event := &DelegatedEvent{}
	assert.Equal(t, Event_Delegated, event.Type())
}

func TestDelegatedEvent_TypeString(t *testing.T) {
	event := &DelegatedEvent{}
	assert.Equal(t, "Event_Delegated", event.TypeString())
}

func TestDelegatedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	coordinator := "coordinator@testNode"
	event := &DelegatedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		Coordinator: coordinator,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, coordinator, event.GetCoordinator())
}

func TestAssembleRequestReceivedEvent_Type(t *testing.T) {
	event := &AssembleRequestReceivedEvent{}
	assert.Equal(t, Event_AssembleRequestReceived, event.Type())
}

func TestAssembleRequestReceivedEvent_TypeString(t *testing.T) {
	event := &AssembleRequestReceivedEvent{}
	assert.Equal(t, "Event_AssembleRequestReceived", event.TypeString())
}

func TestAssembleRequestReceivedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	coordinator := "coordinator@testNode"
	blockHeight := int64(100)

	event := &AssembleRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		RequestID:              requestID,
		Coordinator:            coordinator,
		CoordinatorBlockHeight: blockHeight,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, requestID, event.RequestID)
	assert.Equal(t, coordinator, event.Coordinator)
	assert.Equal(t, blockHeight, event.CoordinatorBlockHeight)
}

func TestAssembleSuccessEvent_Type(t *testing.T) {
	event := &AssembleSuccessEvent{}
	assert.Equal(t, Event_AssembleSuccess, event.Type())
}

func TestAssembleSuccessEvent_TypeString(t *testing.T) {
	event := &AssembleSuccessEvent{}
	assert.Equal(t, "Event_AssembleSuccess", event.TypeString())
}

func TestAssembleSuccessEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	postAssembly := &components.TransactionPostAssembly{}

	event := &AssembleSuccessEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		PostAssembly: postAssembly,
		RequestID:    requestID,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, postAssembly, event.PostAssembly)
	assert.Equal(t, requestID, event.RequestID)
}

func TestAssembleRevertEvent_Type(t *testing.T) {
	event := &AssembleRevertEvent{}
	assert.Equal(t, Event_AssembleRevert, event.Type())
}

func TestAssembleRevertEvent_TypeString(t *testing.T) {
	event := &AssembleRevertEvent{}
	assert.Equal(t, "Event_AssembleRevert", event.TypeString())
}

func TestAssembleRevertEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	postAssembly := &components.TransactionPostAssembly{}

	event := &AssembleRevertEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		PostAssembly: postAssembly,
		RequestID:    requestID,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, postAssembly, event.PostAssembly)
	assert.Equal(t, requestID, event.RequestID)
}

func TestAssembleParkEvent_Type(t *testing.T) {
	event := &AssembleParkEvent{}
	assert.Equal(t, Event_AssemblePark, event.Type())
}

func TestAssembleParkEvent_TypeString(t *testing.T) {
	event := &AssembleParkEvent{}
	assert.Equal(t, "Event_AssemblePark", event.TypeString())
}

func TestAssembleParkEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	postAssembly := &components.TransactionPostAssembly{}

	event := &AssembleParkEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		PostAssembly: postAssembly,
		RequestID:    requestID,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, postAssembly, event.PostAssembly)
	assert.Equal(t, requestID, event.RequestID)
}

func TestAssembleErrorEvent_Type(t *testing.T) {
	event := &AssembleErrorEvent{}
	assert.Equal(t, Event_AssembleError, event.Type())
}

func TestAssembleErrorEvent_TypeString(t *testing.T) {
	event := &AssembleErrorEvent{}
	assert.Equal(t, "Event_AssembleError", event.TypeString())
}

func TestPreDispatchRequestReceivedEvent_Type(t *testing.T) {
	event := &PreDispatchRequestReceivedEvent{}
	assert.Equal(t, Event_PreDispatchRequestReceived, event.Type())
}

func TestPreDispatchRequestReceivedEvent_TypeString(t *testing.T) {
	event := &PreDispatchRequestReceivedEvent{}
	assert.Equal(t, "Event_PreDispatchRequestReceived", event.TypeString())
}

func TestPreDispatchRequestReceivedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	coordinator := "coordinator@testNode"

	event := &PreDispatchRequestReceivedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		RequestID:   requestID,
		Coordinator: coordinator,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, requestID, event.RequestID)
	assert.Equal(t, coordinator, event.Coordinator)
}

func TestDispatchedEvent_Type(t *testing.T) {
	event := &DispatchedEvent{}
	assert.Equal(t, Event_Dispatched, event.Type())
}

func TestDispatchedEvent_TypeString(t *testing.T) {
	event := &DispatchedEvent{}
	assert.Equal(t, "Event_Dispatched", event.TypeString())
}

func TestDispatchedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	signerAddress := *pldtypes.RandAddress()
	coordinator := "coordinator@node1"
	event := &DispatchedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		SignerAddress: signerAddress,
		Coordinator:   coordinator,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, signerAddress, event.SignerAddress)
	assert.Equal(t, coordinator, event.GetCoordinator())
}

func TestNonceAssignedEvent_Type(t *testing.T) {
	event := &NonceAssignedEvent{}
	assert.Equal(t, Event_NonceAssigned, event.Type())
}

func TestNonceAssignedEvent_TypeString(t *testing.T) {
	event := &NonceAssignedEvent{}
	assert.Equal(t, "Event_NonceAssigned", event.TypeString())
}

func TestNonceAssignedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	signerAddress := *pldtypes.RandAddress()
	nonce := uint64(42)
	event := &NonceAssignedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		SignerAddress: signerAddress,
		Nonce:         nonce,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, signerAddress, event.SignerAddress)
	assert.Equal(t, nonce, event.Nonce)
}

func TestSubmittedEvent_Type(t *testing.T) {
	event := &SubmittedEvent{}
	assert.Equal(t, Event_Submitted, event.Type())
}

func TestSubmittedEvent_TypeString(t *testing.T) {
	event := &SubmittedEvent{}
	assert.Equal(t, "Event_Submitted", event.TypeString())
}

func TestSubmittedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	signerAddress := *pldtypes.RandAddress()
	nonce := uint64(42)
	submissionHash := pldtypes.RandBytes32()
	event := &SubmittedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
		SignerAddress:        signerAddress,
		Nonce:                nonce,
		LatestSubmissionHash: submissionHash,
	}
	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, signerAddress, event.SignerAddress)
	assert.Equal(t, nonce, event.Nonce)
	assert.Equal(t, submissionHash, event.LatestSubmissionHash)
}

func TestResumedEvent_Type(t *testing.T) {
	event := &ResumedEvent{}
	assert.Equal(t, Event_Resumed, event.Type())
}

func TestResumedEvent_TypeString(t *testing.T) {
	event := &ResumedEvent{}
	assert.Equal(t, "Event_Resumed", event.TypeString())
}

func TestResumedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &ResumedEvent{
		BaseEvent: BaseEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestEvent_InterfaceCompliance(t *testing.T) {
	// Test that all events implement the Event interface
	txID := uuid.New()
	events := []Event{
		&ConfirmedSuccessEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&ConfirmedRevertedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&CreatedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&DelegatedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&AssembleRequestReceivedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&AssembleSuccessEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&AssembleRevertEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&AssembleParkEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&AssembleErrorEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&PreDispatchRequestReceivedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&DispatchedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&NonceAssignedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&SubmittedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
		&ResumedEvent{
			BaseEvent: BaseEvent{TransactionID: txID},
		},
	}

	for _, event := range events {
		// Verify that Type() returns a valid EventType
		eventType := event.Type()
		assert.NotNil(t, eventType)

		// Verify that TypeString() returns a non-empty string
		typeString := event.TypeString()
		assert.NotEmpty(t, typeString)

		// Verify that GetTransactionID() returns the correct ID
		assert.Equal(t, txID, event.GetTransactionID())

		// Verify that GetEventTime() is callable
		_ = event.GetEventTime()
	}
}

func TestEvent_GetEventTime(t *testing.T) {
	eventTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	txID := uuid.New()

	events := []Event{
		&ConfirmedSuccessEvent{
			BaseEvent: BaseEvent{
				BaseEvent: common.BaseEvent{
					EventTime: eventTime,
				},
				TransactionID: txID,
			},
		},
		&DelegatedEvent{
			BaseEvent: BaseEvent{
				BaseEvent: common.BaseEvent{
					EventTime: eventTime,
				},
				TransactionID: txID,
			},
		},
		&AssembleRequestReceivedEvent{
			BaseEvent: BaseEvent{
				BaseEvent: common.BaseEvent{
					EventTime: eventTime,
				},
				TransactionID: txID,
			},
		},
	}

	for _, event := range events {
		assert.Equal(t, eventTime, event.GetEventTime())
	}
}
