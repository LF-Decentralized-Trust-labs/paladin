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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"gorm.io/gorm/clause"
)

// This contains the fields that go into the database.
// We keep this separate from the pldapi.TransactionXYZ interfaces that clients and applications use to interact
// with this, so we have a separation of concerns on the GORM annotations and data serialization format
type persistedTransaction struct {
	ID                 uuid.UUID                             `gorm:"column:id;primaryKey"`
	IdempotencyKey     *string                               `gorm:"column:idempotency_key"`
	SubmitMode         pldtypes.Enum[pldapi.SubmitMode]      `gorm:"column:submit_mode"`
	Type               pldtypes.Enum[pldapi.TransactionType] `gorm:"column:type"`
	Created            pldtypes.Timestamp                    `gorm:"column:created;autoCreateTime:false"` // set by code before insert
	ABIReference       *pldtypes.Bytes32                     `gorm:"column:abi_ref"`
	Function           *string                               `gorm:"column:function"`
	Domain             *string                               `gorm:"column:domain"`
	From               string                                `gorm:"column:from"`
	To                 *pldtypes.EthAddress                  `gorm:"column:to"`
	Data               pldtypes.RawJSON                      `gorm:"column:data"` // we always store in JSON object format
	TransactionDeps    []*transactionDep                     `gorm:"foreignKey:transaction;references:id"`
	TransactionReceipt *transactionReceipt                   `gorm:"foreignKey:transaction;references:id"`
}

type transactionDep struct {
	Transaction uuid.UUID `gorm:"column:transaction;primaryKey"`
	DependsOn   uuid.UUID `gorm:"column:depends_on"`
}

func (persistedTransaction) TableName() string {
	return "transactions"
}

type persistedTransactionHistory struct {
	ID                   uuid.UUID                             `gorm:"column:id;primaryKey"`
	TXID                 uuid.UUID                             `gorm:"column:tx_id"`
	IdempotencyKey       *string                               `gorm:"column:idempotency_key"`
	Type                 pldtypes.Enum[pldapi.TransactionType] `gorm:"column:type"`
	Created              pldtypes.Timestamp                    `gorm:"column:created;autoCreateTime:false"` // set by code before insert
	ABIReference         *pldtypes.Bytes32                     `gorm:"column:abi_ref"`
	Function             *string                               `gorm:"column:function"`
	Domain               *string                               `gorm:"column:domain"`
	From                 string                                `gorm:"column:from"`
	To                   *pldtypes.EthAddress                  `gorm:"column:to"`
	Data                 pldtypes.RawJSON                      `gorm:"column:data"` // we always store in JSON object format
	Gas                  *pldtypes.HexUint64                   `gorm:"column:gas"`
	Value                *pldtypes.HexUint256                  `gorm:"column:value"`
	GasPrice             *pldtypes.HexUint256                  `gorm:"column:gas_price"`
	MaxFeePerGas         *pldtypes.HexUint256                  `gorm:"column:max_fee_per_gas"`
	MaxPriorityFeePerGas *pldtypes.HexUint256                  `gorm:"column:max_priority_fee_per_gas"`
}

func (persistedTransactionHistory) TableName() string {
	return "transaction_history"
}

type persistedChainedPrivateTxn struct {
	ChainedTransaction uuid.UUID `gorm:"column:chained_transaction;primaryKey"`
	Transaction        uuid.UUID `gorm:"column:transaction;primaryKey"`
	Sender             string    `gorm:"column:sender;primaryKey"`
	Domain             string    `gorm:"column:domain;primaryKey"`
}

func (persistedChainedPrivateTxn) TableName() string {
	return "chained_private_txns"
}

var defaultConstructor = &abi.Entry{Type: abi.Constructor, Inputs: abi.ParameterArray{}}
var defaultConstructorSignature = func() string {
	sig, _ := defaultConstructor.Signature()
	return sig
}()

