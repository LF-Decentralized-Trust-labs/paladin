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
	"net/http"

	"github.com/hyperledger/firefly-transaction-manager/pkg/apitypes"
	btxManager "github.com/kaleido-io/paladin-blockchain-transaction-manager/pkg/manager"
)

type TransactionArgs struct {
	From   string
	To     string
	Amount float64
}

type TransactionReply struct {
	Status string
}

type PaladinTransactionManager interface {
	// TBC, interface below are just examples, should be replaced by real interfaces in the future
	SubmitTransaction(r *http.Request, args *TransactionArgs, reply *TransactionReply) error
}

type PaladinService struct {
	blockchainTransactionManager btxManager.BlockchainTransactionManager
}

func (b *PaladinService) SubmitTransaction(r *http.Request, args *TransactionArgs, reply *TransactionReply) error {
	// Dummy implementation for demonstration purposes
	_, _, err := b.blockchainTransactionManager.HandleNewTransaction(r.Context(), &apitypes.TransactionRequest{})
	reply.Status = "Mock failure"
	return err
}

func NewPaladinService(ctx context.Context, btm btxManager.BlockchainTransactionManager) (*PaladinService, error) {
	return &PaladinService{
		blockchainTransactionManager: btm,
	}, nil
}
