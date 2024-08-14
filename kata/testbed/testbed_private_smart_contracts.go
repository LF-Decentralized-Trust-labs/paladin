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

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type testbedPrivateSmartContract struct {
	tb      *testbed
	domain  *testbedDomain
	address *ethtypes.Address0xHex
}

func (psc *testbedPrivateSmartContract) validateInvoke(ctx context.Context, invocation *types.PrivateContractInvoke) (*uuid.UUID, *proto.TransactionSpecification, error) {

	if invocation.From == "" {
		return nil, nil, fmt.Errorf("no from address specified for transaction")
	}

	functionABI := &invocation.Function

	confirmedBlockHeight, err := psc.tb.blockindexer.GetConfirmedBlockHeight(ctx)
	if err != nil {
		return nil, nil, err
	}

	functionParams, err := functionABI.Inputs.ParseJSONCtx(ctx, invocation.Inputs)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid parameters for function %s: %s", functionABI.SolString(), err)
	}

	functionABIJSON, _ := json.Marshal(functionABI)
	functionParamsJSON, _ := types.StandardABISerializer().SerializeJSONCtx(ctx, functionParams)

	txID := uuid.New()
	return &txID, &proto.TransactionSpecification{
		TransactionId:      uuidToHexBytes32(txID).String(),
		From:               invocation.From,
		ContractAddress:    psc.address.String(),
		FunctionAbiJson:    string(functionABIJSON),
		FunctionSignature:  functionABI.String(),
		FunctionParamsJson: string(functionParamsJSON),
		BaseBlock:          int64(confirmedBlockHeight),
	}, nil
}
