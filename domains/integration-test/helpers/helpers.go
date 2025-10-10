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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TransactionHelper struct {
	ctx     context.Context
	t       *testing.T
	tb      testbed.Testbed
	builder pldclient.TxBuilder
}

type DomainTransactionHelper struct {
	ctx context.Context
	t   *testing.T
	rpc rpcclient.Client
	tx  *pldapi.TransactionInput
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

func NewTransactionHelper(ctx context.Context, t *testing.T, tb testbed.Testbed, builder pldclient.TxBuilder) *TransactionHelper {
	return &TransactionHelper{ctx: ctx, t: t, tb: tb, builder: builder}
}

func (th *TransactionHelper) SignAndSend(signer string) pldclient.SentTransaction {
	stx := th.builder.Public().From(signer).Send()
	require.NoError(th.t, stx.Error())
	return stx
}

func (th *TransactionHelper) Prepare() pldtypes.HexBytes {
	b, err := th.builder.BuildCallData()
	require.NoError(th.t, err)
	return b
}

func (th *TransactionHelper) FindEvent(txHash *pldtypes.Bytes32, abi abi.ABI, eventName string, eventParams any) *pldapi.EventWithData {
	targetEvent := abi.Events()[eventName]
	assert.NotNil(th.t, targetEvent)
	assert.NotEmpty(th.t, targetEvent.SolString())
	events, err := th.tb.Components().BlockIndexer().DecodeTransactionEvents(th.ctx, *txHash, abi, "")
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

func NewDomainTransactionHelper(ctx context.Context, t *testing.T, rpc rpcclient.Client, to *pldtypes.EthAddress, fn *abi.Entry, inputs pldtypes.RawJSON) *DomainTransactionHelper {
	return &DomainTransactionHelper{
		ctx: ctx,
		t:   t,
		rpc: rpc,
		tx: &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				To:   to,
				Data: inputs,
			},
			ABI: abi.ABI{fn},
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
		if rpcerr != nil {
			tx.result <- rpcerr.Error()
		}
		tx.result <- result
	}()
	return tx
}

func (dth *DomainTransactionHelper) SignAndCall(signer string) *SentDomainTransaction {
	tx := &SentDomainTransaction{
		t:      dth.t,
		result: make(chan any),
	}
	dth.tx.From = signer
	go func() {
		var result any
		rpcerr := dth.rpc.CallRPC(dth.ctx, &result, "testbed_call", dth.tx, pldtypes.JSONFormatOptions(""))
		if rpcerr != nil {
			tx.result <- rpcerr.Error()
		}
		tx.result <- result
	}()
	return tx
}

func (dth *DomainTransactionHelper) Prepare(signer string) *testbed.TransactionResult {
	var result testbed.TransactionResult
	dth.tx.From = signer
	rpcerr := dth.rpc.CallRPC(dth.ctx, &result, "testbed_prepare", dth.tx)
	if rpcerr != nil {
		require.NoError(dth.t, rpcerr)
	}
	return &result
}

func (st *SentDomainTransaction) Wait() map[string]any {
	result := <-st.result
	switch r := result.(type) {
	case error:
		require.NoError(st.t, r)
	case string:
		require.Fail(st.t, "RPC error", r)
	case map[string]any:
		return r
	default:
	}
	return nil
}

func toJSON(t *testing.T, v any) []byte {
	result, err := json.Marshal(v)
	assert.NoError(t, err)
	return result
}

func deployBuilder(ctx context.Context, pld pldclient.PaladinClient, abi abi.ABI, bytecode []byte) pldclient.TxBuilder {
	return pld.ForABI(ctx, abi).Constructor().Bytecode(bytecode)
}

func functionBuilder(ctx context.Context, pld pldclient.PaladinClient, abi abi.ABI, functionName string) pldclient.TxBuilder {
	return pld.ForABI(ctx, abi).Function(functionName)
}
