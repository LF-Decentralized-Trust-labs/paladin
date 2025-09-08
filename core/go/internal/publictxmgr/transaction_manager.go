/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicaptm law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package publictxmgr

import (
	"context"
	"encoding/json"
	"math/big"
	"sync"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/publictxmgr/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// configurations
// metrics

// UpdateType informs policy loop whether the transaction needs an update to be persisted after a transaction processor finished processing a transaction
type UpdateType int

const (
	UpdateNo     UpdateType = iota // Instructs that no update is necessary
	UpdateYes                      // Instructs that the transaction should be updated in persistence
	UpdateDelete                   // Instructs that the transaction should be removed completely from persistence - generally only returned when TX status is TxStatusDeleteRequested
)

type transactionUpdate struct {
	newPtx  *DBPublicTxn
	pubTXID uint64
	from    *pldtypes.EthAddress
}

// Public Tx Engine:
// - It offers two ways of calculating gas price: use a fixed number, use the built-in API of a ethereum connector
// - It resubmits the transaction based on a configured interval until it succeed or fail
// - It also recalculate gas price during resubmissions
// - It logs errors transactions breach certain configured thresholds of staleness
// - It offers caches of gas price for transactions targeting same method of a smart contract
// - It provide a outbound request concurrency control

type pubTxManager struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	conf             *pldconf.PublicTxManagerConfig
	thMetrics        metrics.PublicTransactionManagerMetrics
	p                persistence.Persistence
	bIndexer         blockindexer.BlockIndexer
	ethClient        ethclient.EthClient
	keymgr           components.KeyManager
	rootTxMgr        components.TXManager
	ethClientFactory ethclient.EthClientFactory
	// gas price
	gasPriceClient   GasPriceClient
	submissionWriter *submissionWriter

	// a map of signing addresses and transaction engines
	inFlightOrchestrators       map[pldtypes.EthAddress]*orchestrator
	signingAddressesPausedUntil map[pldtypes.EthAddress]time.Time
	inFlightOrchestratorMux     sync.Mutex
	inFlightOrchestratorStale   chan bool

	// inbound concurrency control TBD

	// engine config
	maxInflight              int
	orchestratorIdleTimeout  time.Duration
	orchestratorStaleTimeout time.Duration
	orchestratorSwapTimeout  time.Duration
	retry                    *retry.Retry
	enginePollingInterval    time.Duration
	nonceCacheTimeout        time.Duration
	engineLoopDone           chan struct{}

	activityRecordCache     cache.Cache[uint64, *txActivityRecords]
	maxActivityRecordsPerTx int

	// balance manager
	balanceManager BalanceManager

	// orchestrator config
	gasPriceIncreaseMax     *big.Int
	gasPriceIncreasePercent int

	// gas limit config
	gasEstimateFactor float64

	// updates
	updates   []*transactionUpdate
	updateMux sync.Mutex
}

type txActivityRecords struct {
	lock    sync.Mutex
	records []pldapi.TransactionActivityRecord
}

