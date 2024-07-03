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
	"errors"
	"net/http"
	"testing"

	"github.com/hyperledger/firefly-transaction-manager/pkg/apitypes"
	btxmMocks "github.com/kaleido-io/paladin-transaction-manager/mocks/btxmMocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPaladinService_SubmitTransaction(t *testing.T) {
	mockManager := &btxmMocks.BlockchainTransactionManager{}

	mockManager.On("HandleNewTransaction", mock.Anything, mock.Anything).Return(&apitypes.ManagedTX{}, false, errors.New("mock failure"))

	service := PaladinService{
		blockchainTransactionManager: mockManager,
	}

	request := &http.Request{}
	args := &TransactionArgs{
		From:   "Alice",
		To:     "Bob",
		Amount: 10.5,
	}
	reply := &TransactionReply{}

	err := service.SubmitTransaction(request, args, reply)

	mockManager.AssertCalled(t, "HandleNewTransaction", mock.Anything, mock.Anything)

	assert.Error(t, err, "SubmitTransaction should return an error")
	assert.Equal(t, "Mock failure", reply.Status, "Status should be set to 'Mock failure'")
}

func TestNewPaladinService(t *testing.T) {
	mockManager := &btxmMocks.BlockchainTransactionManager{}

	service := NewPaladinService(context.Background(), mockManager)

	assert.NotNil(t, service, "NewPaladinService should return a non-nil service instance")
}
