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

package txmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This contains the fields that go into the database.
// We keep this separate from the pldapi.TransactionXYZ interfaces that clients and applications use to interact
// with this, so we have a separation of concerns on the GORM annotations and data serialization format
type persistedTransaction struct {
	ID                 uuid.UUID                            `gorm:"column:id;primaryKey"`
	IdempotencyKey     *string                              `gorm:"column:idempotency_key"`
	SubmitMode         tktypes.Enum[pldapi.SubmitMode]      `gorm:"column:submit_mode"`
	Type               tktypes.Enum[pldapi.TransactionType] `gorm:"column:type"`
	Created            tktypes.Timestamp                    `gorm:"column:created;autoCreateTime:nano"`
	ABIReference       *tktypes.Bytes32                     `gorm:"column:abi_ref"`
	Function           *string                              `gorm:"column:function"`
	Domain             *string                              `gorm:"column:domain"`
	From               string                               `gorm:"column:from"`
	To                 *tktypes.EthAddress                  `gorm:"column:to"`
	Data               tktypes.RawJSON                      `gorm:"column:data"` // we always store in JSON object format
	TransactionDeps    []*transactionDep                    `gorm:"foreignKey:transaction;references:id"`
	TransactionReceipt *transactionReceipt                  `gorm:"foreignKey:transaction;references:id"`
}

type transactionDep struct {
	Transaction uuid.UUID `gorm:"column:transaction;primaryKey"`
	DependsOn   uuid.UUID `gorm:"column:depends_on"`
}

func (persistedTransaction) TableName() string {
	return "transactions"
}

var defaultConstructor = &abi.Entry{Type: abi.Constructor, Inputs: abi.ParameterArray{}}
var defaultConstructorSignature = func() string {
	sig, _ := defaultConstructor.Signature()
	return sig
}()

func (tm *txManager) resolveFunction(ctx context.Context, dbTX *gorm.DB, inputABI abi.ABI, inputABIRef *tktypes.Bytes32, requiredFunction string, to *tktypes.EthAddress) (_ *components.ResolvedFunction, err error) {

	// Lookup the ABI we're working with.
	// Only needs to contain the function definition we're calling, but can be the whole ABI of the contract.
	// Beneficial if it includes the error definitions for this
	var pa *pldapi.StoredABI
	if inputABIRef != nil {
		if inputABI != nil {
			return nil, i18n.NewError(ctx, msgs.MsgTxMgrABIAndDefinition)
		}
		pa, err = tm.getABIByHash(ctx, dbTX, *inputABIRef)
	} else {
		if len(inputABI) == 0 {
			if to != nil {
				return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrNoABIOrReference)
			}
			// it's convenient to do a deploy without a constructor, of bytecode with no
			// parameters - treat this as an ABI with just the default constructor
			// (we need something to hash to an abiReference in all cases)
			inputABI = abi.ABI{defaultConstructor}
		}
		pa, err = tm.UpsertABI(ctx, dbTX, inputABI)
	}
	if err != nil || pa == nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrABIReferenceLookupFailed, inputABIRef)
	}

	// If a function is specified, we cannot be invoking the constructor
	if requiredFunction != "" && to == nil {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrFunctionWithoutTo)
	}

	// Find the function in the ABI that we're invoking
	var selectedFunction *abi.Entry
	var functionSignature string
	for _, e := range pa.ABI {
		var isMatch bool
		if e.Type == abi.Constructor && to == nil {
			isMatch = true
		} else if e.Type == abi.Function && to != nil {
			if strings.HasPrefix(requiredFunction, "0x") {
				selectorString := e.FunctionSelectorBytes().String()
				isMatch = strings.EqualFold(selectorString, requiredFunction)
			} else if strings.Contains(requiredFunction, "(") {
				selectorString, _ := e.Signature()
				isMatch = (selectorString == requiredFunction)
			} else if len(requiredFunction) > 0 {
				isMatch = (e.Name == requiredFunction)
			} else {
				// No selector - any function is a match
				isMatch = true
			}
		}
		if isMatch {
			oldSelector := functionSignature
			functionSignature, _ = e.Signature()
			if oldSelector != "" {
				return nil, i18n.NewError(ctx, msgs.MsgTxMgrFunctionMultiMatch, oldSelector, functionSignature)
			}
			selectedFunction = e
		}
	}
	if functionSignature == "" || selectedFunction == nil {
		if to == nil {
			// This is the common case when the ABI was non-empty, but there's no constructor in there.
			selectedFunction = defaultConstructor
			functionSignature = defaultConstructorSignature
		} else {
			return nil, i18n.NewError(ctx, msgs.MsgTxMgrFunctionNoMatch)
		}
	}
	log.L(ctx).Debugf("Function selected: %s", selectedFunction.SolString())
	return &components.ResolvedFunction{
		ABI:          pa.ABI,
		ABIReference: &pa.Hash,
		Definition:   selectedFunction,
		Signature:    functionSignature,
	}, nil
}