func NewPublicTransactionManager(ctx context.Context, conf *pldconf.PublicTxManagerConfig) components.PublicTxManager {
	log.L(ctx).Debugf("Creating new public transaction manager")

	gasPriceClient := NewGasPriceClient(ctx, conf)
	gasPriceIncreaseMax := confutil.BigIntOrNil(conf.GasPrice.IncreaseMax)
	gasEstimateFactor := confutil.Float64Min(conf.GasLimit.GasEstimateFactor, 1.0, *pldconf.PublicTxManagerDefaults.GasLimit.GasEstimateFactor)

	log.L(ctx).Debugf("Enterprise transaction handler created")

	ptmCtx, ptmCtxCancel := context.WithCancel(log.WithLogField(ctx, "role", "public_tx_mgr"))

	return &pubTxManager{
		ctx:                         ptmCtx,
		ctxCancel:                   ptmCtxCancel,
		conf:                        conf,
		gasPriceClient:              gasPriceClient,
		inFlightOrchestratorStale:   make(chan bool, 1),
		signingAddressesPausedUntil: make(map[pldtypes.EthAddress]time.Time),
		maxInflight:                 confutil.IntMin(conf.Manager.MaxInFlightOrchestrators, 1, *pldconf.PublicTxManagerDefaults.Manager.MaxInFlightOrchestrators),
		orchestratorSwapTimeout:     confutil.DurationMin(conf.Manager.OrchestratorSwapTimeout, 0, *pldconf.PublicTxManagerDefaults.Manager.OrchestratorSwapTimeout),
		orchestratorStaleTimeout:    confutil.DurationMin(conf.Manager.OrchestratorStaleTimeout, 0, *pldconf.PublicTxManagerDefaults.Manager.OrchestratorStaleTimeout),
		orchestratorIdleTimeout:     confutil.DurationMin(conf.Manager.OrchestratorIdleTimeout, 0, *pldconf.PublicTxManagerDefaults.Manager.OrchestratorIdleTimeout),
		enginePollingInterval:       confutil.DurationMin(conf.Manager.Interval, 50*time.Millisecond, *pldconf.PublicTxManagerDefaults.Manager.Interval),
		nonceCacheTimeout:           confutil.DurationMin(conf.Manager.NonceCacheTimeout, 0, *pldconf.PublicTxManagerDefaults.Manager.NonceCacheTimeout),
		retry:                       retry.NewRetryIndefinite(&conf.Manager.Retry),
		gasPriceIncreaseMax:         gasPriceIncreaseMax,
		gasPriceIncreasePercent:     confutil.Int(conf.GasPrice.IncreasePercentage, *pldconf.PublicTxManagerDefaults.GasPrice.IncreasePercentage),
		activityRecordCache:         cache.NewCache[uint64, *txActivityRecords](&conf.Manager.ActivityRecords.CacheConfig, &pldconf.PublicTxManagerDefaults.Manager.ActivityRecords.CacheConfig),
		maxActivityRecordsPerTx:     confutil.Int(conf.Manager.ActivityRecords.RecordsPerTransaction, *pldconf.PublicTxManagerDefaults.Manager.ActivityRecords.RecordsPerTransaction),
		gasEstimateFactor:           gasEstimateFactor,
	}
}

func (ptm *pubTxManager) PreInit(pic components.PreInitComponents) (result *components.ManagerInitResult, err error) {
	ptm.thMetrics = metrics.InitMetrics(ptm.ctx, pic.MetricsManager().Registry())
	return &components.ManagerInitResult{}, nil
}

// Post-init allows the manager to cross-bind to other components, or the Engine
func (ptm *pubTxManager) PostInit(pic components.AllComponents) error {
	ctx := ptm.ctx
	log.L(ctx).Debugf("Initializing public transaction manager")
	ptm.ethClientFactory = pic.EthClientFactory()
	ptm.keymgr = pic.KeyManager()
	ptm.p = pic.Persistence()
	ptm.bIndexer = pic.BlockIndexer()
	ptm.rootTxMgr = pic.TxManager()
	ptm.submissionWriter = newSubmissionWriter(ptm.ctx, ptm.p, ptm.conf, ptm.thMetrics)
	ptm.balanceManager = NewBalanceManagerWithInMemoryTracking(ctx, ptm.conf, ptm)

	log.L(ctx).Debugf("Initialized public transaction manager")
	return nil
}

func (ptm *pubTxManager) Start() error {
	ctx := ptm.ctx
	log.L(ctx).Debugf("Starting public transaction manager")

	// The client is assured to be started by this point and availaptm
	ptm.ethClient = ptm.ethClientFactory.SharedWS()
	ptm.gasPriceClient.Init(ctx, ptm.ethClient)
	if ptm.engineLoopDone == nil { // only start once
		ptm.engineLoopDone = make(chan struct{})
		log.L(ctx).Debugf("Kicking off  enterprise handler engine loop")
		go ptm.engineLoop()
	}
	ptm.MarkInFlightOrchestratorsStale()
	ptm.submissionWriter.Start()
	log.L(ctx).Infof("Started public transaction manager")
	return nil
}

func (ptm *pubTxManager) Stop() {
	ptm.ctxCancel()
	if ptm.submissionWriter != nil {
		ptm.submissionWriter.Shutdown()
	}
	if ptm.engineLoopDone != nil {
		<-ptm.engineLoopDone
	}
}

