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

package server

import (
	"context"

	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	btxManager "github.com/kaleido-io/paladin-blockchain-transaction-manager/pkg/manager"
	pldManager "github.com/kaleido-io/paladin-transaction-manager/pkg/manager"
)

func NewRPCServer(ctx context.Context) (*rpc.Server, error) {
	// Create a new RPC server
	s := rpc.NewServer()
	s.RegisterCodec(json2.NewCodec(), "application/json")

	// Create a blockchain transaction manager
	blockchainTransactionManager := btxManager.NewBlockchainTxManager(ctx)

	// Create a paladin transaction manager service
	pldTxMgrService := pldManager.NewPaladinService(ctx, blockchainTransactionManager)

	// Register the BlockchainService
	_ = s.RegisterService(pldTxMgrService, "pld" /**subject name of the JSON rpc method, e.g. pld.SubmitTransaction**/)

	// if err != nil {
	// 	return nil, err
	// }
	return s, nil
}
