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

package pldclient

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
)

// General builder pattern approaches:
// - Context and errors propagate with chaining
// - Error deferred from return into a chainable object wherever chaining is likely
// - Setters on the builder have no prefix, as that's the primary job of the builder
// - Getters are also provided, and have a "Get" prefix to access them

type Chainable interface {
	GetCtx() context.Context
	Client() PaladinClient
	Error() error // if an error has happened at any point that was not returned, it is returned here
}

type TxBuilder interface {
	Chainable

	Public() TxBuilder  // a public transaction to submit directly to the base ledger
	Private() TxBuilder // a private transaction that will manage selective disclosure of state via a Paladin domain
	GetType() pldapi.TransactionType

	SolidityBuild(build *solutils.SolidityBuild) TxBuilder // sets the ABI and Bytecode from a Solidity build

	ABI(a abi.ABI) TxBuilder             // sets the ABI to use locally for function processing and pass to Paladin on TX submission - processes it locally to aid lookups and will defer an error if ABI is invalid
	ABIJSON(abiJson []byte) TxBuilder    // can result in a deferred error being stored if the ABI JSON is invalid, or the contained ABI is invalid
	ABIFunction(fn *abi.Entry) TxBuilder // sets the ABI and Function with a single call (overwrites any previous ABI)
	GetABI() abi.ABI

	Constructor() TxBuilder       // shortcut to Function("").To(nil)
	Function(fn string) TxBuilder // fhe name or full signature of a function - can be left unset for a constructor
	GetFunction() string

	ABIReference(hash *pldtypes.Bytes32) TxBuilder
	GetABIReference() *pldtypes.Bytes32 // not calculated client set when an ABI is used - only returns non-nil if set explicitly

	IdempotencyKey(idempotencyKey string) TxBuilder // A unique identifier for your business transaction, allowing exactly-once submission over the JSON/RPC API
	GetIdempotencyKey() string

	From(from string) TxBuilder // the identifier for the signing address to use to create the signature that authorizes the transaction
	GetFrom() string

	To(to *pldtypes.EthAddress) TxBuilder // the contract address to send the transaction to, or nil for a constructor
	GetTo() *pldtypes.EthAddress

	Bytecode(bytecode []byte) TxBuilder // for public transaction constructors this is required (not applicable to private transactions directly - Pente is a special case handled separately)
	GetBytecode() pldtypes.HexBytes

	Domain(domain string) TxBuilder // for private transaction constructors the domain must be specified. It is optional for private transactions as it will be inferred from the to address
	GetDomain() string

	Inputs(inputs any) TxBuilder // can be string and pldtypes.RawJSON are interpreted as JSON, abi.ComponentValue trees can be used, and any other type will be serialized to JSON then parsed against the ABI inputs. Errors processing this input against the ABI function definition are deferred
	GetInputs() any

	Outputs(outputs any) TxBuilder // only used for call - must be a pointer to the place to store the return value. Same type rules as Inputs
	GetOutputs() any

	PublicTxOptions(opts pldapi.PublicTxOptions) TxBuilder // detailed options of how to submit the public / base ledger transaction that results from the public/private transaction
	GetPublicTxOptions() pldapi.PublicTxOptions

	PublicCallOptions(opts pldapi.PublicCallOptions) TxBuilder // allows you to set the block number the call happens against, when running queries directly against the base ledger
	GetPublicCallOptions() pldapi.PublicCallOptions

	DataFormat(format pldtypes.JSONFormatOptions) TxBuilder // determines how JSON will be sent/received to/from the server as serialized JSON
	GetDataFormat() pldtypes.JSONFormatOptions

	Clone() TxBuilder                           // creates a copy that is useful as a way to create a common reference builder for multiple calls
	Wrap(*pldapi.TransactionInput) TxBuilder    // initializes a TxBuilder from an existing transaction, including setting the inputs to be the Data from the TX
	WrapCall(*pldapi.TransactionCall) TxBuilder // initializes a TxBuilder from an existing call, including setting the inputs to be the Data from the TX

	ResolveDefinition() (*abi.Entry, error)                                // resolves the function/constructor client-side against the ABI and returns the full definition
	BuildCallData() (callData pldtypes.HexBytes, err error)                // builds binary call data, useful for various low level functions on ABI
	BuildInputDataCV() (def *abi.Entry, cv *abi.ComponentValue, err error) // build the intermediate abi.ComponentValue tree for the inputs
	BuildInputDataJSON() (jsonData pldtypes.RawJSON, err error)            // build the input data as JSON (object by default, with serialization options via Serializer())
	BuildTX() SendableTransaction                                          // builds the full TransactionInput object for use with Paladin - copies the TX so the builder can be re-used safely
	Send() SentTransaction                                                 // shortcut to BuildTX() then Send() with a chainable result (errors deferred)
	Prepare() PreparingTransaction                                         // shortcut to BuildTX() then Prepare() with a chainable result (errors deferred)
	Call() error                                                           // shortcut to BuildTX() then Call()
}