func buildEthTX(
	from pldtypes.EthAddress,
	nonce *uint64,
	to *pldtypes.EthAddress,
	data pldtypes.HexBytes,
	options *pldapi.PublicTxOptions,
) *ethsigner.Transaction {
	ethTx := &ethsigner.Transaction{
		From:                 json.RawMessage(pldtypes.JSONString(from)),
		To:                   to.Address0xHex(),
		GasPrice:             (*ethtypes.HexInteger)(options.GasPrice),
		MaxPriorityFeePerGas: (*ethtypes.HexInteger)(options.MaxPriorityFeePerGas),
		MaxFeePerGas:         (*ethtypes.HexInteger)(options.MaxFeePerGas),
		Value:                (*ethtypes.HexInteger)(options.Value),
		Data:                 ethtypes.HexBytes0xPrefix(data),
	}
	if nonce != nil {
		ethTx.Nonce = ethtypes.NewHexIntegerU64(*nonce)
	}
	if options.Gas != nil {
		ethTx.GasLimit = ethtypes.NewHexIntegerU64(options.Gas.Uint64())
	}
	return ethTx
}

func (ptm *pubTxManager) SingleTransactionSubmit(ctx context.Context, txi *components.PublicTxSubmission) (tx *pldapi.PublicTx, err error) {
	var txs []*pldapi.PublicTx
	err = ptm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		err := ptm.ValidateTransaction(ctx, dbTX, txi)
		if err == nil {
			txs, err = ptm.WriteNewTransactions(ctx, dbTX, []*components.PublicTxSubmission{txi})
		}
		return err
	})
	if err == nil {
		tx = txs[0]
	}
	return tx, err
}

func (ptm *pubTxManager) ValidateTransaction(ctx context.Context, dbTX persistence.DBTX, txi *components.PublicTxSubmission) error {
	log.L(ctx).Tracef("PrepareSubmission transaction: %+v", txi)

	if txi.From == nil {
		return i18n.NewError(ctx, msgs.MsgInvalidTXMissingFromAddr)
	}

	prepareStart := time.Now()
	var txType InFlightTxOperation

	if txi.Gas == nil || *txi.Gas == 0 {
		gasEstimateResult, err := ptm.ethClient.EstimateGasNoResolve(ctx, buildEthTX(
			*txi.From,
			nil, /* nonce not assigned at this point */
			txi.To,
			txi.Data,
			&txi.PublicTxOptions,
		))
		if err != nil {
			log.L(ctx).Errorf("HandleNewTx <%s> error estimating gas for transaction: %+v, request: (%+v)", txType, err, txi)
			ptm.thMetrics.RecordOperationMetrics(ctx, string(txType), string(GenericStatusFail), time.Since(prepareStart).Seconds())
			if ethclient.MapSubmissionRejected(err) {
				// transaction is rejected. We can build a useful error message hopefully by processing the rejection info
				if len(gasEstimateResult.RevertData) > 0 {
					// we can use the error dictionary callback to TXManager to look up the ABI
					// Note: The ABI is already persisted before TXManager calls down into us.
					err = ptm.rootTxMgr.CalculateRevertError(ctx, dbTX, gasEstimateResult.RevertData)
					log.L(ctx).Warnf("Estimate gas reverted (%s): %s", err, err)
				}
				return err
			}
			return err
		}
		factoredGasLimit := pldtypes.HexUint64((float64)(gasEstimateResult.GasLimit) * ptm.gasEstimateFactor)
		txi.Gas = &factoredGasLimit
		log.L(ctx).Tracef("HandleNewTx <%s> using the estimated gas limit %s multiplied by the gas estimate factor %.f (=%s) for transaction: %+v", txType, gasEstimateResult.GasLimit, ptm.gasEstimateFactor, factoredGasLimit, txi)
	} else {
		log.L(ctx).Tracef("HandleNewTx <%s> using the provided gas limit %s for transaction: %+v", txType, txi.Gas, txi)
	}

	ptm.thMetrics.RecordOperationMetrics(ctx, string(txType), string(GenericStatusSuccess), time.Since(prepareStart).Seconds())
	log.L(ctx).Debugf("HandleNewTx <%s> transaction validated and nonce assignment intent created for %s", txType, txi.From)
	return nil
}