func (tm *txManager) resolveFunction(ctx context.Context, dbTX persistence.DBTX, inputABI abi.ABI, inputABIRef *pldtypes.Bytes32, requiredFunction string, to *pldtypes.EthAddress) (_ *components.ResolvedFunction, err error) {

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
		// We support a NOTX transaction in this function, particularly for Call when the ABI is already written/cached.
		// However, in the case we're about to write the ABI we need a TX for post commit handling - so take the hit here of a mini-TX
		if !dbTX.FullTransaction() {
			err = tm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
				pa, err = tm.UpsertABI(ctx, dbTX, inputABI)
				return err
			})
		} else {
			pa, err = tm.UpsertABI(ctx, dbTX, inputABI)
		}
	}
	if err != nil || pa == nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrABIReferenceLookupFailed, inputABIRef)
	}

	resolvedFunction, err := tm.pickFunction(ctx, pa, requiredFunction, to)
	if err != nil {
		return nil, err
	}

	log.L(ctx).Debugf("Function selected: %s", resolvedFunction.Definition.SolString())
	return resolvedFunction, nil
}

func (tm *txManager) pickFunction(ctx context.Context, pa *pldapi.StoredABI, requiredFunction string, to *pldtypes.EthAddress) (_ *components.ResolvedFunction, err error) {

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
	return &components.ResolvedFunction{
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
	txType pldtypes.Enum[pldapi.TransactionType],
	data pldtypes.RawJSON,
	bytecode pldtypes.HexBytes,
) (cv *abi.ComponentValue, jsonData pldtypes.RawJSON, err error) {

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
		dataBytes, err = pldtypes.ParseHexBytes(ctx, decoded)
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
	jsonData, err = pldtypes.StandardABISerializer().SerializeJSONCtx(ctx, cv)
	return
}

func (tm *txManager) sendTransactionNewDBTX(ctx context.Context, tx *pldapi.TransactionInput) (*uuid.UUID, error) {
	// TODO: Add flush writer for parallel performance here, that calls sendTransactions
	// in the flush writer on the batch (rather than doing a DB commit per TX)
	txIDs, err := tm.sendTransactionsNewDBTX(ctx, []*pldapi.TransactionInput{tx})
	if err != nil {
		return nil, err
	}
	return &txIDs[0], nil
}

func (tm *txManager) prepareTransactionNewDBTX(ctx context.Context, tx *pldapi.TransactionInput) (*uuid.UUID, error) {
	txIDs, err := tm.prepareTransactionsNewDBTX(ctx, []*pldapi.TransactionInput{tx})
	if err != nil {
		return nil, err
	}
	return &txIDs[0], nil
}

func (tm *txManager) CallTransaction(ctx context.Context, dbTX persistence.DBTX, result any, call *pldapi.TransactionCall) (err error) {

	txi, err := tm.resolveNewTransaction(ctx, dbTX, &call.TransactionInput, pldapi.SubmitModeCall)
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
	cv, err := tm.privateTxMgr.CallPrivateSmartContract(ctx, &txi.ResolvedTransaction)
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
			var senderAddr *pldtypes.EthAddress
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

func (tm *txManager) PrepareChainedPrivateTransaction(ctx context.Context, dbTX persistence.DBTX, origSender string, origTxID uuid.UUID, origDomain string, tx *pldapi.TransactionInput, submitMode pldapi.SubmitMode) (chained *components.ChainedPrivateTransaction, err error) {
	tx.Type = pldapi.TransactionTypePrivate.Enum()
	if tx.IdempotencyKey == "" {
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrPrivateChainedTXIdemKey)
	}
	newTX, err := tm.resolveNewTransaction(ctx, dbTX, tx, submitMode)
	if err == nil {
		chained = &components.ChainedPrivateTransaction{
			OriginalSenderLocator: origSender,
			OriginalTransaction:   origTxID,
			OriginalDomain:        origDomain,
			NewTransaction:        newTX,
		}
	}
	return chained, err
}

func (tm *txManager) ChainPrivateTransactions(ctx context.Context, dbTX persistence.DBTX, chainedTxns []*components.ChainedPrivateTransaction) error {

	txis := make([]*components.ValidatedTransaction, len(chainedTxns))
	chainingRecords := make([]*persistedChainedPrivateTxn, len(chainedTxns))
	for i, chainedTxn := range chainedTxns {
		txis[i] = chainedTxn.NewTransaction
		chainingRecords[i] = &persistedChainedPrivateTxn{
			Sender:             chainedTxn.OriginalSenderLocator,
			Transaction:        chainedTxn.OriginalTransaction,
			Domain:             chainedTxn.OriginalDomain,
			ChainedTransaction: *chainedTxn.NewTransaction.Transaction.ID,
		}
	}

	// On this path we handle the idempotency key matching - noting that we validate the existence of an idempotency key in PrepareChainedPrivateTransaction
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
		err := dbTX.DB().
			WithContext(ctx).
			Select("id", "created", "idempotency_key").
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
					tx.Transaction.Created = txInDB.Created
					log.L(ctx).Infof("matched insert idempotencyKey=%s txID=%s", tx.Transaction.IdempotencyKey, txID)
					matchCount++
				}
			}
		}
		if matchCount != len(txis) {
			return i18n.NewError(ctx, msgs.MsgTxMgrPrivateInsertErrorMismatch, len(txsInDB), matchCount, len(txis))
		}
	}

	// Insert the chaining records which allow us to correlate the completion of the chained transaction, back
	// to the completion of the original transaction in the case of a failure in particular.
	return tm.writeChainingRecords(ctx, dbTX, chainingRecords)

	// Note deliberately no notification to private TX manager here, as this function is for it to call us.
	// So when it's flushed its internal transaction, it notifies itself.
}