func (tm *txManager) parseDataBytes(ctx context.Context, e *abi.Entry, dataBytes []byte) (cv *abi.ComponentValue, err error) {
	// We might have the function selector
	selector := e.FunctionSelectorBytes()
	if len(dataBytes) >= len(selector) && len(dataBytes)%32 == 4 && bytes.Equal(selector, dataBytes[0:4]) {
		cv, err = e.Inputs.DecodeABIDataCtx(ctx, selector, 4) // we will run out of data if this is not right, so safe to do first
	}
	if cv == nil || err != nil {
		cv, err = e.Inputs.DecodeABIDataCtx(ctx, dataBytes, 0)
	}
	return cv, err
}

func (tm *txManager) parseInputs(
	ctx context.Context,
	e *abi.Entry,
	txType tktypes.Enum[pldapi.TransactionType],
	data tktypes.RawJSON,
	bytecode tktypes.HexBytes,
) (cv *abi.ComponentValue, jsonData tktypes.RawJSON, err error) {

	if (e.Type != abi.Constructor || txType.V() != pldapi.TransactionTypePublic) && len(bytecode) != 0 {
		return nil, nil, i18n.NewError(ctx, msgs.MsgTxMgrBytecodeNonPublicConstructor, txType.V(), e.String())
	} else if e.Type == abi.Constructor && len(bytecode) == 0 && txType == pldapi.TransactionTypePublic.Enum() {
		// We don't support supplying bytecode for public transactions precompiled ahead of the constructor
		// inputs, you must split the contract code out into bytecode
		return nil, nil, i18n.NewError(ctx, msgs.MsgTxMgrBytecodeAndHexData, e.String())
	}

	// TODO: Resolve domain for private TX

	var iDecoded any
	if data != nil {
		d := json.NewDecoder(bytes.NewReader(data.Bytes()))
		d.UseNumber()
		if err := d.Decode(&iDecoded); err != nil {
			return nil, nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrInvalidInputData, e.String())
		}
	}
	switch decoded := iDecoded.(type) {
	case nil:
		cv, err = tm.parseDataBytes(ctx, e, []byte{})
	case string:
		// Must be a byte array pre-encoded
		var dataBytes []byte
		dataBytes, err = tktypes.ParseHexBytes(ctx, decoded)
		if err == nil {
			cv, err = tm.parseDataBytes(ctx, e, dataBytes)
		}
	case map[string]interface{}, []interface{}:
		cv, err = e.Inputs.ParseExternalDataCtx(ctx, decoded)
	default:
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrInvalidInputDataType, iDecoded)
	}
	if err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrInvalidInputData, e.String())
	}
	jsonData, err = tktypes.StandardABISerializer().SerializeJSONCtx(ctx, cv)
	return
}