func (ptm *pubTxManager) WriteNewTransactions(ctx context.Context, dbTX persistence.DBTX, transactions []*components.PublicTxSubmission) (pubTxns []*pldapi.PublicTx, err error) {
	persistedTransactions := make([]*DBPublicTxn, len(transactions))
	for i, txi := range transactions {
		persistedTransactions[i] = &DBPublicTxn{
			From:            *txi.From, // safe because validated in ValidateTransaction
			To:              txi.To,
			Gas:             txi.Gas.Uint64(),
			Value:           txi.Value,
			Data:            txi.Data,
			FixedGasPricing: pldtypes.JSONString(txi.PublicTxGasPricing),
		}
	}
	// All the nonce processing to this point should have ensured we do not have a conflict on nonces.
	// It is the caller's responsibility to ensure we do not have a conflict on transaction+resubmit_idx.
	if len(persistedTransactions) > 0 {
		err = dbTX.DB().
			WithContext(ctx).
			Table("public_txns").
			Clauses(clause.Returning{Columns: []clause.Column{{Name: "pub_txn_id"}}}).
			Create(persistedTransactions).
			Error
	}
	if err == nil {
		publicTxBindings := make([]*DBPublicTxnBinding, 0, len(transactions))
		for i, txi := range transactions {
			pubTxnID := persistedTransactions[i].PublicTxnID
			for _, bnd := range txi.Bindings {
				publicTxBindings = append(publicTxBindings, &DBPublicTxnBinding{
					Transaction:     bnd.TransactionID,
					TransactionType: bnd.TransactionType,
					Sender:          bnd.TransactionSender,
					ContractAddress: bnd.TransactionContractAddress,
					PublicTxnID:     pubTxnID,
				})
			}
		}
		if len(publicTxBindings) > 0 {
			err = dbTX.DB().
				WithContext(ctx).
				Table("public_txn_bindings").
				Create(publicTxBindings).
				Error
		}
	}
	if err == nil {
		pubTxns = make([]*pldapi.PublicTx, len(persistedTransactions))
		toNotify := make(map[pldtypes.EthAddress]bool)
		for i, ptx := range persistedTransactions {
			pubTxns[i] = mapPersistedTransaction(ptx)
			toNotify[ptx.From] = true
		}
		dbTX.AddPostCommit(ptm.postCommitNewTransactions(toNotify))
	}

	return pubTxns, err
}

func (ptm *pubTxManager) writeUpdatedTransaction(ctx context.Context, dbTX persistence.DBTX, pubTXID uint64, from pldtypes.EthAddress, newPtx *DBPublicTxn) error {
	err := dbTX.DB().
		WithContext(ctx).
		Table("public_txns").
		Where("pub_txn_id = ?", pubTXID).
		Updates(newPtx).
		Error

	if err == nil {
		toNotify := map[pldtypes.EthAddress]bool{
			from: true,
		}
		dbTX.AddPostCommit(ptm.postCommitNewTransactions(toNotify))
	}
	return err
}

func (ptm *pubTxManager) postCommitNewTransactions(toNotify map[pldtypes.EthAddress]bool) func(ctx context.Context) {
	return func(ctx context.Context) {
		// Mark any active orchestrators stale
		inactive := false
		for addr := range toNotify {
			oc := ptm.getOrchestratorForAddress(addr)
			if oc != nil {
				log.L(ctx).Debugf("Notified orchestrator %s to re-poll due to new transactions", &addr)
				oc.MarkInFlightTxStale()
			} else {
				inactive = true
			}
		}
		// And if there was an orchestrator un-loaded, then mark the main poll loop stale
		// TODO: this doesn't guarantee it will be loaded if there are no free orchestrator spaces
		if inactive {
			ptm.MarkInFlightOrchestratorsStale()
		}
	}
}

func recoverGasPriceOptions(gpoJSON pldtypes.RawJSON) (ptgp pldapi.PublicTxGasPricing) {
	if gpoJSON != nil {
		_ = json.Unmarshal(gpoJSON, &ptgp)
	}
	return
}

// Component interface: query public transactions, outside of the scope of a binding to a parent Paladin transaction.
// Returns each public transaction a maximum of once
func (ptm *pubTxManager) QueryPublicTxWithBindings(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PublicTxWithBinding, error) {
	return ptm.queryPublicTxWithBinding(ctx, dbTX, nil, jq)
}

