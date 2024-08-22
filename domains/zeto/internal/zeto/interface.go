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

package zeto

import (
	"context"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
)

type DomainInterface map[string]*DomainEntry

type DomainEntry struct {
	ABI     *abi.Entry
	handler DomainHandler
}

type DomainHandler interface {
	ValidateParams(params string) (interface{}, error)
	Init(ctx context.Context, tx *parsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error)
	Assemble(ctx context.Context, tx *parsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error)
	Endorse(ctx context.Context, tx *parsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error)
	Prepare(ctx context.Context, tx *parsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error)
}

type domainHandler struct {
	Zeto *Zeto
}

func (d *Zeto) getInterface() DomainInterface {
	iface := DomainInterface{
		"constructor": {
			ABI: &abi.Entry{
				Type: abi.Constructor,
				Inputs: abi.ParameterArray{
					{Name: "notary", Type: "string"},
				},
			},
		},
		// TODO: add methods
	}

	return iface
}

type ZetoConstructorParams struct {
	Notary string `json:"notary"`
}