func (tm *txManager) SendTransaction(ctx context.Context, tx *pldapi.TransactionInput) (*uuid.UUID, error) {
	// TODO: Add flush writer for parallel performance here, that calls sendTransactions
	// in the flush writer on the batch (rather than doing a DB commit per TX)
	txIDs, err := tm.SendTransactions(ctx, []*pldapi.TransactionInput{tx})
	if err != nil {
		return nil, err
	}
	return &txIDs[0], nil
}

func (tm *txManager) PrepareTransaction(ctx context.Context, tx *pldapi.TransactionInput) (*uuid.UUID, error) {
	txIDs, err := tm.PrepareTransactions(ctx, []*pldapi.TransactionInput{tx})
	if err != nil {
		return nil, err
	}
	return &txIDs[0], nil
}

func (tm *txManager) CallTransaction(ctx context.Context, result any, call *pldapi.TransactionCall) (err error) {

	txi, err := tm.resolveNewTransaction(ctx, tm.p.DB(), &call.TransactionInput, pldapi.SubmitModeCall)
	if err != nil {
		return err
	}

	serializer, err := call.DataFormat.GetABISerializer(ctx)
	if err != nil {
		return err
	}

	if call.Type.V() == pldapi.TransactionTypePublic {
		return tm.callTransactionPublic(ctx, result, call, txi, serializer)
	}

	if call.To == nil {
		// We don't support a "call" of a deploy for private Transactions
		return i18n.NewError(ctx, msgs.MsgTxMgrPrivateCallRequiresTo)
	}

	// Do the call
	cv, err := tm.privateTxMgr.CallPrivateSmartContract(ctx, &components.TransactionInputs{
		Domain:   call.Domain,
		From:     call.From,
		To:       *call.To,
		Function: txi.Function.Definition,
		Inputs:   txi.Inputs,
		Intent:   prototk.TransactionSpecification_CALL,
	})
	if err != nil {
		return err
	}

	// Serialize the result
	b, err := serializer.SerializeJSONCtx(ctx, cv)
	if err == nil {
		err = json.Unmarshal(b, result)
	}
	return err
}

func (tm *txManager) callTransactionPublic(ctx context.Context, result any, call *pldapi.TransactionCall, txi *components.ValidatedTransaction, serializer *abi.Serializer) (err error) {

	ec := tm.ethClientFactory.HTTPClient().(ethclient.EthClientWithKeyManager)
	var callReq ethclient.ABIFunctionRequestBuilder
	abiFunc, err := ec.ABIFunction(ctx, txi.Function.Definition)
	blockRef := call.Block.String()
	if blockRef == "" {
		blockRef = "latest"
	}
	if err == nil {
		callReq = abiFunc.R(ctx).
			To(call.To.Address0xHex()).
			Input(call.Data).
			BlockRef(ethclient.BlockRef(blockRef)).
			Serializer(serializer).
			Output(result)
		if call.From != "" {
			var senderAddr *tktypes.EthAddress
			senderAddr, err = tm.keyManager.ResolveEthAddressNewDatabaseTX(ctx, txi.LocalFrom)
			if err == nil {
				callReq = callReq.Signer(senderAddr.String())
			}
		}
	}
	if err == nil {
		err = callReq.Call()
	}
	return err
}

func (tm *txManager) PrepareInternalPrivateTransaction(ctx context.Context, dbTX *gorm.DB, tx *pldapi.TransactionInput, submitMode pldapi.SubmitMode) (*components.ValidatedTransaction, error) {
	tx.Type = pldapi.TransactionTypePrivate.Enum()
	if tx.IdempotencyKey == "" {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrPrivateChainedTXIdemKey)
	}
	return tm.resolveNewTransaction(ctx, dbTX, tx, submitMode)
}