type SendableTransaction interface {
	Chainable

	TX() *pldapi.TransactionInput    // get the transaction directly to use
	CallTX() *pldapi.TransactionCall // get the call version of the transaction directly to use
	Error() error                    // deferred error
	Send() SentTransaction           // sends the transaction and builds a wrapper around the returned ID, including handling idempotency key conflicts
	Prepare() PreparingTransaction   // requests the node to prepare the transaction, but not submit it to the chain. Once prepared the transaction ready fro submission can be obtained to send later
	Call() error                     // performs a call, storing the returned value back into the outputs (as JSON per the DataFormat if a string/[]byte/RawJSON is provided, otherwise un-marshalling to your value)
}

type SentTransaction interface {
	Chainable

	ID() *uuid.UUID                                  // nil if there was an error
	Wait(timeout time.Duration) TransactionResult    // chainable
	Error() error                                    // get any deferred error
	GetTransaction() (*pldapi.Transaction, error)    // calls ptx_getTransaction
	GetReceipt() (*pldapi.TransactionReceipt, error) // calls ptx_getTransactionReceipt
}

type TransactionResult interface {
	Chainable

	ID() uuid.UUID
	TransactionHash() *pldtypes.Bytes32  // non-nil if this made it to the chain - which is possible when error is true, for revert cases
	Error() error                        // could be a failure to submit, or an error at any point up to and including execution reversion on-chain
	Receipt() *pldapi.TransactionReceipt // if nil, then error is guaranteed to be non-nil
}

type PreparingTransaction interface {
	Chainable

	ID() *uuid.UUID                                               // nil if there was an error
	Wait(timeout time.Duration) PreparedTransactionResult         // chainable
	Error() error                                                 // get any deferred error
	GetTransaction() (*pldapi.Transaction, error)                 // calls ptx_getTransaction
	GetPreparedTransaction() (*pldapi.PreparedTransaction, error) // calls ptx_getPreparedTransaction
}

type PreparedTransactionResult interface {
	Chainable

	ID() uuid.UUID
	Error() error                                     // could be a failure to prepare, or an error at any point up to and including execution reversion on-chain
	PreparedTransaction() *pldapi.PreparedTransaction // if nil, then error is guaranteed to be non-nil
}

var defaultConstructor = &abi.Entry{Type: abi.Constructor, Inputs: abi.ParameterArray{}}

type chainable struct {
	c           *paladinClient
	ctx         context.Context
	deferredErr error
}

type txBuilder struct {
	chainable
	functions map[string]*abi.Entry
	tx        *pldapi.TransactionCall
	inputs    any
	outputs   any
}

type sendableTransaction struct {
	chainable
	tx      *pldapi.TransactionCall
	outputs any
}

type sentTransaction struct {
	chainable
	txID *uuid.UUID
}

