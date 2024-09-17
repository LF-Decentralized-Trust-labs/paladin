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

package publictxmgr

import (
	"context"
	"encoding/json"
	"math/big"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/retry"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/kaleido-io/paladin/core/internal/components"
	baseTypes "github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"

	"github.com/hyperledger/firefly-common/pkg/cache"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
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

// Public Tx Engine:
// - It offers two ways of calculating gas price: use a fixed number, use the built-in API of a ethereum connector
// - It resubmits the transaction based on a configured interval until it succeed or fail
// - It also recalculate gas price during resubmissions
// - It logs errors transactions breach certain configured thresholds of staleness
// - It offers caches of gas price for transactions targeting same method of a smart contract
// - It provide a outbound request concurrency control

type publicTxEngine struct {
	ctx                    context.Context
	thMetrics              *publicTxEngineMetrics
	txStore                baseTypes.TransactionStore
	bIndexer               blockindexer.BlockIndexer
	ethClient              ethclient.EthClient
	managedTXEventNotifier baseTypes.ManagedTxEventNotifier
	keymgr                 ethclient.KeyManager
	// gas price
	gasPriceClient GasPriceClient

	// a map of signing addresses and transaction engines
	InFlightOrchestrators       map[string]*orchestrator
	SigningAddressesPausedUntil map[string]time.Time
	InFlightOrchestratorMux     sync.Mutex
	InFlightOrchestratorStale   chan bool

	// a map of signing addresses and the highest nonce of their completed transactions
	completedTxNoncePerAddress      map[string]big.Int
	completedTxNoncePerAddressMutex sync.Mutex

	// a map of signing addresses and the highest nonce of their confirmed transactions seen from the block indexer
	confirmedTxNoncePerAddress        map[string]*big.Int
	confirmedTxNoncePerAddressRWMutex sync.RWMutex

	// inbound concurrency control TBD

	// engine config
	maxInFlightOrchestrators int
	maxOrchestratorStale     time.Duration
	maxOrchestratorIdle      time.Duration
	maxOverloadProcessTime   time.Duration
	retry                    *retry.Retry
	enginePollingInterval    time.Duration
	engineLoopDone           chan struct{}

	cacheManager cache.Manager
	// balance manager
	balanceManager       baseTypes.BalanceManager
	balanceManagerConfig config.Section

	// orchestrator config
	orchestratorConfig      config.Section
	gasPriceIncreaseMax     *big.Int
	gasPriceIncreasePercent *big.Int
}

func NewTransactionEngine(ctx context.Context, conf config.Section) (baseTypes.PublicTxEngine, error) {
	log.L(ctx).Debugf("Creating new enterprise transaction handler")

	cm := cache.NewCacheManager(ctx, true)

	gasPriceConf := conf.SubSection(GasPriceSection)
	gasPriceCache, _ := cm.GetCache(ctx, "enterprise", "gasPrice", gasPriceConf.GetByteSize(GasPriceCacheSizeByteString), gasPriceConf.GetDuration(GasPriceCacheTTLDurationString), gasPriceConf.GetBool(GasPriceCacheEnabled), cache.StrictExpiry, cache.TTLFromInitialAdd)
	log.L(ctx).Debugf("Gas price cache setting. Enabled: %t , size: %d , ttl: %s", gasPriceConf.GetBool(GasPriceCacheEnabled), gasPriceConf.GetByteSize(GasPriceCacheSizeByteString), gasPriceConf.GetDuration(GasPriceCacheTTLDurationString))

	gasPriceClient := NewGasPriceClient(ctx, gasPriceConf, gasPriceCache)
	engineConfig := conf.SubSection(TransactionEngineSection)
	orchestratorConfig := conf.SubSection(OrchestratorSection)
	balanceManagerConfig := conf.SubSection(BalanceManagerSection)

	var gasPriceIncreaseMax *big.Int
	configuredGasPriceIncreaseMax := &big.Int{}
	gasPriceIncreaseMaxString := orchestratorConfig.GetString(OrchestratorGasPriceIncreaseMaxBigIntString)
	if gasPriceIncreaseMaxString != "" {
		_, ok := configuredGasPriceIncreaseMax.SetString(gasPriceIncreaseMaxString, 10)
		if !ok {
			log.L(ctx).Errorf("Failed to parse max increase gas price %s into a bigInt", gasPriceIncreaseMaxString)
			return nil, i18n.NewError(ctx, msgs.MsgInvalidGasPriceIncreaseMax, gasPriceIncreaseMaxString)
		}
		gasPriceIncreaseMax = configuredGasPriceIncreaseMax
		log.L(ctx).Debugf("Gas price increment gasPriceIncreaseMax setting: %s", gasPriceIncreaseMax.String())
	}

	ble := &publicTxEngine{
		gasPriceClient:              gasPriceClient,
		InFlightOrchestratorStale:   make(chan bool, 1),
		SigningAddressesPausedUntil: make(map[string]time.Time),
		maxInFlightOrchestrators:    engineConfig.GetInt(TransactionEngineMaxInFlightOrchestratorsInt),
		maxOverloadProcessTime:      engineConfig.GetDuration(TransactionEngineMaxOverloadProcessTimeDurationString),
		maxOrchestratorStale:        engineConfig.GetDuration(TransactionEngineMaxStaleDurationString),
		maxOrchestratorIdle:         engineConfig.GetDuration(TransactionEngineMaxIdleDurationString),
		enginePollingInterval:       engineConfig.GetDuration(TransactionEngineIntervalDurationString),
		retry: &retry.Retry{
			InitialDelay: engineConfig.GetDuration(TransactionEngineRetryInitDelayDurationString),
			MaximumDelay: engineConfig.GetDuration(TransactionEngineRetryMaxDelayDurationString),
			Factor:       engineConfig.GetFloat64(TransactionEngineRetryFactorFloat),
		},
		cacheManager:               cm,
		balanceManagerConfig:       balanceManagerConfig,
		completedTxNoncePerAddress: make(map[string]big.Int),
		confirmedTxNoncePerAddress: make(map[string]*big.Int),
		orchestratorConfig:         orchestratorConfig,
		gasPriceIncreaseMax:        gasPriceIncreaseMax,
		gasPriceIncreasePercent:    big.NewInt(orchestratorConfig.GetInt64(OrchestratorGasPriceIncreasePercentageInt)),
	}

	log.L(ctx).Debugf("Enterprise transaction handler created")
	return ble, nil
}

func (ble *publicTxEngine) Init(ctx context.Context, ethClient ethclient.EthClient, keymgr ethclient.KeyManager, txStore baseTypes.TransactionStore, managedTXEventNotifier baseTypes.ManagedTxEventNotifier, blockIndexer blockindexer.BlockIndexer) {
	log.L(ctx).Debugf("Initializing enterprise transaction handler")
	ble.ethClient = ethClient
	ble.keymgr = keymgr
	ble.txStore = txStore
	ble.gasPriceClient.Init(ctx, ethClient)
	ble.managedTXEventNotifier = managedTXEventNotifier
	ble.bIndexer = blockIndexer

	balanceManager, err := NewBalanceManagerWithInMemoryTracking(ctx, ble.balanceManagerConfig, ethClient, ble)
	if err != nil {
		log.L(ctx).Errorf("Failed to create balance manager for enterprise transaction handler due to %+v", err)
		panic(err)
	}
	log.L(ctx).Debugf("Initialized enterprise transaction handler")
	ble.balanceManager = balanceManager
}

func (ble *publicTxEngine) Start(ctx context.Context) (done <-chan struct{}, err error) {
	log.L(ctx).Debugf("Starting enterprise transaction handler")
	if ble.ctx == nil { // only start once
		ble.ctx = ctx // set the context for policy loop
		ble.engineLoopDone = make(chan struct{})
		log.L(ctx).Debugf("Kicking off  enterprise handler engine loop")
		if err = ble.bIndexer.RegisterIndexedTransactionHandler(ctx, ble.HandleConfirmedTransactions); err != nil {
			return
		}
		go ble.engineLoop()
	}
	ble.MarkInFlightOrchestratorsStale()
	log.L(ctx).Infof("Started enterprise transaction handler")
	return ble.engineLoopDone, nil
}

type preparedTransaction struct {
	ethTx *ethsigner.Transaction
	id    string
}

func (txn *preparedTransaction) ID() string {
	return txn.id
}

// HandleEvent
func (ble *publicTxEngine) HandleEvent(ctx context.Context, signingAddress string, txPayload interface{}) (err error) {
	ble.MarkInFlightOrchestratorsStale()
	return nil
}

func (ble *publicTxEngine) PrepareSubmissionBatch(ctx context.Context, reqOptions *baseTypes.RequestOptions, txPayloads []interface{}) (preparedSubmission []baseTypes.PreparedSubmission, submissionRejected bool, err error) {
	preparedSubmissions := make([]baseTypes.PreparedSubmission, len(txPayloads))
	for i, tx := range txPayloads {
		preparedSubmission, submissionRejected, err := ble.PrepareSubmission(ctx, reqOptions, tx)
		if submissionRejected || err != nil {
			return nil, submissionRejected, err
		}
		preparedSubmissions[i] = preparedSubmission
	}
	return preparedSubmissions, false, nil
}

// PrepareSubmission prepares and validates the transaction input data so that a later call to
// Submit can be made in the middle of a wider database transaction with minimal risk of error
func (ble *publicTxEngine) PrepareSubmission(ctx context.Context, reqOptions *baseTypes.RequestOptions, txPayload interface{}) (preparedSubmission baseTypes.PreparedSubmission, submissionRejected bool, err error) {
	log.L(ctx).Tracef("PrepareSubmission new request, options: %+v, payload: %+v", reqOptions, txPayload)
	log.L(ctx).Tracef("HandleNewTx new request, options: %+v, payload: %+v", reqOptions, txPayload)

	err = reqOptions.Validate(ctx)
	if err != nil {
		return nil, true, err
	}
	prepareStart := time.Now()
	var txType InFlightTxOperation

	// this is a transfer only transaction
	// Resolve the key (directly with the signer - we have no key manager here in the teseced)
	_, fromAddr, err := ble.keymgr.ResolveKey(ctx, reqOptions.SignerID, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	if err != nil {
		return nil, false, err
	}

	var ethTx *ethsigner.Transaction
	switch ethPayload := txPayload.(type) {
	case *components.EthTransfer:
		ethTx = &ethsigner.Transaction{
			From:  json.RawMessage(fromAddr),
			To:    ethPayload.To.Address0xHex(),
			Value: ethPayload.Value,
		}
		txType = InFlightTxOperationTransferPreparation
	case *components.EthTransaction:
		abiFunc, err := ble.ethClient.ABIFunction(ctx, ethPayload.FunctionABI)
		if err != nil {
			return nil, false, err
		}
		addr := ethPayload.To.Address0xHex()
		txCallDataBuilder := abiFunc.R(ctx).
			To(addr).
			Input(ethPayload.Inputs)
		buildErr := txCallDataBuilder.BuildCallData()
		if buildErr != nil {
			return nil, false, buildErr
		}
		ethTx = txCallDataBuilder.TX()
		txType = InFlightTxOperationInvokePreparation
	case *components.EthDeployTransaction:
		abiFunc, err := ble.ethClient.ABIConstructor(ctx, ethPayload.ConstructorABI, ethPayload.Bytecode)
		if err != nil {
			return nil, false, err
		}
		txCallDataBuilder := abiFunc.R(ctx).
			Input(ethPayload.Inputs)
		buildErr := txCallDataBuilder.BuildCallData()
		if buildErr != nil {
			return nil, false, buildErr
		}
		ethTx = txCallDataBuilder.TX()
		txType = InFlightTxOperationDeployPreparation
	default:
		return nil, true, i18n.NewError(ctx, msgs.MsgInvalidTransactionType)
	}

	estimatedGasLimit := reqOptions.GasLimit

	if estimatedGasLimit == nil {
		estimatedGasLimitHexInt, err := ble.ethClient.GasEstimate(ctx, ethTx)
		if err != nil {
			log.L(ctx).Errorf("HandleNewTx <%s> error estimating gas for transfer request: %+v, request: (%+v)", txType, err, txPayload)
			ble.thMetrics.RecordOperationMetrics(ctx, string(txType), string(GenericStatusFail), time.Since(prepareStart).Seconds())
			return nil, ethclient.MapSubmissionRejected(err), err
		}
		estimatedGasLimit = estimatedGasLimitHexInt
		log.L(ctx).Tracef("HandleNewTx <%s> using the estimated gas limit %s for transfer request: %+v", txType, estimatedGasLimit.String(), txPayload)
	} else {
		log.L(ctx).Tracef("HandleNewTx <%s> using the provided gas limit %s for transfer request: %+v", txType, estimatedGasLimit.String(), txPayload)
	}
	ethTx.GasLimit = estimatedGasLimit

	ble.gasPriceClient.SetFixedGasPriceIfConfigured(ctx, ethTx)

	ble.thMetrics.RecordOperationMetrics(ctx, string(txType), string(GenericStatusSuccess), time.Since(prepareStart).Seconds())
	log.L(ctx).Debugf("HandleNewTx <%s> creating a new managed transaction with ID %s", txType, reqOptions.ID)
	return &preparedTransaction{
		ethTx: ethTx,
		id:    reqOptions.ID.String(),
	}, false, nil

}

// Submit writes the prepared submission to the database using the provided context
// This is expected to be a lightweight operation involving not much more than writing to the database, as the heavy lifting should have been done in PrepareSubmission
// The database transaction will be coordinated by the caller
func (ble *publicTxEngine) Submit(ctx context.Context, preparedSubmission baseTypes.PreparedSubmission) (mtx *baseTypes.ManagedTX, err error) {
	preparedTransaction := preparedSubmission.(*preparedTransaction)
	mtx, err = ble.createManagedTx(ctx, preparedTransaction.ID(), preparedTransaction.ethTx)
	return mtx, err
}

func (ble *publicTxEngine) SubmitBatch(ctx context.Context, preparedSubmissions []baseTypes.PreparedSubmission) ([]*baseTypes.ManagedTX, error) {
	mtxBatch := make([]*baseTypes.ManagedTX, len(preparedSubmissions))
	for i, preparedSubmission := range preparedSubmissions {
		mtx, err := ble.Submit(ctx, preparedSubmission)
		if err != nil {
			return nil, err
		}
		mtxBatch[i] = mtx
	}
	return mtxBatch, nil
}
func (ble *publicTxEngine) createManagedTx(ctx context.Context, txID string, ethTx *ethsigner.Transaction) (*baseTypes.ManagedTX, error) {
	log.L(ctx).Tracef("createManagedTx creating a new managed transaction with ID: %s, and payload %+v", txID, ethTx)
	now := fftypes.Now()
	mtx := &baseTypes.ManagedTX{
		ID:          txID,
		Created:     now,
		Updated:     now,
		Transaction: ethTx,
		Status:      baseTypes.BaseTxStatusPending,
	}

	log.L(ctx).Tracef("createManagedTx persisting managed transaction %+v", mtx)
	// Sequencing ID will be added as part of persistence logic - so we have a deterministic order of transactions
	// Note: We must ensure persistence happens this within the nonce lock, to ensure that the nonce sequence and the
	//       global transaction sequence line up.
	err := ble.txStore.InsertTransactionWithNextNonce(ctx, mtx, func(ctx context.Context, signer string) (uint64, error) {
		log.L(ctx).Tracef("createManagedTx getting next nonce for transaction ID %s", mtx.ID)
		nextNonce, err := ble.ethClient.GetTransactionCount(ctx, string(ethTx.From))
		if err != nil {
			log.L(ctx).Errorf("createManagedTx getting next nonce for transaction ID %s failed: %+v", mtx.ID, err)
			return 0, err
		}
		log.L(ctx).Tracef("createManagedTx getting next nonce for transaction ID %s succeeded: %s, converting to uint: %d", mtx.ID, nextNonce.String(), nextNonce.Uint64())
		return nextNonce.Uint64(), nil
	})
	if err == nil {
		log.L(ctx).Tracef("createManagedTx persisted transaction with ID: %s, using nonce %s", mtx.ID, mtx.Nonce.String())
		err = ble.txStore.AddSubStatusAction(ctx, txID, baseTypes.BaseTxSubStatusReceived, baseTypes.BaseTxActionAssignNonce, fftypes.JSONAnyPtr(`{"nonce":"`+mtx.Nonce.String()+`"}`), nil, fftypes.Now())
	}
	if err != nil {
		log.L(ctx).Errorf("createManagedTx failed to create managed traction with ID: %s, due to %+v", mtx.ID, err)
		return nil, err
	}
	log.L(ctx).Debugf("createManagedTx a new managed transaction with ID %s is persisted", mtx.ID)
	ble.MarkInFlightOrchestratorsStale()

	return mtx, nil
}

func (ble *publicTxEngine) HandleNewTransaction(ctx context.Context, reqOptions *baseTypes.RequestOptions, txPayload interface{}) (mtx *baseTypes.ManagedTX, submissionRejected bool, err error) {
	preparedSubmission, submissionRejected, err := ble.PrepareSubmission(ctx, reqOptions, txPayload)
	if submissionRejected || err != nil {
		return nil, submissionRejected, err
	}
	mtx, err = ble.Submit(ctx, preparedSubmission)
	return
}

// HandleConfirmedTransactions
// handover events to the inflight orchestrators for the related signing addresses and record the highest confirmed nonce
// new orchestrators will be created if there are space, orchestrators will use the recorded highest nonce to drive completion logic of transactions
func (ble *publicTxEngine) HandleConfirmedTransactions(ctx context.Context, confirmedTransactions []*blockindexer.IndexedTransaction) error {
	// firstly, we group the confirmed transactions by from address
	// note: filter out transactions that are before the recorded nonce in confirmedTXNonce map requires multiple reads to a single address (as the loop keep switching between addresses)
	// so we delegate the logic to the orchestrator as it will have a list of records for a single address
	itMap := make(map[string]map[string]*blockindexer.IndexedTransaction)
	itMaxNonce := make(map[string]*big.Int)
	for _, it := range confirmedTransactions {
		itNonce := new(big.Int).SetUint64(it.Nonce)
		if itMap[it.From.String()] == nil {
			itMap[it.From.String()] = map[string]*blockindexer.IndexedTransaction{itNonce.String(): it}
		} else {
			itMap[it.From.String()][itNonce.String()] = it
		}
		if itMaxNonce[it.From.String()] == nil || itMaxNonce[it.From.String()].Cmp(itNonce) == -1 {
			itMaxNonce[it.From.String()] = itNonce
		}
	}
	if len(itMap) > 0 {
		// secondly, we obtain the lock for the orchestrator map:
		ble.InFlightOrchestratorMux.Lock()
		defer ble.InFlightOrchestratorMux.Unlock() // note, using lock might cause the event sequence to get lost when this function is invoked concurrently by several go routines, this code assumes the upstream logic does not do that

		//     for address that has or could have a running orchestrator, triggers event handlers of each orchestrator in parallel to handle the event
		//         (logic implemented in orchestrator handler)for the orchestrator handler, it obtains the stage process buffer lock and add the event into the stage process buffer and then exit

		localRWLock := sync.RWMutex{} // could consider switch InFlightOrchestrators to use sync.Map for this logic here as the go routines will only modify disjoint set of keys
		eventHandlingErrors := make(chan error, len(itMap))
		for from, its := range itMap {
			fromAddress := from
			go func() {
				localRWLock.RLock()
				inFlightOrchestrator := ble.InFlightOrchestrators[fromAddress]
				localRWLock.RUnlock()
				if inFlightOrchestrator == nil {
					localRWLock.Lock()
					itTotal := len(ble.InFlightOrchestrators)
					if itTotal < ble.maxInFlightOrchestrators {
						inFlightOrchestrator = NewOrchestrator(ble, fromAddress, ble.orchestratorConfig)
						ble.InFlightOrchestrators[fromAddress] = inFlightOrchestrator
						_, _ = inFlightOrchestrator.Start(ble.ctx)
						log.L(ctx).Infof("(Event handler) Engine added orchestrator for signing address %s", fromAddress)
						localRWLock.Unlock()
					} else {
						// no action can be taken
						log.L(ctx).Debugf("(Event handler) Cannot add orchestrator for signing address %s due to in-flight queue is full", fromAddress)
						localRWLock.Unlock()
						eventHandlingErrors <- nil
						return
					}
				}
				err := inFlightOrchestrator.HandleConfirmedTransactions(ctx, its, itMaxNonce[fromAddress])
				// finally, we update the confirmed nonce for each address to the highest number that is observed ever. This then can be used by the orchestrator to retrospectively fetch missed confirmed transaction data.
				ble.updateConfirmedTxNonce(fromAddress, itMaxNonce[fromAddress])
				eventHandlingErrors <- err
			}()
		}

		resultCount := 0
		var accumulatedError error

		// wait for all add output to complete
		for {
			select {
			case err := <-eventHandlingErrors:
				if err != nil {
					accumulatedError = err
				}
				resultCount++
			case <-ctx.Done():
				return i18n.NewError(ctx, msgs.MsgContextCanceled)
			}
			if resultCount == len(itMap) {
				break
			}
		}
		return accumulatedError
	}
	return nil
}

func (ble *publicTxEngine) HandleSuspendTransaction(ctx context.Context, txID string) (mtx *baseTypes.ManagedTX, err error) {
	mtx, err = ble.txStore.GetTransactionByID(ctx, txID)
	if err != nil {
		return nil, err
	}
	res := ble.dispatchAction(ctx, mtx, ActionSuspend)
	if res.err != nil {
		return nil, res.err
	}
	return res.tx, nil
}

func (ble *publicTxEngine) HandleResumeTransaction(ctx context.Context, txID string) (mtx *baseTypes.ManagedTX, err error) {
	mtx, err = ble.txStore.GetTransactionByID(ctx, txID)
	if err != nil {
		return nil, err
	}
	res := ble.dispatchAction(ctx, mtx, ActionResume)
	if res.err != nil {
		return nil, res.err
	}
	return res.tx, nil
}

func (ble *publicTxEngine) getConfirmedTxNonce(addr string) (nonce *big.Int) {
	ble.confirmedTxNoncePerAddressRWMutex.RLock()
	nonce = ble.confirmedTxNoncePerAddress[addr]
	defer ble.confirmedTxNoncePerAddressRWMutex.RUnlock()
	return
}

func (ble *publicTxEngine) updateConfirmedTxNonce(addr string, nonce *big.Int) {
	ble.confirmedTxNoncePerAddressRWMutex.Lock()
	defer ble.confirmedTxNoncePerAddressRWMutex.Unlock()
	if ble.confirmedTxNoncePerAddress[addr] == nil || ble.confirmedTxNoncePerAddress[addr].Cmp(nonce) != 1 {
		ble.confirmedTxNoncePerAddress[addr] = nonce
	}
}