func (tm *txManager) UpsertInternalPrivateTxsFinalizeIDs(ctx context.Context, dbTX *gorm.DB, txis []*components.ValidatedTransaction) error {
	// On this path we handle the idempotency key matching - noting that we validate the existence of an idempotency key in PrepareInternalPrivateTransaction
	insertCount, err := tm.insertTransactions(ctx, dbTX, txis, true /* on conflict do nothing */)
	if err != nil {
		return err
	}

	// if the insert count is not the same as the transaction found we have to reconcile the IDs
	if int(insertCount) != len(txis) {
		idempotencyKeys := make([]string, len(txis))
		for i, tx := range txis {
			idempotencyKeys[i] = tx.Transaction.IdempotencyKey
		}
		log.L(ctx).Warnf("insert count mismatch - checking for idempotency key clashes: %v", idempotencyKeys)
		var txsInDB []*persistedTransaction
		err := dbTX.
			WithContext(ctx).
			Select("id", "idempotency_key").
			Where("idempotency_key in (?)", idempotencyKeys).
			Find(&txsInDB).
			Error
		if err != nil {
			return err
		}
		matchCount := 0
		for _, tx := range txis {
			for _, txInDB := range txsInDB {
				if txInDB.IdempotencyKey != nil && tx.Transaction.IdempotencyKey == *txInDB.IdempotencyKey {
					txID := txInDB.ID
					tx.Transaction.ID = &txID
					log.L(ctx).Infof("matched insert idempotencyKey=%s txID=%s", tx.Transaction.IdempotencyKey, txID)
					matchCount++
				}
			}
		}
		if matchCount != len(txis) {
			return i18n.NewError(ctx, msgs.MsgTxMgrPrivateInsertErrorMismatch, len(txsInDB), matchCount, len(txis))
		}
	}

	// Note deliberately no notification to private TX manager here, as this function is for it to call us.
	// So when it's flushed its internal transaction, it notifies itself.

	return nil
}

func (tm *txManager) SendTransactions(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error) {
	return tm.processNewTransactions(ctx, txs, pldapi.SubmitModeAuto)
}

func (tm *txManager) PrepareTransactions(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error) {
	return tm.processNewTransactions(ctx, txs, pldapi.SubmitModeExternal)
}

func (tm *txManager) processNewTransactions(ctx context.Context, txs []*pldapi.TransactionInput, submitMode pldapi.SubmitMode) (txIDs []uuid.UUID, err error) {

	// Public transactions need a signing address resolution and nonce allocation trackers
	// before we open the database transaction
	var publicTxs []*components.PublicTxSubmission
	var publicTxSenders []string
	txis := make([]*components.ValidatedTransaction, len(txs))
	txIDs = make([]uuid.UUID, len(txs))
	for i, tx := range txs {
		txi, err := tm.resolveNewTransaction(ctx, tm.p.DB() /* no db tx for this part currently */, tx, submitMode)
		if err != nil {
			return nil, err
		}
		txID := *txi.Transaction.ID
		txis[i] = txi
		txIDs[i] = txID
		if tx.Type.V() == pldapi.TransactionTypePublic {
			publicTxs = append(publicTxs, &components.PublicTxSubmission{
				// Public transaction bound 1:1 with our parent transaction
				Bindings: []*components.PaladinTXReference{{TransactionID: txID, TransactionType: pldapi.TransactionTypePublic.Enum()}},
				PublicTxInput: pldapi.PublicTxInput{
					To:              tx.To,
					Data:            txi.PublicTxData,
					PublicTxOptions: tx.PublicTxOptions,
				},
			})
			publicTxSenders = append(publicTxSenders, txi.LocalFrom)
		}
	}

	// Need to resolve the addresses for any public senders
	if len(publicTxs) > 0 {
		ethAddresses, err := tm.keyManager.ResolveEthAddressBatchNewDatabaseTX(ctx, publicTxSenders)
		if err != nil {
			return nil, err
		}
		for i, ptx := range publicTxs {
			ptx.From = ethAddresses[i]
		}
	}

	// Perform pre-transaction processing in the public TX manager as required
	var committed = false
	var publicBatch components.PublicTxBatch
	if len(publicTxs) > 0 {
		publicBatch, err = tm.publicTxMgr.PrepareSubmissionBatch(ctx, publicTxs)
		if err != nil {
			return nil, err
		}
		// Must ensure we close the batch, now it's open (for good or bad)
		defer func() {
			publicBatch.Completed(ctx, committed)
		}()
		// TODO: don't support partial rejection currently - will be important when we introduce the flush writer
		if len(publicBatch.Rejected()) > 0 {
			return nil, publicBatch.Rejected()[0].RejectedError()
		}
	}

	// Do in-transaction processing for our tables, and the public tables
	insertedOK := false
	err = tm.p.DB().Transaction(func(dbTX *gorm.DB) (err error) {
		_, err = tm.insertTransactions(ctx, dbTX, txis, false /* all must succeed on this path - we map idempotency errors below */)
		insertedOK = (err == nil)
		if err == nil && publicBatch != nil {
			err = publicBatch.Submit(ctx, dbTX)
		}
		// TODO: private insertion too
		return err
	})
	if err != nil {
		return nil, tm.checkIdempotencyKeys(ctx, err, insertedOK, txs)
	}
	// From this point on we're committed, and need to tell the public tx manager as such
	committed = true

	// TODO: Integrate with private TX manager persistence when available, as it will follow the
	// same pattern as public transactions above
	for _, txi := range txis {
		if txi.Transaction.Type.V() == pldapi.TransactionTypePrivate {
			if err := tm.privateTxMgr.HandleNewTx(ctx, txi); err != nil {
				return nil, err
			}
		}
	}
	return txIDs, err
}