type preparingTransaction struct {
	chainable
	txID *uuid.UUID
}

type transactionResult struct {
	chainable
	txID    uuid.UUID
	receipt *pldapi.TransactionReceipt
}

type preparedTransactionResult struct {
	chainable
	txID     uuid.UUID
	prepared *pldapi.PreparedTransaction
}

func (wc *chainable) GetCtx() context.Context {
	return wc.ctx
}

func (wc *chainable) Client() PaladinClient {
	return wc.c
}

func (wc *chainable) deferError(err error) {
	if err != nil {
		log.L(wc.GetCtx()).Errorf("deferred error: %s", err)
		if wc.deferredErr == nil {
			wc.deferredErr = err // first error wins
		}
	}
}

func (wc *chainable) Error() error {
	return wc.deferredErr
}

func (c *paladinClient) TxBuilder(ctx context.Context) TxBuilder {
	return &txBuilder{
		// Root of a chain
		chainable: chainable{
			ctx: ctx,
			c:   c,
		},
		functions: map[string]*abi.Entry{},
		tx:        &pldapi.TransactionCall{},
	}
}

func (t *txBuilder) Clone() TxBuilder {
	txCopy := *t.tx
	return &txBuilder{
		chainable: t.chainable,
		tx:        &txCopy,
		functions: t.functions,
		inputs:    t.inputs,
		outputs:   t.outputs,
	}
}

func (c *paladinClient) ForABI(ctx context.Context, a abi.ABI) TxBuilder {
	return c.TxBuilder(ctx).ABI(a)
}

func (t *txBuilder) ABI(a abi.ABI) TxBuilder {
	t.tx.ABI = a
	t.functions = map[string]*abi.Entry{}
	for _, e := range a {
		s, err := e.SignatureCtx(t.ctx)
		if err != nil {
			t.deferError(err)
			return t
		}
		if e.Name != "" && e.IsFunction() {
			for i, o := range e.Outputs {
				if o.Name == "" {
					o.Name = strconv.Itoa(i)
				}
			}
			t.functions[e.Name] = e
			t.functions[s] = e
		}
	}
	return t
}

func (t *txBuilder) ABIFunction(fn *abi.Entry) TxBuilder {
	return t.ABI(abi.ABI{fn}).Function(fn.String())
}

func (t *txBuilder) ABIJSON(abiJson []byte) TxBuilder {
	var a abi.ABI
	err := json.Unmarshal(abiJson, &a)
	if err != nil {
		t.deferError(i18n.WrapError(t.ctx, err, pldmsgs.MsgPaladinClientABIJson))
		return t
	}
	return t.ABI(a)
}

func (t *txBuilder) ABIReference(hash *pldtypes.Bytes32) TxBuilder {
	t.tx.ABIReference = hash
	return t
}

func (t *txBuilder) Bytecode(b []byte) TxBuilder {
	t.tx.Bytecode = b
	return t
}

func (t *txBuilder) Domain(domain string) TxBuilder {
	t.tx.Domain = domain
	return t
}

func (t *txBuilder) From(from string) TxBuilder {
	t.tx.From = from
	return t
}

func (t *txBuilder) Constructor() TxBuilder {
	return t.Function("").To(nil)
}

func (t *txBuilder) Function(fn string) TxBuilder {
	t.tx.Function = fn
	return t
}

func (t *txBuilder) GetABI() abi.ABI {
	return t.tx.ABI
}

func (t *txBuilder) GetABIReference() *pldtypes.Bytes32 {
	return t.tx.ABIReference
}

func (t *txBuilder) GetBytecode() pldtypes.HexBytes {
	return t.tx.Bytecode
}

func (t *txBuilder) GetDomain() string {
	return t.tx.Domain
}

func (t *txBuilder) GetFrom() string {
	return t.tx.From
}

func (t *txBuilder) GetFunction() string {
	return t.tx.Function
}

