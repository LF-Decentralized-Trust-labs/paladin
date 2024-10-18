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

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TransactionHelper struct {
	ctx     context.Context
	t       *testing.T
	tb      testbed.Testbed
	builder pldclient.TransactionBuilder
}

type DomainTransactionHelper struct {
	ctx context.Context
	t   *testing.T
	rpc rpcbackend.Backend
	tx  *tktypes.PrivateContractInvoke
}

type SentDomainTransaction struct {
	t      *testing.T
	result chan any
}

func NewPaladinClient(t *testing.T, ctx context.Context, tb testbed.Testbed) pldclient.PaladinClient {
	c, err := pldclient.New().
		ReceiptPollingInterval(200*time.Millisecond).
		HTTP(ctx, &pldconf.HTTPClientConfig{
			URL: fmt.Sprintf("http://%s", tb.Components().RPCServer().HTTPAddr()),
		})
	require.NoError(t, err)
	return c
}

func NewTransactionHelper(ctx context.Context, t *testing.T, tb testbed.Testbed, builder pldclient.TransactionBuilder) *TransactionHelper {
	return &TransactionHelper{ctx: ctx, t: t, tb: tb, builder: builder}
}

func (th *TransactionHelper) SignAndSend(signer string) pldclient.SentTransaction {
	stx, err := th.builder.Public().From(signer).SendTX()
	require.NoError(th.t, err)
	return stx
}

func (th *TransactionHelper) Prepare() tktypes.HexBytes {
	b, err := th.builder.BuildCallData()
	require.NoError(th.t, err)
	return b
}

func (th *TransactionHelper) FindEvent(txHash *tktypes.Bytes32, abi abi.ABI, eventName string, eventParams any) *blockindexer.EventWithData {
	targetEvent := abi.Events()[eventName]
	assert.NotNil(th.t, targetEvent)
	assert.NotEmpty(th.t, targetEvent.SolString())
	events, err := th.tb.Components().BlockIndexer().DecodeTransactionEvents(th.ctx, *txHash, abi)
	assert.NoError(th.t, err)
	for _, event := range events {
		if event.SoliditySignature == targetEvent.SolString() {
			err = json.Unmarshal(event.Data, eventParams)
			assert.NoError(th.t, err)
			return event
		}
	}
	return nil
}

func NewDomainTransactionHelper(ctx context.Context, t *testing.T, rpc rpcbackend.Backend, to *tktypes.EthAddress, fn *abi.Entry, inputs tktypes.RawJSON) *DomainTransactionHelper {
	return &DomainTransactionHelper{
		ctx: ctx,
		t:   t,
		rpc: rpc,
		tx: &tktypes.PrivateContractInvoke{
			To:       *to,
			Function: *fn,
			Inputs:   inputs,
		},
	}
}

func (dth *DomainTransactionHelper) SignAndSend(signer string, confirm ...bool) *SentDomainTransaction {
	tx := &SentDomainTransaction{
		t:      dth.t,
		result: make(chan any),
	}
	dth.tx.From = signer
	confirmEvents := false
	if len(confirm) > 0 {
		confirmEvents = confirm[0]
	}
	go func() {
		var result any
		rpcerr := dth.rpc.CallRPC(dth.ctx, &result, "testbed_invoke", dth.tx, confirmEvents)
		if rpcerr != nil && rpcerr.Error() != nil {
			tx.result <- rpcerr.Error()
		}
		tx.result <- result
	}()
	return tx
}

func (dth *DomainTransactionHelper) Prepare(signer string) *tktypes.PrivateContractTransaction {
	var result tktypes.PrivateContractTransaction
	dth.tx.From = signer
	rpcerr := dth.rpc.CallRPC(dth.ctx, &result, "testbed_prepare", dth.tx)
	if rpcerr != nil {
		require.NoError(dth.t, rpcerr.Error())
	}
	return &result
}

func (st *SentDomainTransaction) Wait() {
	result := <-st.result
	switch r := result.(type) {
	case error:
		require.NoError(st.t, r)
	default:
	}
}

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	assert.NoError(t, err)
	return result
}

func deployBuilder(ctx context.Context, t *testing.T, pld pldclient.PaladinClient, abi abi.ABI, bytecode []byte) pldclient.TransactionBuilder {
	abiClient, err := pld.ABI(ctx, abi)
	assert.NoError(t, err)
	construct, err := abiClient.Constructor(ctx, bytecode)
	assert.NoError(t, err)
	return construct.TXBuilder(ctx)
}

func functionBuilder(ctx context.Context, t *testing.T, pld pldclient.PaladinClient, abi abi.ABI, functionName string) pldclient.TransactionBuilder {
	abiClient, err := pld.ABI(ctx, abi)
	assert.NoError(t, err)
	fn, err := abiClient.Function(ctx, functionName)
	assert.NoError(t, err)
	return fn.TXBuilder(ctx)
}