// Will either return the original error, or will return a special idempotency key error that can be used by the caller
// to determine that they need to ask for the existing transactions (rather than fail)
func (tm *txManager) checkIdempotencyKeys(ctx context.Context, origErr error, insertedOK bool, txis []*pldapi.TransactionInput) error {
	idempotencyKeys := make([]any, 0, len(txis))
	for _, tx := range txis {
		if tx.IdempotencyKey != "" {
			idempotencyKeys = append(idempotencyKeys, tx.IdempotencyKey)
		}
	}
	if !insertedOK && len(idempotencyKeys) > 0 {
		existingTxs, lookupErr := tm.QueryTransactions(ctx, query.NewQueryBuilder().In("idempotencyKey", idempotencyKeys).Limit(len(idempotencyKeys)).Query(), false)
		if lookupErr != nil {
			log.L(ctx).Errorf("Failed to query for existing idempotencyKeys after insert error (returning original error): %s", lookupErr)
		} else if (len(existingTxs)) > 0 {
			msgInfo := make([]string, len(existingTxs))
			for i, tx := range existingTxs {
				msgInfo[i] = fmt.Sprintf("%s=%s", tx.IdempotencyKey, tx.ID)
			}
			log.L(ctx).Errorf("Overriding insertion error with idempotencyKey error. origErr: %s", origErr)
			return i18n.NewError(ctx, msgs.MsgTxMgrIdempotencyKeyClash, strings.Join(msgInfo, ","))
		}
	}
	return origErr
}