// Component interface: query the associated public transactions, for a set of parent Paladin transactions
// Can return the same public transaction multiple times, if bound to multiple private transactions.
// The results are grouped, so the caller can be assured to have exactly one entry in the map (even if an empty array) per supplied TX ID
func (ptm *pubTxManager) QueryPublicTxForTransactions(ctx context.Context, dbTX persistence.DBTX, boundToTxns []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error) {
	if boundToTxns == nil {
		boundToTxns = []uuid.UUID{}
	}
	boundPublicTxns, err := ptm.queryPublicTxWithBinding(ctx, dbTX, boundToTxns, jq)
	if err != nil {
		return nil, err
	}
	results := make(map[uuid.UUID][]*pldapi.PublicTx)
	for _, id := range boundToTxns {
		results[id] = []*pldapi.PublicTx{}
		for _, pubTX := range boundPublicTxns {
			if pubTX.Transaction == id {
				results[id] = append(results[id], pubTX.PublicTx)
			}
		}
	}
	return results, nil
}

func (ptm *pubTxManager) queryPublicTxWithBinding(ctx context.Context, dbTX persistence.DBTX, scopeToTxns []uuid.UUID, jq *query.QueryJSON) ([]*pldapi.PublicTxWithBinding, error) {
	q := dbTX.DB().Table("public_txns").
		WithContext(ctx).
		Joins("Completed")
	if jq != nil {
		q = filters.BuildGORM(ctx, jq, q, components.PublicTxFilterFields)
	}
	ptxs, err := ptm.runTransactionQuery(ctx, dbTX, true /* one record per TX binding */, scopeToTxns, q)
	if err != nil {
		return nil, err
	}
	results := make([]*pldapi.PublicTxWithBinding, len(ptxs))
	for iTx, ptx := range ptxs {
		tx := mapPersistedTransaction(ptx)
		tx.Submissions = make([]*pldapi.PublicTxSubmissionData, len(ptx.Submissions))
		for iSub, pSub := range ptx.Submissions {
			tx.Submissions[iSub] = mapPersistedSubmissionData(pSub)
		}
		tx.Activity = ptm.getActivityRecords(ptx.PublicTxnID)
		results[iTx] = &pldapi.PublicTxWithBinding{
			PublicTx: tx,
		}
		if ptx.Binding != nil {
			results[iTx].PublicTxBinding = pldapi.PublicTxBinding{
				Transaction:     ptx.Binding.Transaction,
				TransactionType: ptx.Binding.TransactionType,
			}
		}
	}
	return results, nil
}

func (ptm *pubTxManager) CheckTransactionCompleted(ctx context.Context, pubTxnID uint64) (bool, error) {
	// Runs a DB query to see if the transaction is marked completed (for good or bad)
	// A non existent transaction results in false
	var ptxs []*DBPublicTxn
	err := ptm.p.DB().
		WithContext(ctx).
		Table("public_txns").
		Where(`"public_txns"."pub_txn_id" = ?`, pubTxnID).
		Joins("Completed").
		Select(`"Completed"."tx_hash"`).
		Limit(1).
		Find(&ptxs).
		Error
	if err != nil {
		return false, err
	}
	if len(ptxs) > 0 && ptxs[0].Completed != nil {
		log.L(ctx).Debugf("CheckTransactionCompleted returned true for %s:%d (pubTxnID=%d)", ptxs[0].From, ptxs[0].Nonce, pubTxnID)
		return true, nil
	}
	return false, nil
}

func (ptm *pubTxManager) runTransactionQuery(ctx context.Context, dbTX persistence.DBTX, bindings bool, scopeToTxns []uuid.UUID, q *gorm.DB) (ptxs []*DBPublicTxn, err error) {
	if bindings {
		// We'll get one row per binding
		q = q.Joins("Binding")
	}
	if scopeToTxns != nil {
		// which can be scoped to a set of transactions
		q = q.Where(`"Binding"."transaction" IN (?)`, scopeToTxns)
	}
	err = q.Find(&ptxs).Error
	if err != nil {
		return nil, err
	}
	publicTxRefs := make([]uint64, len(ptxs))
	for i, ptx := range ptxs {
		publicTxRefs[i] = ptx.PublicTxnID
	}
	if len(publicTxRefs) > 0 {
		allSubs, err := ptm.getTransactionSubmissions(ctx, dbTX, publicTxRefs)
		if err != nil {
			return nil, err
		}
		for _, sub := range allSubs {
			for _, ptx := range ptxs {
				if sub.PublicTxnID == ptx.PublicTxnID {
					ptx.Submissions = append(ptx.Submissions, sub)
				}
			}
		}
	}
	return ptxs, nil
}