func (tm *txManager) writeChainingRecords(ctx context.Context, dbTX persistence.DBTX, chainingRecords []*persistedChainedPrivateTxn) error {
	return dbTX.DB().
		Clauses(clause.OnConflict{DoNothing: true}).
		WithContext(ctx).
		Create(chainingRecords).Error
}

func (tm *txManager) SendTransactions(ctx context.Context, dbTX persistence.DBTX, txs ...*pldapi.TransactionInput) (txIDs []uuid.UUID, err error) {
	return tm.processNewTransactions(ctx, dbTX, txs, pldapi.SubmitModeAuto)
}

func (tm *txManager) sendTransactionsNewDBTX(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error) {
	err = tm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		txIDs, err = tm.SendTransactions(ctx, dbTX, txs...)
		return err
	})
	return txIDs, err
}

func (tm *txManager) prepareTransactionsNewDBTX(ctx context.Context, txs []*pldapi.TransactionInput) (txIDs []uuid.UUID, err error) {
	err = tm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		txIDs, err = tm.PrepareTransactions(ctx, dbTX, txs...)
		return err
	})
	return txIDs, err

}

func (tm *txManager) PrepareTransactions(ctx context.Context, dbTX persistence.DBTX, txs ...*pldapi.TransactionInput) (txIDs []uuid.UUID, err error) {
	return tm.processNewTransactions(ctx, dbTX, txs, pldapi.SubmitModeExternal)
}