func (t *txBuilder) GetIdempotencyKey() string {
	return t.tx.IdempotencyKey
}

func (t *txBuilder) GetInputs() any {
	return t.inputs
}

func (t *txBuilder) GetOutputs() any {
	return t.outputs
}

func (t *txBuilder) GetDataFormat() pldtypes.JSONFormatOptions {
	return t.tx.DataFormat
}

func (t *txBuilder) GetPublicTxOptions() pldapi.PublicTxOptions {
	return t.tx.PublicTxOptions
}

func (t *txBuilder) GetPublicCallOptions() pldapi.PublicCallOptions {
	return t.tx.PublicCallOptions
}

func (t *txBuilder) GetTo() *pldtypes.EthAddress {
	return t.tx.To
}

func (t *txBuilder) GetType() pldapi.TransactionType {
	return t.tx.Type.V()
}

func (t *txBuilder) IdempotencyKey(idempotencyKey string) TxBuilder {
	t.tx.IdempotencyKey = idempotencyKey
	return t
}

func (t *txBuilder) Inputs(inputs any) TxBuilder {
	t.inputs = inputs
	return t
}

func (t *txBuilder) Outputs(outputs any) TxBuilder {
	t.outputs = outputs
	return t
}

func (t *txBuilder) DataFormat(format pldtypes.JSONFormatOptions) TxBuilder {
	t.tx.DataFormat = format
	return t
}

func (t *txBuilder) Private() TxBuilder {
	t.tx.Type = pldapi.TransactionTypePrivate.Enum()
	return t
}

func (t *txBuilder) Public() TxBuilder {
	t.tx.Type = pldapi.TransactionTypePublic.Enum()
	return t
}

func (t *txBuilder) PublicTxOptions(opts pldapi.PublicTxOptions) TxBuilder {
	t.tx.PublicTxOptions = opts
	return t
}

func (t *txBuilder) PublicCallOptions(opts pldapi.PublicCallOptions) TxBuilder {
	t.tx.PublicCallOptions = opts
	return t
}

func (t *txBuilder) SolidityBuild(build *solutils.SolidityBuild) TxBuilder {
	return t.ABI(build.ABI).Bytecode(build.Bytecode)
}

func (t *txBuilder) To(to *pldtypes.EthAddress) TxBuilder {
	t.tx.To = to
	return t
}

func (t *txBuilder) Wrap(tx *pldapi.TransactionInput) TxBuilder {
	t.tx = &pldapi.TransactionCall{
		TransactionInput: *tx,
	}
	t.inputs = tx.Data
	return t
}

func (t *txBuilder) WrapCall(tx *pldapi.TransactionCall) TxBuilder {
	txCopy := *tx
	t.tx = &txCopy
	t.inputs = tx.Data
	return t
}

func (t *txBuilder) BuildTX() SendableTransaction {
	st := &sendableTransaction{
		chainable: t.chainable,
		outputs:   t.outputs,
	}
	var err error
	st.tx, err = t.copyTX()
	// Check it's valid before we attempt to send (won't override any earlier error)
	st.deferError(err)
	return st
}

func (t *txBuilder) copyTX() (*pldapi.TransactionCall, error) {
	tx := *t.tx
	err := t.validateForSend()
	if err == nil {
		if tx.ABI != nil {
			// Finalize the data of the transaction in the chained sendable TX
			tx.Data, err = t.BuildInputDataJSON()
			return &tx, err
		}
		// We will just send thd data blindly as JSON
		switch tv := t.inputs.(type) {
		case nil:
			tx.Data = nil
		case []byte:
			tx.Data = tv
		case string:
			tx.Data = pldtypes.RawJSON(tv)
		default:
			tx.Data, err = json.Marshal(tv)
		}
	}
	return &tx, err
}

func (t *txBuilder) Send() SentTransaction {
	return t.BuildTX().Send()
}