func mapPersistedTransaction(ptx *DBPublicTxn) *pldapi.PublicTx {
	tx := &pldapi.PublicTx{
		LocalID: &ptx.PublicTxnID,
		From:    ptx.From,
		Created: ptx.Created,
		To:      ptx.To,
		Nonce:   (*pldtypes.HexUint64)(ptx.Nonce),
		Data:    ptx.Data,
		PublicTxOptions: pldapi.PublicTxOptions{
			Gas:                (*pldtypes.HexUint64)(&ptx.Gas),
			Value:              ptx.Value,
			PublicTxGasPricing: recoverGasPriceOptions(ptx.FixedGasPricing),
		},
	}
	// We use a separate Table in the DB for the completion data, but
	// we allow a single query and return interface for users.
	if ptx.Completed != nil {
		completed := ptx.Completed
		tx.CompletedAt = &completed.Created
		tx.TransactionHash = &completed.TransactionHash
		tx.Success = &completed.Success
		tx.RevertData = completed.RevertData
	}
	// Note: Submissions (sent to the mempool of the chain, but not yet complete) are separate.
	// See mapPersistedSubmissionData()
	return tx
}

func mapPersistedSubmissionData(pSub *DBPubTxnSubmission) *pldapi.PublicTxSubmissionData {
	return &pldapi.PublicTxSubmissionData{
		Time:               pSub.Created,
		TransactionHash:    pldtypes.Bytes32(pSub.TransactionHash),
		PublicTxGasPricing: recoverGasPriceOptions(pSub.GasPricing),
	}
}

func (ptm *pubTxManager) getTransactionSubmissions(ctx context.Context, dbTX persistence.DBTX, pubTxnIDs []uint64) ([]*DBPubTxnSubmission, error) {
	var ptxs []*DBPubTxnSubmission
	err := dbTX.DB().
		WithContext(ctx).
		Table("public_submissions").
		Where("pub_txn_id IN (?)", pubTxnIDs).
		Order("created DESC").
		Find(&ptxs).
		Error
	return ptxs, err
}

func (ptm *pubTxManager) SuspendTransaction(ctx context.Context, from pldtypes.EthAddress, nonce uint64) error {
	if err := ptm.dispatchAction(ctx, from, nonce, ActionSuspend); err != nil {
		return err
	}
	return nil
}

func (ptm *pubTxManager) ResumeTransaction(ctx context.Context, from pldtypes.EthAddress, nonce uint64) error {
	if err := ptm.dispatchAction(ctx, from, nonce, ActionResume); err != nil {
		return err
	}
	return nil
}