func (tm *txManager) resolveNewTransaction(ctx context.Context, dbTX *gorm.DB, tx *pldapi.TransactionInput, submitMode pldapi.SubmitMode) (*components.ValidatedTransaction, error) {
	txID := uuid.New()

	switch tx.Type.V() {
	case pldapi.TransactionTypePrivate:
	case pldapi.TransactionTypePublic:
		if submitMode == pldapi.SubmitModeExternal {
			return nil, i18n.NewError(ctx, msgs.MsgTxMgrPrivateOnlyForPrepare)
		}
	default:
		// Note autofuel transactions can only be created internally within the public TX manager
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrInvalidTXType)
	}

	// We resolve the function outside of a DB transaction, because it's idempotent processing
	// and needs to happen before we open the DB transaction that is used by the public TX manager.
	// Note there is only a DB cost for read if we haven't cached the function, and there
	// is only a DB cost for write, if it's the first time we've invoked the function.
	fn, err := tm.resolveFunction(ctx, dbTX, tx.ABI, tx.ABIReference, tx.Function, tx.To)
	if err != nil {
		return nil, err
	}

	var publicTxData []byte
	cv, normalizedJSON, err := tm.parseInputs(ctx, fn.Definition, tx.Type, tx.Data, tx.Bytecode)
	if err == nil && tx.Type.V() == pldapi.TransactionTypePublic {
		publicTxData, err = tm.getPublicTxData(ctx, fn.Definition, tx.Bytecode, cv)
	}
	if err != nil {
		return nil, err
	}

	var localFrom string
	bypassFromCheck := submitMode == pldapi.SubmitModePrepare || /* no checking on from for prepare */
		(submitMode == pldapi.SubmitModeCall && tx.From == "") /* call is allowed no sender */
	if !bypassFromCheck {

		identifier, node, err := tktypes.PrivateIdentityLocator(tx.From).Validate(ctx, tm.localNodeName, false)
		if err != nil || node != tm.localNodeName {
			return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrPublicSenderNotValidLocal, tx.From)
		}
		localFrom = identifier
		tx.From = fmt.Sprintf("%s@%s", identifier, node)
	}

	return &components.ValidatedTransaction{
		LocalFrom: localFrom,
		Transaction: &pldapi.Transaction{
			TransactionBase: tx.TransactionBase,
			ID:              &txID,
			SubmitMode:      submitMode.Enum(),
		},
		DependsOn:    tx.DependsOn,
		Function:     fn,
		PublicTxData: publicTxData,
		Inputs:       normalizedJSON,
	}, nil
}

func (tm *txManager) getPublicTxData(ctx context.Context, fnDef *abi.Entry, bytecode []byte, cv *abi.ComponentValue) ([]byte, error) {
	switch fnDef.Type {
	case abi.Function:
		return fnDef.EncodeCallDataCtx(ctx, cv)
	case abi.Constructor:
		// Encode the parameters after the bytecode
		var paramBytes []byte
		buff := bytes.NewBuffer(make([]byte, 0, len(bytecode)))
		_, err := buff.Write(bytecode)
		if err == nil {
			paramBytes, err = cv.EncodeABIDataCtx(ctx)
		}
		if err == nil {
			_, err = buff.Write(paramBytes)
		}
		if err != nil {
			return nil, err
		}
		return buff.Bytes(), nil
	default:
		// This is unexpected - earlier processing should have prevented this
		return nil, i18n.NewError(ctx, msgs.MsgInvalidTransactionType)
	}
}

func (tm *txManager) insertTransactions(ctx context.Context, dbTX *gorm.DB, txis []*components.ValidatedTransaction, ignoreConflicts bool) (int64, error) {
	ptxs := make([]*persistedTransaction, len(txis))
	var transactionDeps []*transactionDep
	for i, txi := range txis {
		tx := txi.Transaction

		ptxs[i] = &persistedTransaction{
			ID:             *tx.ID,
			SubmitMode:     tx.SubmitMode,
			IdempotencyKey: notEmptyOrNull(tx.IdempotencyKey),
			Type:           tx.Type,
			ABIReference:   txi.Function.ABIReference,
			Function:       notEmptyOrNull(txi.Function.Signature),
			Domain:         notEmptyOrNull(tx.Domain),
			From:           tx.From,
			To:             tx.To,
			Data:           txi.Inputs,
		}
		for _, d := range txi.DependsOn {
			transactionDeps = append(transactionDeps, &transactionDep{
				Transaction: *tx.ID,
				DependsOn:   d,
			})
		}
	}

	insert := dbTX.
		WithContext(ctx).
		Table("transactions").
		Omit("TransactionDeps")
	if ignoreConflicts {
		insert = insert.Clauses(clause.OnConflict{DoNothing: true})
	}
	res := insert.Create(ptxs)
	err := res.Error
	if err == nil && len(transactionDeps) > 0 {
		err = dbTX.
			Table("transaction_deps").
			Create(transactionDeps).
			Error
	}
	if err != nil {
		return -1, err
	}
	return res.RowsAffected, nil
}