func (t *txBuilder) Prepare() PreparingTransaction {
	return t.BuildTX().Prepare()
}

func (t *txBuilder) Call() error {
	return t.BuildTX().Call()
}

func (t *txBuilder) BuildCallData() (callData pldtypes.HexBytes, err error) {
	var inputDataRLP []byte
	def, cv, err := t.BuildInputDataCV()
	if err == nil {
		inputDataRLP, err = cv.EncodeABIDataCtx(t.ctx)
	}
	if err == nil {
		if t.tx.Function != "" {
			// function call
			var selector []byte
			selector, err = def.GenerateFunctionSelectorCtx(t.ctx)
			if err == nil {
				callData = make([]byte, len(selector)+len(inputDataRLP))
				copy(callData, selector)
				copy(callData[len(selector):], inputDataRLP)
			}
		} else {
			// constructor
			callData = make([]byte, len(t.tx.Bytecode)+len(inputDataRLP))
			copy(callData, t.tx.Bytecode)
			copy(callData[len(t.tx.Bytecode):], inputDataRLP)
		}
	}
	return callData, err
}

func (t *txBuilder) ResolveDefinition() (*abi.Entry, error) {
	if t.tx.ABI == nil {
		return nil, i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientNoABISupplied)
	}
	if t.tx.Function == "" {
		def := t.tx.ABI.Constructor()
		if def == nil {
			def = defaultConstructor
		}
		return def, nil
	}
	def := t.functions[t.tx.Function]
	if def == nil {
		return nil, i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientFunctionNotFound, t.tx.Function)
	}
	return def, nil
}

func (t *txBuilder) BuildInputDataCV() (def *abi.Entry, cv *abi.ComponentValue, err error) {
	var typeTree abi.TypeComponent
	def, err = t.ResolveDefinition()
	if err == nil {
		typeTree, err = def.Inputs.TypeComponentTreeCtx(t.ctx)
	}
	if err != nil {
		return nil, nil, err
	}

	var inputJSONable any
	if t.inputs == nil {
		if len(typeTree.TupleChildren()) > 0 {
			return nil, nil, i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientMissingInput, def.SolString())
		}
		inputJSONable = []any{}
	} else {
		switch input := t.inputs.(type) {
		case string:
			err = json.Unmarshal([]byte(input), &inputJSONable)
		case []byte:
			err = json.Unmarshal(input, &inputJSONable)
		case pldtypes.RawJSON:
			err = json.Unmarshal(input, &inputJSONable)
		case *abi.ComponentValue:
			cv = input
		default:
			var jsonInput []byte
			jsonInput, err = json.Marshal(t.inputs)
			if err == nil {
				err = json.Unmarshal(jsonInput, &inputJSONable)
			}
		}
	}
	if err == nil && cv == nil /* might have got a CV directly */ {
		cv, err = typeTree.ParseExternalCtx(t.ctx, inputJSONable)
	}
	if err != nil {
		return nil, nil, i18n.WrapError(t.ctx, err, pldmsgs.MsgPaladinClientInvalidInput, def.SolString())
	}
	return def, cv, err
}

func (t *txBuilder) BuildInputDataJSON() (jsonData pldtypes.RawJSON, err error) {
	var serializer *abi.Serializer
	_, cv, err := t.BuildInputDataCV()
	if err == nil {
		serializer, err = t.tx.DataFormat.GetABISerializer(t.ctx)
	}
	if err != nil {
		return nil, err
	}
	return serializer.SerializeJSONCtx(t.ctx, cv)
}