func (ptm *pubTxManager) UpdateTransaction(ctx context.Context, id uuid.UUID, pubTXID uint64, from *pldtypes.EthAddress, tx *pldapi.TransactionInput, publicTxData []byte, txmgrDBUpdate func(dbTX persistence.DBTX) error) error {
	ptxs := []*DBPublicTxn{}
	err := ptm.p.DB().
		WithContext(ctx).
		Table("public_txns").
		Where(`"pub_txn_id" = ?`, pubTXID).
		Limit(1).
		Find(&ptxs).
		Error
	if err != nil {
		return err
	}
	if len(ptxs) == 0 {
		log.L(ctx).Warnf("UpdateTransaction: Public transaction local id not found: %d (%+v)", pubTXID, id)
		return i18n.NewError(ctx, msgs.MsgPublicTransactionNotFound, id)
	}

	// error if the transaction is already completed
	complete, err := ptm.CheckTransactionCompleted(ctx, pubTXID)
	if err != nil {
		return err
	}
	if complete {
		log.L(ctx).Warnf("UpdateTransaction: Public transaction already completed: %d (%+v)", pubTXID, id)
		return i18n.NewError(ctx, msgs.MsgTransactionAlreadyComplete, id)
	}

	if tx.Gas == nil || *tx.Gas == 0 {
		ethTx := buildEthTX(*from, nil, tx.To, publicTxData, &tx.PublicTxOptions)
		gasEstimateResult, err := ptm.ethClient.EstimateGasNoResolve(ctx, ethTx)
		if err != nil {
			log.L(ctx).Errorf("EstimateGas error estimating gas for transaction: %+v, request: (%+v)", err, ethTx)
			if ethclient.MapSubmissionRejected(err) {
				// transaction is rejected. We can build a useful error message hopefully by processing the rejection info
				if len(gasEstimateResult.RevertData) > 0 {
					// we can use the error dictionary callback to TXManager to look up the ABI
					err = ptm.rootTxMgr.CalculateRevertError(ctx, ptm.p.NOTX(), gasEstimateResult.RevertData)
					log.L(ctx).Warnf("Estimate gas reverted: %s", err.Error())
				}
			}
			return err
		}
		tx.Gas = &gasEstimateResult.GasLimit
	}

	newPtx := &DBPublicTxn{
		From:            *from,
		To:              tx.To,
		Gas:             tx.Gas.Uint64(),
		Value:           tx.Value,
		Data:            publicTxData,
		FixedGasPricing: pldtypes.JSONString(tx.PublicTxGasPricing),
	}

	ptm.updateMux.Lock()
	defer ptm.updateMux.Unlock()

	err = ptm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		err := txmgrDBUpdate(dbTX)
		if err == nil {
			err = ptm.writeUpdatedTransaction(ctx, dbTX, pubTXID, *from, newPtx)
		}
		return err
	})

	if err == nil {
		ptm.dispatchUpdate(&transactionUpdate{
			pubTXID: pubTXID,
			from:    from,
			newPtx:  newPtx,
		})
	}

	return err
}

func (ptm *pubTxManager) UpdateSubStatus(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info pldtypes.RawJSON, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
	// TODO: Choose after testing the right way to treat these records - if text is right or not
	if err == nil {
		ptm.addActivityRecord(imtx.GetPubTxnID(),
			i18n.ExpandWithCode(ctx,
				i18n.MessageKey(msgs.MsgPublicTxHistoryInfo),
				imtx.GetFrom(),
				imtx.GetNonce(),
				subStatus,
				action,
				info.String(),
			),
		)
	} else {
		ptm.addActivityRecord(imtx.GetPubTxnID(),
			i18n.ExpandWithCode(ctx,
				i18n.MessageKey(msgs.MsgPublicTxHistoryError),
				imtx.GetFrom(),
				imtx.GetNonce(),
				subStatus,
				action,
				err,
			),
		)
	}

	return nil
}

// add an activity record - this function assumes caller will not add multiple
func (ptm *pubTxManager) addActivityRecord(pubTxnID uint64, msg string) {
	if ptm.maxActivityRecordsPerTx == 0 {
		return
	}
	txr, _ := ptm.activityRecordCache.Get(pubTxnID)
	if txr == nil {
		txr = &txActivityRecords{}
		ptm.activityRecordCache.Set(pubTxnID, txr)
	}
	// We add to the front of the list (newest record first) and cap the size
	txr.lock.Lock()
	defer txr.lock.Unlock()
	record := &pldapi.TransactionActivityRecord{
		Time:    pldtypes.TimestampNow(),
		Message: msg,
	}
	copyLen := len(txr.records)
	if copyLen >= ptm.maxActivityRecordsPerTx {
		copyLen = ptm.maxActivityRecordsPerTx - 1
	}
	newActivity := make([]pldapi.TransactionActivityRecord, copyLen+1)
	copy(newActivity[1:], txr.records[0:copyLen])
	newActivity[0] = *record
	txr.records = newActivity
}

func (ptm *pubTxManager) getActivityRecords(pubTxID uint64) []pldapi.TransactionActivityRecord {
	txr, _ := ptm.activityRecordCache.Get(pubTxID)
	if txr != nil {
		// Snap the current activity array pointer in the lock and return it directly
		// (it does not get modified, only re-allocated on each update)
		txr.lock.Lock()
		defer txr.lock.Unlock()
		return txr.records
	}
	return []pldapi.TransactionActivityRecord{}
}

