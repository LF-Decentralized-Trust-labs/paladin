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

	"github.com/hyperledger/firefly-transaction-manager/pkg/apitypes"
)

type BlockchainTransactionManager interface {
	// TBC, interface below are just examples, this should goes into FFTM repo instead as an alternative of its REST API
	Init(ctx context.Context) error
	HandleNewTransaction(ctx context.Context, txReq *apitypes.TransactionRequest) (mtx *apitypes.ManagedTX, submissionRejected bool, err error)
}

type BlockchainTxManager struct{} // mock example manager, to be replaced by a real one

func (b *BlockchainTxManager) Init(ctx context.Context) error {
	return nil
}

func (b *BlockchainTxManager) HandleNewTransaction(ctx context.Context, txReq *apitypes.TransactionRequest) (mtx *apitypes.ManagedTX, submissionRejected bool, err error) {
	return nil, false, errors.New("mock failure")
}

func NewBlockchainTxManager(context.Context) (*BlockchainTxManager, error) {
	return &BlockchainTxManager{}, nil
}