func (st sendableTransaction) Send() SentTransaction {
	sent := &sentTransaction{
		chainable: st.chainable,
	}
	if st.tx.From == "" {
		sent.deferError(i18n.NewError(st.ctx, pldmsgs.MsgPaladinClientMissingFrom))
	}
	if sent.deferredErr != nil {
		return sent
	}
	var err error
	var existingTX *pldapi.Transaction
	sent.txID, err = st.c.PTX().SendTransaction(st.ctx, &st.tx.TransactionInput)
	if err != nil && st.tx.IdempotencyKey != "" && strings.Contains(err.Error(), "PD012220") {
		log.L(st.ctx).Infof("Idempotency key clash for %s - checking for existing transaction: %s", st.tx.IdempotencyKey, err)
		existingTX, err = st.c.PTX().GetTransactionByIdempotencyKey(st.ctx, st.tx.IdempotencyKey)
		if err == nil && existingTX != nil {
			err = nil
			sent.txID = existingTX.ID
		}
	}
	sent.deferError(err)
	return sent
}

func (st sendableTransaction) Prepare() PreparingTransaction {
	preparing := &preparingTransaction{
		chainable: st.chainable,
	}
	if st.tx.From == "" {
		preparing.deferError(i18n.NewError(st.ctx, pldmsgs.MsgPaladinClientMissingFrom))
	}
	if preparing.deferredErr != nil {
		return preparing
	}
	var err error
	var existingTX *pldapi.Transaction
	preparing.txID, err = st.c.PTX().PrepareTransaction(st.ctx, &st.tx.TransactionInput)
	if err != nil && st.tx.IdempotencyKey != "" && strings.Contains(err.Error(), "PD012220") {
		log.L(st.ctx).Infof("Idempotency key clash for %s - checking for existing transaction: %s", st.tx.IdempotencyKey, err)
		existingTX, err = st.c.PTX().GetTransactionByIdempotencyKey(st.ctx, st.tx.IdempotencyKey)
		if err == nil && existingTX != nil {
			err = nil
			preparing.txID = existingTX.ID
		}
	}
	preparing.deferError(err)
	return preparing
}

func (st sendableTransaction) Call() error {
	if st.deferredErr != nil {
		return st.deferredErr
	}
	data, err := st.c.PTX().Call(st.ctx, st.tx)
	if err == nil {
		err = json.Unmarshal(data, st.outputs)
	}
	return err
}

func (st sendableTransaction) TX() *pldapi.TransactionInput {
	return &st.tx.TransactionInput
}

func (st sendableTransaction) CallTX() *pldapi.TransactionCall {
	return st.tx
}

func (sent *sentTransaction) GetReceipt() (*pldapi.TransactionReceipt, error) {
	if sent.deferredErr != nil {
		return nil, sent.deferredErr
	}
	return sent.c.PTX().GetTransactionReceipt(sent.ctx, *sent.txID)
}

func (sent *sentTransaction) GetTransaction() (*pldapi.Transaction, error) {
	if sent.deferredErr != nil {
		return nil, sent.deferredErr
	}
	return sent.c.PTX().GetTransaction(sent.ctx, *sent.txID)
}

func (sent *sentTransaction) ID() *uuid.UUID {
	return sent.txID
}

func (sent *sentTransaction) Wait(timeout time.Duration) TransactionResult {
	tr := &transactionResult{
		chainable: sent.chainable,
	}
	if tr.deferredErr != nil {
		return tr
	}
	tr.txID = *sent.txID
	return tr.wait(timeout)
}

func (preparing *preparingTransaction) GetPreparedTransaction() (*pldapi.PreparedTransaction, error) {
	if preparing.deferredErr != nil {
		return nil, preparing.deferredErr
	}
	return preparing.c.PTX().GetPreparedTransaction(preparing.ctx, *preparing.txID)
}

func (preparing *preparingTransaction) GetTransaction() (*pldapi.Transaction, error) {
	if preparing.deferredErr != nil {
		return nil, preparing.deferredErr
	}
	return preparing.c.PTX().GetTransaction(preparing.ctx, *preparing.txID)
}

func (preparing *preparingTransaction) ID() *uuid.UUID {
	return preparing.txID
}