func (ptm *pubTxManager) GetPublicTransactionForHash(ctx context.Context, dbTX persistence.DBTX, hash pldtypes.Bytes32) (*pldapi.PublicTxWithBinding, error) {
	var publicTxnIDs []uint64
	var txns []*pldapi.PublicTxWithBinding
	err := dbTX.DB().
		Table("public_submissions").
		Model(DBPubTxnSubmission{}).
		Where(`tx_hash = ?`, hash).
		Pluck("pub_txn_id", &publicTxnIDs).
		Limit(1).
		Error
	if err == nil && len(publicTxnIDs) > 0 {
		txns, err = ptm.QueryPublicTxWithBindings(ctx, dbTX, query.NewQueryBuilder().
			Equal("localId", publicTxnIDs[0]).
			Query())
	}
	if err != nil || len(txns) == 0 {
		return nil, err
	}
	return txns[0], nil
}

// note this function guarantees the return order of the matches corresponds to the input order
func (ptm *pubTxManager) MatchUpdateConfirmedTransactions(ctx context.Context, dbTX persistence.DBTX, itxs []*blockindexer.IndexedTransactionNotify) ([]*components.PublicTxMatch, error) {

	// Do a DB query in the TX to reverse lookup the TX details we need to match/update the completed status
	// and return the list that matched (which is very possibly none as we only track transactions submitted
	// via our node to the network).
	txHashes := make([]pldtypes.Bytes32, len(itxs))
	for i, itx := range itxs {
		txHashes[i] = itx.Hash
	}
	var lookups []*bindingsMatchingSubmission

	err := dbTX.DB().
		Table("public_txn_bindings").
		Select(`"transaction"`, "sender", "contract_address", `"tx_type"`, `"Submission"."pub_txn_id"`, `"Submission"."tx_hash"`).
		Joins("Submission").
		Where(`"Submission"."tx_hash" IN (?)`, txHashes).
		Find(&lookups).
		Error
	if err != nil {
		return nil, err
	}

	// Correlate our results with the inputs to build - we guarantee to insert and return
	// the results in the original order
	results := make([]*components.PublicTxMatch, 0, len(lookups))
	completions := make([]*DBPublicTxnCompletion, 0, len(lookups))
	for _, txi := range itxs {
		for _, match := range lookups {
			if txi.Hash.Equals(&match.Submission.TransactionHash) {
				// matched results in the order of the inputs
				log.L(ctx).Debugf("Matched on-chain transaction %s (result=%s): %+v", txi.Hash, txi.Result.V(), match)
				results = append(results, &components.PublicTxMatch{
					PaladinTXReference: components.PaladinTXReference{
						TransactionID:              match.Transaction,
						TransactionType:            match.TransactionType,
						TransactionSender:          match.Sender,
						TransactionContractAddress: match.ContractAddress,
					},
					IndexedTransactionNotify: txi,
				})
				// completions to insert, in the order of the inputs
				completions = append(completions, &DBPublicTxnCompletion{
					PublicTxnID:     match.PublicTxnID,
					TransactionHash: txi.Hash,
					Success:         txi.Result.V() == pldapi.TXResult_SUCCESS,
					RevertData:      txi.RevertReason,
				})
				break
			}
		}
	}

	if len(completions) > 0 {
		// We have some completions to persis - in the same order as the confirmations that came in
		err := dbTX.DB().
			Table("public_completions").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "pub_txn_id"}},
				DoNothing: true, // immuTable
			}).
			Create(completions).
			Error
		if err != nil {
			return nil, err
		}
		ptm.thMetrics.IncCompletedTransactionsByN(uint64(len(completions)))
	}

	return results, nil
}

// We've got to be super careful not to block this thread, so we treat this just like a suspend/resume
// on each of these transactions
func (ptm *pubTxManager) NotifyConfirmPersisted(ctx context.Context, confirms []*components.PublicTxMatch) {
	for _, conf := range confirms {
		_ = ptm.dispatchAction(ctx, *conf.From, conf.Nonce, ActionCompleted)
	}
}