func (tm *txManager) processNewTransactions(ctx context.Context, dbTX persistence.DBTX, txs []*pldapi.TransactionInput, submitMode pldapi.SubmitMode) (txIDs []uuid.UUID, err error) {

	// Public transactions need a signing address resolution and nonce allocation trackers
	// before we open the database transaction
	var publicTxs []*components.PublicTxSubmission
	var publicTxSenders []string
	txis := make([]*components.ValidatedTransaction, len(txs))
	txIDs = make([]uuid.UUID, len(txs))

	for i, tx := range txs {
		txi, err := tm.resolveNewTransaction(ctx, dbTX, tx, submitMode)
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

	// Public transactions need key resolution and validation
	if len(publicTxs) > 0 {
		kr := tm.keyManager.KeyResolverForDBTX(dbTX)
		for i, ptx := range publicTxs {
			resolvedKey, err := kr.ResolveKey(ctx, publicTxSenders[i], algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			if err == nil {
				ptx.From, err = pldtypes.ParseEthAddress(resolvedKey.Verifier.Verifier)
			}
			if err == nil {
				err = tm.publicTxMgr.ValidateTransaction(ctx, dbTX, ptx)
			}
			if err != nil {
				return nil, err
			}
		}
	}

	// Now we're ready to insert into the database
	_, err = tm.insertTransactions(ctx, dbTX, txis, false /* all must succeed on this path - we map idempotency errors below */)
	if err != nil {
		dbTX.AddPostRollback(func(txCtx context.Context, err error) error {
			// OUTSIDE of the rolled back transaction
			return tm.checkIdempotencyKeys(ctx, err, txs)
		})
		return nil, err
	}

	// Insert any public txns (validated above)
	if len(publicTxs) > 0 {
		if _, err = tm.publicTxMgr.WriteNewTransactions(ctx, dbTX, publicTxs); err != nil {
			return nil, err
		}
	}

	// TODO: Integrate with private TX manager persistence when available, as it will follow the
	// same pattern as public transactions above
	for _, txi := range txis {
		if txi.Transaction.Type.V() == pldapi.TransactionTypePrivate {
			if err := tm.privateTxMgr.HandleNewTx(ctx, dbTX, txi); err != nil {
				return nil, err
			}
		}
	}
	return txIDs, err
}

// Will either return the original error, or will return a special idempotency key error that can be used by the caller
// to determine that they need to ask for the existing transactions (rather than fail)
func (tm *txManager) checkIdempotencyKeys(ctx context.Context, origErr error, txis []*pldapi.TransactionInput) error {
	idempotencyKeys := make([]any, 0, len(txis))
	for _, tx := range txis {
		if tx.IdempotencyKey != "" {
			idempotencyKeys = append(idempotencyKeys, tx.IdempotencyKey)
		}
	}
	if len(idempotencyKeys) > 0 {
		existingTxs, lookupErr := tm.QueryTransactions(ctx, query.NewQueryBuilder().In("idempotencyKey", idempotencyKeys).Limit(len(idempotencyKeys)).Query(),
			tm.p.NOTX(), /* intentionally outside of any transaction that might just rolling back in caller */
			false)
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

func (tm *txManager) resolvePrivateDomain(ctx context.Context, dbTX persistence.DBTX, tx *pldapi.TransactionInput) error {
	if tx.To != nil {
		// We've been given the contract to invoke, we need to check it's valid
		psc, err := tm.domainMgr.GetSmartContractByAddress(ctx, dbTX, *tx.To)
		if err != nil {
			return err
		}
		domain := psc.Domain().Name()
		if tx.Domain == "" {
			tx.Domain = domain
		} else if tx.Domain != domain {
			return i18n.NewError(ctx, msgs.MsgTxMgrDomainMismatch, tx.Domain, domain, psc.Address())
		}
	} else if tx.Domain == "" {
		// We deploying a private smart contract, so we must have a domain
		return i18n.NewError(ctx, msgs.MsgTxMgrDomainMissingForDeploy)
	}
	return nil
}

func (tm *txManager) resolveNewTransaction(ctx context.Context, dbTX persistence.DBTX, tx *pldapi.TransactionInput, submitMode pldapi.SubmitMode) (*components.ValidatedTransaction, error) {
	txID := uuid.New()
	// Useful to have a correlation from transactionID to idempotencyKey in the logs
	log.L(ctx).Debugf("Resolving new transaction TransactionID: %s, idempotencyKey: %s ", txID, tx.IdempotencyKey)

	switch tx.Type.V() {
	case pldapi.TransactionTypePrivate:
		if err := tm.resolvePrivateDomain(ctx, dbTX, tx); err != nil {
			return nil, err
		}
	case pldapi.TransactionTypePublic:
		if submitMode == pldapi.SubmitModeExternal {
			return nil, i18n.NewError(ctx, msgs.MsgTxMgrPrivateOnlyForPrepare)
		}
	default:
		return nil, i18n.NewError(ctx, msgs.MsgTxMgrInvalidTXType)
	}

	var publicTxData []byte
	fn, cv, normalizedJSON, err := tm.ResolveTransactionInputs(ctx, dbTX, tx)
	if err == nil && tx.Type.V() == pldapi.TransactionTypePublic {
		publicTxData, err = tm.getPublicTxData(ctx, fn.Definition, tx.Bytecode, cv)
	}
	if err != nil {
		return nil, err
	}
	// Update to normalized JSON in what we store
	tx.Data = normalizedJSON

	var localFrom string
	bypassFromCheck := submitMode == pldapi.SubmitModePrepare || /* no checking on from for prepare */
		(submitMode == pldapi.SubmitModeCall && tx.From == "") /* call is allowed no sender */
	if !bypassFromCheck {
		if strings.HasPrefix(tx.From, "eth_address:") {
			addr := strings.TrimPrefix(tx.From, "eth_address:")
			if _, err := pldtypes.ParseEthAddress(addr); err != nil {
				return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrVerifierNotEthAddress, addr)
			}
			// Doing the reverse lookup here means that we can persist the identifier on the transaction. It does mean that the
			// identifier will later be resolved back to the verifier but this should just be a cache read
			mapping, err := tm.keyManager.ReverseKeyLookup(ctx, dbTX, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, addr)
			if err != nil {
				return nil, err
			}
			tx.From = mapping.Identifier
		}

		identifier, node, err := pldtypes.PrivateIdentityLocator(tx.From).Validate(ctx, tm.localNodeName, false)
		if err != nil || node != tm.localNodeName {
			return nil, i18n.WrapError(ctx, err, msgs.MsgTxMgrPublicSenderNotValidLocal, tx.From)
		}
		localFrom = identifier
		tx.From = fmt.Sprintf("%s@%s", identifier, node)
	}

	return &components.ValidatedTransaction{
		LocalFrom: localFrom,
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				TransactionBase: tx.TransactionBase,
				ID:              &txID,
				SubmitMode:      submitMode.Enum(),
			},
			DependsOn: tx.DependsOn,
			Function:  fn,
		},
		PublicTxData: publicTxData,
	}, nil
}

func (tm *txManager) ResolveTransactionInputs(ctx context.Context, dbTX persistence.DBTX, tx *pldapi.TransactionInput) (*components.ResolvedFunction, *abi.ComponentValue, pldtypes.RawJSON, error) {
	fn, err := tm.resolveFunction(ctx, dbTX, tx.ABI, tx.ABIReference, tx.Function, tx.To)
	if err != nil {
		return nil, nil, nil, err
	}

	cv, normalizedJSON, err := tm.parseInputs(ctx, fn.Definition, tx.Type, tx.Data, tx.Bytecode)
	if err != nil {
		return nil, nil, nil, err
	}

	return fn, cv, normalizedJSON, nil
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

func (tm *txManager) insertTransactions(ctx context.Context, dbTX persistence.DBTX, txis []*components.ValidatedTransaction, ignoreConflicts bool) (int64, error) {
	ptxs := make([]*persistedTransaction, len(txis))
	txhs := make([]*persistedTransactionHistory, len(txis))
	var transactionDeps []*transactionDep
	for i, txi := range txis {
		// Resolve the finalized fields on the input object for return
		tx := txi.Transaction
		tx.Created = pldtypes.TimestampNow()
		tx.ABIReference = txi.Function.ABIReference
		tx.Function = txi.Function.Signature
		// Build the object to insert
		ptxs[i] = &persistedTransaction{
			ID:             *tx.ID,
			SubmitMode:     tx.SubmitMode,
			Created:        tx.Created,
			IdempotencyKey: notEmptyOrNull(tx.IdempotencyKey),
			Type:           tx.Type,
			ABIReference:   tx.ABIReference,
			Function:       notEmptyOrNull(txi.Function.Signature),
			Domain:         notEmptyOrNull(tx.Domain),
			From:           tx.From,
			To:             tx.To,
			Data:           tx.Data,
		}
		for _, d := range txi.DependsOn {
			transactionDeps = append(transactionDeps, &transactionDep{
				Transaction: *tx.ID,
				DependsOn:   d,
			})
		}
		txhs[i] = &persistedTransactionHistory{
			ID:                   uuid.New(),
			TXID:                 ptxs[i].ID,
			IdempotencyKey:       ptxs[i].IdempotencyKey,
			Type:                 ptxs[i].Type,
			Created:              ptxs[i].Created,
			ABIReference:         ptxs[i].ABIReference,
			Function:             ptxs[i].Function,
			Domain:               ptxs[i].Domain,
			From:                 ptxs[i].From,
			To:                   ptxs[i].To,
			Data:                 ptxs[i].Data,
			Gas:                  tx.Gas,
			Value:                tx.Value,
			GasPrice:             tx.GasPrice,
			MaxFeePerGas:         tx.MaxFeePerGas,
			MaxPriorityFeePerGas: tx.MaxPriorityFeePerGas,
		}
	}

	insert := dbTX.DB().
		WithContext(ctx).
		Table("transactions").
		Omit("TransactionDeps")
	if ignoreConflicts {
		insert = insert.Clauses(clause.OnConflict{DoNothing: true})
	}
	txInsertResult := insert.Create(ptxs)
	err := txInsertResult.Error
	if err == nil {
		err = dbTX.DB().
			Table("transaction_history").
			Create(txhs).
			Error
	}
	if err == nil && len(transactionDeps) > 0 {
		err = dbTX.DB().
			Table("transaction_deps").
			Clauses(clause.OnConflict{DoNothing: true}). // for idempotency retry
			Create(transactionDeps).
			Error
	}
	if err != nil {
		return -1, err
	}
	rowsAffected := txInsertResult.RowsAffected

	dbTX.AddPostCommit(func(ctx context.Context) {
		// Only update the cache if there were no conflicts
		if rowsAffected == int64(len(txis)) {
			for _, tx := range txis {
				tm.txCache.Set(*tx.Transaction.ID, &components.ResolvedTransaction{
					Transaction: tx.Transaction,
					DependsOn:   tx.DependsOn,
					Function:    tx.Function,
				})
			}
		}
	})
	return rowsAffected, nil
}

func (tm *txManager) UpdateTransaction(ctx context.Context, id uuid.UUID, tx *pldapi.TransactionInput) (uuid.UUID, error) {
	oldTX, err := tm.GetTransactionByID(ctx, id)
	if err != nil {
		return id, err
	}

	if oldTX == nil {
		return id, i18n.NewError(ctx, msgs.MsgTxMgrTransactionNotFound, id)
	}

	if oldTX.Type.V() != pldapi.TransactionTypePublic {
		return id, i18n.NewError(ctx, msgs.MsgTxMgrUpdateInvalidType)
	}

	var pubTXID uint64
	var publicTxData []byte
	var validatedTransaction *components.ValidatedTransaction
	var from *pldtypes.EthAddress

	err = tm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		pubTXs, err := tm.publicTxMgr.QueryPublicTxForTransactions(ctx, dbTX, []uuid.UUID{id}, nil)
		if err != nil {
			return err
		}
		// if this is a public transaction there should be exactly one entry in the map and exactly one entry
		// in the array but it's still best to avoid any risk of a nil pointer exception
		if _, ok := pubTXs[id]; !ok || len(pubTXs[id]) == 0 {
			return i18n.NewError(ctx, msgs.MsgPublicTransactionNotFound, id)
		}
		pubTXID = *pubTXs[id][0].LocalID

		validatedTransaction, err = tm.resolveUpdatedTransaction(ctx, dbTX, id, tx, oldTX)
		if err != nil {
			return err
		}

		if validatedTransaction != nil {
			publicTxData = validatedTransaction.PublicTxData
		}

		from, err = pldtypes.ParseEthAddress(oldTX.From)
		if err != nil {
			identifier := strings.Split(oldTX.From, "@")[0]
			kr := tm.keyManager.KeyResolverForDBTX(dbTX)
			var resolvedKey *pldapi.KeyMappingAndVerifier
			resolvedKey, err = kr.ResolveKey(ctx, identifier, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			if err == nil {
				// this failure should be impossible if key manager is working correctly
				from, err = pldtypes.ParseEthAddress(resolvedKey.Verifier.Verifier)
			}
		}
		return err
	})

	if err != nil {
		return id, err
	}

	err = tm.publicTxMgr.UpdateTransaction(ctx, id, pubTXID, from, tx, publicTxData, func(dbTX persistence.DBTX) error {
		return tm.processUpdatedTransaction(ctx, dbTX, oldTX.ID, validatedTransaction)
	})

	return id, err
}

func (tm *txManager) processUpdatedTransaction(ctx context.Context, dbTX persistence.DBTX, id *uuid.UUID, validatedTransaction *components.ValidatedTransaction) error {
	// only update the fields which might have changed with this request
	err := dbTX.DB().
		WithContext(ctx).
		Table("transactions").
		Where("id = ?", id).
		Updates(&persistedTransaction{
			ABIReference: validatedTransaction.Function.ABIReference,
			Function:     notEmptyOrNull(validatedTransaction.Function.Signature),
			To:           validatedTransaction.Transaction.To,
			Data:         validatedTransaction.Transaction.Data,
		}).
		Error

	if err == nil {
		tx := validatedTransaction.Transaction
		txh := &persistedTransactionHistory{
			ID:                   uuid.New(),
			TXID:                 *tx.ID,
			IdempotencyKey:       notEmptyOrNull(tx.IdempotencyKey),
			Type:                 tx.Type,
			Created:              pldtypes.TimestampNow(),
			ABIReference:         validatedTransaction.Function.ABIReference,
			Function:             notEmptyOrNull(validatedTransaction.Function.Signature),
			Domain:               notEmptyOrNull(tx.Domain),
			From:                 tx.From,
			To:                   tx.To,
			Data:                 tx.Data,
			Gas:                  tx.Gas,
			Value:                tx.Value,
			GasPrice:             tx.GasPrice,
			MaxFeePerGas:         tx.MaxFeePerGas,
			MaxPriorityFeePerGas: tx.MaxPriorityFeePerGas,
		}
		err = dbTX.DB().
			Table("transaction_history").
			Create(txh).
			Error
	}
	return err
}

func (tm *txManager) resolveUpdatedTransaction(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID, txi *pldapi.TransactionInput, oldTX *pldapi.Transaction) (*components.ValidatedTransaction, error) {
	fn, err := tm.resolveFunction(ctx, dbTX, txi.ABI, txi.ABIReference, txi.Function, txi.To)
	if err != nil {
		return nil, err
	}

	var publicTxData []byte
	cv, normalizedJSON, err := tm.parseInputs(ctx, fn.Definition, pldapi.TransactionTypePublic.Enum(), txi.Data, txi.Bytecode)
	if err == nil {
		publicTxData, err = tm.getPublicTxData(ctx, fn.Definition, nil, cv)
	}
	if err != nil {
		return nil, err
	}

	validatedTransaction := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				TransactionBase: txi.TransactionBase,
				ID:              &id,
			},
			Function: fn,
		},
		PublicTxData: publicTxData,
	}

	tx := validatedTransaction.Transaction
	// Update to normalized JSON in what we store
	tx.Data = normalizedJSON

	// copy across immutable fields from old transaction
	tx.Type = oldTX.Type
	tx.From = oldTX.From
	tx.IdempotencyKey = oldTX.IdempotencyKey

	return validatedTransaction, nil
}