func (preparing *preparingTransaction) Wait(timeout time.Duration) PreparedTransactionResult {
	tr := &preparedTransactionResult{
		chainable: preparing.chainable,
	}
	if tr.deferredErr != nil {
		return tr
	}
	tr.txID = *preparing.txID
	return tr.wait(timeout)
}

func txPoller[R any](ch *chainable, timeout time.Duration, txID uuid.UUID, doGet func() (*R, error)) *R {
	pollingInterval := ch.c.receiptPollingInterval
	if pollingInterval > timeout {
		pollingInterval = timeout
	}

	// With HTTP we poll
	startTime := time.Now()
	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()
	attempt := 0
	var lastErr error
	var result *R
	for {
		attempt++
		result, lastErr = doGet()
		if lastErr != nil {
			log.L(ch.ctx).Warnf("attempt %d to poll transaction %s failed: %s", attempt, txID, lastErr)
		}
		// Did we get one?
		if result != nil {
			return result
		}
		// Check we didn't timeout
		waitTime := time.Since(startTime)
		if waitTime > timeout {
			ch.deferError(i18n.WrapError(ch.ctx, lastErr, pldmsgs.MsgPaladinClientPollTxTimedOut, attempt, waitTime, txID))
			return result
		}
		// Wait before polling
		select {
		case <-ticker.C:
		case <-ch.ctx.Done():
			ch.deferError(i18n.WrapError(ch.ctx, lastErr, pldmsgs.MsgContextCanceled))
			return result
		}
	}
}

func (tr *transactionResult) wait(timeout time.Duration) TransactionResult {
	// TODO: Websocket optimization
	tr.receipt = txPoller(&tr.chainable, timeout, tr.txID, func() (*pldapi.TransactionReceipt, error) {
		return tr.c.PTX().GetTransactionReceipt(tr.ctx, tr.txID)
	})
	if tr.receipt != nil && !tr.receipt.Success {
		tr.deferError(errors.New(tr.receipt.FailureMessage))
	}
	return tr
}

func (tr *transactionResult) ID() uuid.UUID {
	return tr.txID
}

func (tr *transactionResult) Receipt() *pldapi.TransactionReceipt {
	return tr.receipt
}

func (tr *transactionResult) TransactionHash() *pldtypes.Bytes32 {
	if tr.receipt != nil && tr.receipt.TransactionReceiptDataOnchain != nil {
		return tr.receipt.TransactionHash
	}
	return nil
}

func (ptr *preparedTransactionResult) wait(timeout time.Duration) PreparedTransactionResult {
	ptr.prepared = txPoller(&ptr.chainable, timeout, ptr.txID, func() (*pldapi.PreparedTransaction, error) {
		return ptr.c.PTX().GetPreparedTransaction(ptr.ctx, ptr.txID)
	})
	return ptr
}

func (ptr *preparedTransactionResult) ID() uuid.UUID {
	return ptr.txID
}

func (ptr *preparedTransactionResult) PreparedTransaction() *pldapi.PreparedTransaction {
	return ptr.prepared
}

func (t *txBuilder) validateForSend() error {
	if t.tx.Type == "" {
		return i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientMissingType)
	}
	if t.tx.Domain == "" && t.tx.Type.V() == pldapi.TransactionTypePrivate {
		return i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientNoDomain)
	}
	if t.tx.Function == "" {
		if t.tx.To != nil {
			return i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientNoFunction)
		}
		if t.tx.Type.V() == pldapi.TransactionTypePrivate && t.tx.Bytecode != nil {
			return i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientBytecodeWithPriv)
		} else if t.tx.Type.V() == pldapi.TransactionTypePublic && len(t.tx.Bytecode) == 0 {
			return i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientBytecodeMissing)
		}
	} else {
		if t.tx.To == nil {
			return i18n.NewError(t.ctx, pldmsgs.MsgPaladinClientMissingTo, t.tx.Function)
		}
	}
	return nil
}
