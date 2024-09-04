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

package domain

import (
	"context"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type DomainInterface[C any] map[string]*DomainEntry[C]

type DomainEntry[C any] struct {
	ABI     *abi.Entry
	Handler DomainHandler[C]
}

type ParsedTransaction[C any] struct {
	Transaction     *pb.TransactionSpecification
	FunctionABI     *abi.Entry
	ContractAddress *ethtypes.Address0xHex
	DomainConfig    *C
	Params          interface{}
}

type DomainHandler[C any] interface {
	ValidateParams(params string) (interface{}, error)
	Init(ctx context.Context, tx *ParsedTransaction[C], req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error)
	Assemble(ctx context.Context, tx *ParsedTransaction[C], req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error)
	Endorse(ctx context.Context, tx *ParsedTransaction[C], req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error)
	Prepare(ctx context.Context, tx *ParsedTransaction[C], req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error)
}
