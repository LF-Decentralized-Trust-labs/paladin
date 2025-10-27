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

package sequencer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator"
	coordinatorTx "github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/originator"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/syncpoints"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
)

type sequencerManager struct {
	ctx                           context.Context
	cancelCtx                     func()
	config                        *pldconf.SequencerConfig
	components                    components.AllComponents
	nodeName                      string
	sequencersLock                sync.RWMutex
	syncPoints                    syncpoints.SyncPoints
	metrics                       metrics.DistributedSequencerMetrics
	sequencers                    map[string]*sequencer
	blockHeight                   int64
	engineIntegration             common.EngineIntegration
	targetActiveCoordinatorsLimit int // Max number of contracts this node aims to concurrently act as coordinator for. It could still efficiently respond to dispatch requests from other coordinators because the originator will remain in memory.
	targetActiveSequencersLimit   int // Max number of sequencers this node aims to retain in memory concurrently. Hitting this limit will cause an attempt to remove the lowest priority sequencer from memory, and hence require it to be recreated from persisted state if it is needed in the future
}

// Init implements Engine.
func (sMgr *sequencerManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	log.L(log.WithComponent(sMgr.ctx, common.SUBCOMP_MISC)).Infof("PreInit distributed sequencer manager")
	sMgr.metrics = metrics.InitMetrics(sMgr.ctx, c.MetricsManager().Registry())

	return &components.ManagerInitResult{
		PreCommitHandler: func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
			latestBlockNumber := blocks[len(blocks)-1].Number
			dbTX.AddPostCommit(func(ctx context.Context) {
				sMgr.OnNewBlockHeight(ctx, latestBlockNumber)
			})
			return nil
		},
	}, nil
}

func (sMgr *sequencerManager) PostInit(c components.AllComponents) error {
	log.L(log.WithComponent(sMgr.ctx, common.SUBCOMP_MISC)).Infof("PostInit distributed sequencer manager")
	sMgr.components = c
	sMgr.nodeName = sMgr.components.TransportManager().LocalNodeName()
	sMgr.syncPoints = syncpoints.NewSyncPoints(sMgr.ctx, &sMgr.config.Writer, c.Persistence(), c.TxManager(), c.PublicTxManager(), c.TransportManager())
	return nil
}

func (sMgr *sequencerManager) Start() error {
	log.L(log.WithComponent(sMgr.ctx, common.SUBCOMP_MISC)).Infof("Starting distributed sequencer manager")
	sMgr.syncPoints.Start()

	// We may have in-flight transactions that never completed. Load any we have pending and and resume them

	// Repeat getting pending transactions until none are returned. Run in a goroutine to avoid blocking the main thread
	go func() {
		for {
			// On startup we can't assemble any transactions without having a confirmed block height so
			// wait until the indexer is ready
			_, err := sMgr.components.BlockIndexer().GetConfirmedBlockHeight(sMgr.ctx)
			if err == nil {
				break
			}

			ctx, cancel := context.WithTimeout(sMgr.ctx, 1*time.Second)
			defer cancel()

			// Wait for the block indexer to be ready
			<-ctx.Done()
			if sMgr.ctx.Err() != nil {
				return
			}
		}

		resumedTransactions := 0
		// MRW TODO - we ought to repeat this process or have an idle sequencer manager loop to periodically checks for incomplete DB transactions to process
		pendingTx, err := sMgr.components.TxManager().QueryTransactionsResolved(sMgr.ctx, query.NewQueryBuilder().Limit(1000).Query(), sMgr.components.Persistence().NOTX(), true)
		if err != nil {
			log.L(sMgr.ctx).Errorf("Error querying pending transactions to resume incomplete ones: %s", err)
		}
		if len(pendingTx) == 0 {
			log.L(sMgr.ctx).Infof("No pending transactions to resume")
			return
		}
		resumedTransactions += len(pendingTx)
		log.L(sMgr.ctx).Infof("Resuming %d transactions", resumedTransactions)
		for _, tx := range pendingTx {
			log.L(sMgr.ctx).Debugf("Resuming pending transaction %s", tx.Transaction.ID)
			err = sMgr.HandleTxResume(sMgr.ctx, &components.ValidatedTransaction{
				ResolvedTransaction: *tx,
			})
			if err != nil {
				log.L(sMgr.ctx).Errorf("Error resuming pending transaction %s: %s", tx.Transaction.ID, err)
			}
		}
	}()

	return nil
}

func (sMgr *sequencerManager) Stop() {
	log.L(log.WithComponent(sMgr.ctx, common.SUBCOMP_MISC)).Infof("Stopping distributed sequencer manager")
	sMgr.cancelCtx()
}

func NewDistributedSequencerManager(ctx context.Context, config *pldconf.SequencerConfig) components.SequencerManager {

	dsmCtx, dsmCtxCancel := context.WithCancel(log.WithLogField(ctx, "role", "sequencer"))
	sMgr := &sequencerManager{
		ctx:                           dsmCtx,
		cancelCtx:                     dsmCtxCancel,
		config:                        config,
		sequencers:                    make(map[string]*sequencer),
		targetActiveCoordinatorsLimit: 10, // MRW TODO configurable
		targetActiveSequencersLimit:   10, // MRW TODO configurable
	}
	return sMgr
}

func (sMgr *sequencerManager) OnNewBlockHeight(ctx context.Context, blockHeight int64) {
	log.L(log.WithComponent(sMgr.ctx, common.SUBCOMP_MISC)).Tracef("new block height %d", blockHeight)
	sMgr.blockHeight = blockHeight
}

// Synchronous function to submit a deployment request which is asynchronously processed
// Private transaction manager will receive a notification when the public transaction is confirmed
// (same as for invokes)
func (sMgr *sequencerManager) handleDeployTx(ctx context.Context, tx *components.PrivateContractDeploy) error {
	log.L(ctx).Debugf("handling new private contract deploy transaction: %v", tx)
	if tx.Domain == "" {
		return i18n.NewError(ctx, msgs.MsgDomainNotProvided)
	}

	domain, err := sMgr.components.DomainManager().GetDomainByName(ctx, tx.Domain)
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainNotFound, tx.Domain)
	}

	err = domain.InitDeploy(ctx, tx)
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDeployInitFailed)
	}

	// this is a transaction that will confirm just like invoke transactions
	// unlike invoke transactions, we don't yet have the sequencer thread to dispatch to so we start a new go routine for each deployment
	// TODO - should have a pool of deployment threads? Maybe size of pool should be one? Or at least one per domain?
	sMgr.metrics.IncDispatchedTransactions()
	go sMgr.deploymentLoop(log.WithLogField(sMgr.ctx, "role", "deploy-loop"), domain, tx)

	return nil
}

func (sMgr *sequencerManager) deploymentLoop(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) {
	log.L(ctx).Info("starting deployment loop")

	var err error

	// Resolve keys synchronously on this go routine so that we can return an error if any key resolution fails
	tx.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.RequiredVerifiers))
	for i, v := range tx.RequiredVerifiers {
		// TODO: This is a synchronous cross-node exchange, done sequentially for each verifier.
		// Potentially needs to move to an event-driven model like on invocation.
		verifier, resolveErr := sMgr.components.IdentityResolver().ResolveVerifier(ctx, v.Lookup, v.Algorithm, v.VerifierType)
		if resolveErr != nil {
			err = i18n.WrapError(ctx, resolveErr, msgs.MsgKeyResolutionFailed, v.Lookup, v.Algorithm, v.VerifierType)
			break
		}
		tx.Verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:       v.Lookup,
			Algorithm:    v.Algorithm,
			Verifier:     verifier,
			VerifierType: v.VerifierType,
		}
	}

	if err == nil {
		err = sMgr.evaluateDeployment(ctx, domain, tx)
	}
	if err != nil {
		log.L(ctx).Errorf("error evaluating deployment: %s", err)
		return
	}

	log.L(ctx).Info("deployment completed successfully")
}

func (sMgr *sequencerManager) evaluateDeployment(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) error {

	// TODO there is a lot of common code between this and the Dispatch function in the sequencer. should really move some of it into a common place
	// and use that as an opportunity to refactor to be more readable

	err := domain.PrepareDeploy(ctx, tx)
	if err != nil {
		return sMgr.revertDeploy(ctx, tx, err)
	}

	publicTransactionEngine := sMgr.components.PublicTxManager()

	// The signer needs to be in our local node or it's an error
	identifier, node, err := pldtypes.PrivateIdentityLocator(tx.Signer).Validate(ctx, sMgr.nodeName, true)
	if err != nil {
		return err
	}
	if node != sMgr.nodeName {
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerNonLocalSigningAddr, tx.Signer)
	}

	keyMgr := sMgr.components.KeyManager()
	resolvedAddrs, err := keyMgr.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
	if err != nil {
		return sMgr.revertDeploy(ctx, tx, err)
	}

	publicTXs := []*components.PublicTxSubmission{
		{
			Bindings: []*components.PaladinTXReference{{TransactionID: tx.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
			PublicTxInput: pldapi.PublicTxInput{
				From:            resolvedAddrs[0],
				PublicTxOptions: pldapi.PublicTxOptions{}, // TODO: Consider propagation from paladin transaction input
			},
		},
	}

	if tx.InvokeTransaction != nil {
		log.L(ctx).Debug("deploying by invoking a base ledger contract")

		data, err := tx.InvokeTransaction.FunctionABI.EncodeCallDataCtx(ctx, tx.InvokeTransaction.Inputs)
		if err != nil {
			return sMgr.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrEncodeCallDataFailed))
		}
		publicTXs[0].Data = pldtypes.HexBytes(data)
		publicTXs[0].To = &tx.InvokeTransaction.To

	} else if tx.DeployTransaction != nil {
		// TODO
		return sMgr.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "deployTransaction not implemented"))
	} else {
		return sMgr.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "neither InvokeTransaction nor DeployTransaction set"))
	}

	for _, pubTx := range publicTXs {
		err := publicTransactionEngine.ValidateTransaction(ctx, sMgr.components.Persistence().NOTX(), pubTx)
		if err != nil {
			return sMgr.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, "PrepareSubmissionBatch failed"))
		}
	}

	//transactions are always dispatched as a sequence, even if only a sequence of one
	sequence := &syncpoints.PublicDispatch{
		PrivateTransactionDispatches: []*syncpoints.DispatchPersisted{
			{
				PrivateTransactionID: tx.ID.String(),
			},
		},
	}
	sequence.PublicTxs = publicTXs
	dispatchBatch := &syncpoints.DispatchBatch{
		PublicDispatches: []*syncpoints.PublicDispatch{
			sequence,
		},
	}

	// as this is a deploy we specify the null address
	err = sMgr.syncPoints.PersistDeployDispatchBatch(ctx, dispatchBatch)
	if err != nil {
		log.L(ctx).Errorf("error persisting batch: %s", err)
		return sMgr.revertDeploy(ctx, tx, err)
	}

	return nil
}

func (sMgr *sequencerManager) revertDeploy(ctx context.Context, tx *components.PrivateContractDeploy, err error) error {
	deployError := i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerDeployError)

	var tryFinalize func()
	tryFinalize = func() {
		sMgr.syncPoints.QueueTransactionFinalize(ctx, tx.Domain, pldtypes.EthAddress{}, tx.From, tx.ID, deployError.Error(),
			func(ctx context.Context) {
				log.L(ctx).Debugf("finalized deployment transaction: %s", tx.ID)
			},
			func(ctx context.Context, err error) {
				log.L(ctx).Errorf("error finalizing deployment: %s", err)
				tryFinalize()
			})
	}
	tryFinalize()
	return deployError
}

// Handling a new transaction. We don't need to persist anything under the DBTX but we do need to ensure the DBTX
// has committed before passing any events to the sequencer to process the tranasction.
func (sMgr *sequencerManager) HandleNewTx(ctx context.Context, dbTX persistence.DBTX, txi *components.ValidatedTransaction) error {
	tx := txi.Transaction
	if tx.To == nil {
		if txi.Transaction.SubmitMode.V() != pldapi.SubmitModeAuto {
			return i18n.NewError(ctx, msgs.MsgPrivateTxMgrPrepareNotSupportedDeploy)
		}
		log.L(sMgr.ctx).Infof("handling deploy transaction %s from signer %s", tx.ID, tx.From)
		return sMgr.handleDeployTx(ctx, &components.PrivateContractDeploy{
			ID:     *tx.ID,
			Domain: tx.Domain,
			From:   tx.From,
			Inputs: tx.Data,
		})
	}
	intent := prototk.TransactionSpecification_SEND_TRANSACTION
	if txi.Transaction.SubmitMode.V() == pldapi.SubmitModeExternal {
		intent = prototk.TransactionSpecification_PREPARE_TRANSACTION
	}
	if txi.Function == nil || txi.Function.Definition == nil {
		return i18n.NewError(ctx, msgs.MsgPrivateTxMgrFunctionNotProvided)
	}
	log.L(sMgr.ctx).Infof("handling transaction %s from signer %s", tx.ID, tx.From)
	return sMgr.handleTx(ctx, dbTX, &components.PrivateTransaction{
		ID:      *tx.ID,
		Domain:  tx.Domain,
		Address: *tx.To,
		Intent:  intent,
	}, &txi.ResolvedTransaction, false)
}

// Resume a transaction we have read from the DB on startup. There is no DBTX because we don't need to delay
// the sequencer running while we wait for the original DB insert to commit.
func (sMgr *sequencerManager) HandleTxResume(ctx context.Context, txi *components.ValidatedTransaction) error {
	tx := txi.Transaction
	if tx.To == nil {
		if txi.Transaction.SubmitMode.V() != pldapi.SubmitModeAuto {
			return i18n.NewError(ctx, msgs.MsgPrivateTxMgrPrepareNotSupportedDeploy)
		}
		log.L(sMgr.ctx).Infof("resuming deploy transaction %s from signer %s", txi.Transaction.ID, txi.Transaction.From)
		return sMgr.handleDeployTx(ctx, &components.PrivateContractDeploy{
			ID:     *tx.ID,
			Domain: tx.Domain,
			From:   tx.From,
			Inputs: tx.Data,
		})
	}
	intent := prototk.TransactionSpecification_SEND_TRANSACTION
	if txi.Transaction.SubmitMode.V() == pldapi.SubmitModeExternal {
		intent = prototk.TransactionSpecification_PREPARE_TRANSACTION
	}
	if txi.Function == nil || txi.Function.Definition == nil {
		return i18n.NewError(ctx, msgs.MsgPrivateTxMgrFunctionNotProvided)
	}
	log.L(sMgr.ctx).Infof("resuming transaction %s from signer %s", tx.ID, tx.From)
	return sMgr.handleTx(ctx, sMgr.components.Persistence().NOTX(), &components.PrivateTransaction{
		ID:      *tx.ID,
		Domain:  tx.Domain,
		Address: *tx.To,
		Intent:  intent,
	}, &txi.ResolvedTransaction, true)
}

// HandleNewTx synchronously receives a new transaction submission
// TODO this should really be a 2 (or 3?) phase handshake with
//   - Pre submit phase to validate the inputs
//   - Submit phase to persist the record of the submission as part of a database transaction that is coordinated by the caller
//   - Post submit phase to clean up any locks / resources that were held during the submission after the database transaction has been committed ( given that we cannot be sure on completeion of phase 2 that the transaction will be committed)
//
// We are currently proving out this pattern on the boundary of the private transaction manager and the public transaction manager and once that has settled, we will implement the same pattern here.
// In the meantime, we a single function to submit a transaction and there is currently no persistence of the submission record.  It is all held in memory only
func (sMgr *sequencerManager) handleTx(ctx context.Context, dbTX persistence.DBTX, tx *components.PrivateTransaction, localTx *components.ResolvedTransaction, resume bool) error {
	contractAddr := *localTx.Transaction.To
	emptyAddress := pldtypes.EthAddress{}
	if contractAddr == emptyAddress {
		return i18n.NewError(ctx, msgs.MsgContractAddressNotProvided)
	}

	domainAPI, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
	if err != nil {
		return err
	}

	domainName := domainAPI.Domain().Name()
	if localTx.Transaction.Domain != "" && domainName != localTx.Transaction.Domain {
		return i18n.NewError(ctx, msgs.MsgPrivateTxMgrDomainMismatch, localTx.Transaction.Domain, domainName, domainAPI.Address())
	}
	localTx.Transaction.Domain = domainName

	err = domainAPI.InitTransaction(ctx, tx, localTx)
	if err != nil {
		return err
	}

	if tx.PreAssembly == nil {
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "PreAssembly is nil")
	}

	sequencer, err := sMgr.LoadSequencer(ctx, dbTX, contractAddr, domainAPI, tx)
	if err != nil {
		return err
	}

	txCreatedEvent := &originator.TransactionCreatedEvent{
		Transaction: tx,
	}

	if !resume {
		dbTX.AddPostCommit(func(ctx context.Context) {
			sequencer.GetOriginator().QueueEvent(ctx, txCreatedEvent)
			sMgr.metrics.IncAcceptedTransactions()
		})
	} else {
		// We're resuming an existing transaction, no need for a post-commit, just handle the TX
		sequencer.GetOriginator().QueueEvent(ctx, txCreatedEvent)
	}

	return nil
}

func (sMgr *sequencerManager) GetBlockHeight() int64 {
	return sMgr.blockHeight
}

func (sMgr *sequencerManager) GetNodeName() string {
	return sMgr.nodeName
}

func (sMgr *sequencerManager) GetTxStatus(ctx context.Context, domainAddress string, txID uuid.UUID) (status components.PrivateTxStatus, err error) {
	// MRW TODO - what type of TX status does this return?
	// MRW TODO - this returns status that we happen to have in memory at the moment and might be useful for debugging

	sequencer, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *pldtypes.MustEthAddress(domainAddress), nil, nil)
	if err != nil || sequencer == nil {
		return components.PrivateTxStatus{
			TxID:   txID.String(),
			Status: "unknown",
		}, err
	}
	return sequencer.GetOriginator().GetTxStatus(ctx, txID)
}

func (sMgr *sequencerManager) HandleTransactionCollected(ctx context.Context, signerAddress string, contractAddress string, txID uuid.UUID) error {
	log.L(sMgr.ctx).Tracef("HandleTransactionCollected %s %s %s", signerAddress, contractAddress, txID.String())

	// Get the sequencer for the signer address
	sequencer, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *pldtypes.MustEthAddress(contractAddress), nil, nil)
	if err != nil {
		return err
	}

	// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
	// so accept the fact that there may not be a sequencer for this public TX submission
	if sequencer != nil {
		collectedEvent := &coordinatorTx.CollectedEvent{
			BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
				TransactionID: txID,
			},
			SignerAddress: *pldtypes.MustEthAddress(signerAddress),
		}

		sequencer.GetCoordinator().QueueEvent(ctx, collectedEvent)
	}

	return nil
}

// Handle nonce assignment by updating our coordination state machine(s)
func (sMgr *sequencerManager) HandleNonceAssigned(ctx context.Context, nonce uint64, contractAddress string, txID uuid.UUID) error {
	log.L(sMgr.ctx).Tracef("HandleNonceAssigned %d %s %s", nonce, contractAddress, txID.String())

	// Get the sequencer for the signer address
	sequencer, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *pldtypes.MustEthAddress(contractAddress), nil, nil)
	if err != nil {
		return err
	}

	// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
	// so accept the fact that there may not be a sequencer for this public TX submission
	if sequencer != nil {
		coordinatorNonceAllocatedEvent := &coordinatorTx.NonceAllocatedEvent{
			BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
				TransactionID: txID,
			},
			Nonce: nonce,
		}

		sequencer.GetCoordinator().QueueEvent(ctx, coordinatorNonceAllocatedEvent)

		coordTx := sequencer.GetCoordinator().GetTransactionByID(ctx, txID)

		if coordTx == nil {
			return fmt.Errorf("transaction %s not found in coordinator, cannot handle nonce assignment event", txID)
		}

		// Forward the event to the originator
		originatorNode := coordTx.OriginatorNode()
		transportWriter := sequencer.GetTransportWriter()
		err := transportWriter.SendNonceAssigned(ctx, txID, originatorNode, pldtypes.MustEthAddress(contractAddress), nonce)
		if err != nil {
			return err
		}

		return nil
	}

	return nil
}

// Handle public TX submission, both for our own coordination state machine(s), and by distributing this public TX submission to other parties who need to have it
func (sMgr *sequencerManager) HandlePublicTXSubmission(ctx context.Context, dbTX persistence.DBTX, txHash *pldtypes.Bytes32, contractAddress string, gasPricing string, txID uuid.UUID) error {
	log.L(sMgr.ctx).Tracef("HandlePublicTXSubmission %s %s %s %s", txHash.String(), contractAddress, gasPricing, txID.String())

	deploy := contractAddress == ""
	if !deploy {
		sequencer, err := sMgr.LoadSequencer(ctx, dbTX, *pldtypes.MustEthAddress(contractAddress), nil, nil)
		if err != nil {
			return err
		}

		// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
		// so accept the fact that there may not be a sequencer for this public TX submission
		if sequencer != nil {
			coordinatorSubmittedEvent := &coordinatorTx.SubmittedEvent{
				BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
					TransactionID: txID,
				},
				SubmissionHash: *txHash,
			}
			sequencer.GetCoordinator().QueueEvent(ctx, coordinatorSubmittedEvent)
			sequencerTX := sequencer.GetCoordinator().GetTransactionByID(ctx, txID)

			if sequencerTX != nil {
				originatorNode := sequencerTX.OriginatorNode()

				// Forward the event to the originator
				transportWriter := sequencer.GetTransportWriter()
				err = transportWriter.SendTransactionSubmitted(ctx, txID, originatorNode, pldtypes.MustEthAddress(contractAddress), txHash)
				if err != nil {
					return err
				}

				// Inform endorsers about the public submission
				tx := sequencer.GetCoordinator().GetTransactionByID(ctx, txID)
				if tx == nil {
					return fmt.Errorf("transaction %s not found in coordinator", txID)
				}

				publicTXSubmission := &pldapi.PublicTxToDistribute{
					TransactionHash: txHash,
					GasPricing:      []byte(gasPricing),
					Bindings: []*pldapi.PublicTxBinding{
						{
							Transaction: txID,
						},
					},
				}

				// Get all endorsers
				for _, endorsement := range tx.PostAssembly.Endorsements {
					if strings.Contains(endorsement.Verifier.Lookup, "@") {
						node := strings.Split(endorsement.Verifier.Lookup, "@")[1]
						if node != sMgr.nodeName {
							// Send reliable message to the node under the current DBTX
							err = sMgr.components.TransportManager().SendReliable(ctx, dbTX, &pldapi.ReliableMessage{
								MessageType: pldapi.RMTPublicTransactionSubmission.Enum(),
								Metadata:    pldtypes.JSONString(publicTXSubmission),
								Node:        node,
							})
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// Distribute locally written public transactions to other parties who need to have the public TX
func (sMgr *sequencerManager) HandlePublicTXsWritten(ctx context.Context, dbTX persistence.DBTX, persistedTxns []*pldapi.PublicTxToDistribute) error {
	log.L(sMgr.ctx).Tracef("HandlePublicTXsWritten %d", len(persistedTxns))

	for _, persistedTxn := range persistedTxns {
		for _, binding := range persistedTxn.Bindings {
			if persistedTxn.To == nil {
				// Deploy not handled by sequencer
				continue
			}
			sequencer, err := sMgr.LoadSequencer(ctx, dbTX, *persistedTxn.To, nil, nil)
			if err != nil {
				return err
			}

			if sequencer != nil {
				tx := sequencer.GetCoordinator().GetTransactionByID(ctx, binding.Transaction)
				if tx == nil {
					return fmt.Errorf("transaction %s not found in coordinator", binding.Transaction)
				}

				// Get all endorsers
				for _, endorsement := range tx.PostAssembly.Endorsements {
					if strings.Contains(endorsement.Verifier.Lookup, "@") {
						node := strings.Split(endorsement.Verifier.Lookup, "@")[1]
						if node != sMgr.nodeName {
							log.L(sMgr.ctx).Infof("Endorser %s is not the sequencer node, send this info to them", node)
							// Send reliable message to the node under the current DBTX
							err = sMgr.components.TransportManager().SendReliable(ctx, dbTX, &pldapi.ReliableMessage{
								MessageType: pldapi.RMTPublicTransaction.Enum(),
								Metadata:    pldtypes.JSONString(persistedTxn),
								Node:        node,
							})
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}

	return nil
}

func (sMgr *sequencerManager) HandleTransactionConfirmed(ctx context.Context, confirmedTxn *components.TxCompletion, from *pldtypes.EthAddress, nonce *pldtypes.HexUint64) error {
	log.L(sMgr.ctx).Tracef("HandleTransactionConfirmed %s %s %+v", confirmedTxn.TransactionID.String(), from.String(), nonce)

	// A transaction can be confirmed after the coordinating node has restarted. The coordinator doesn't persist the private TX, it relies
	// on the originating node to delegate the private TX to it. handleDeleationRequest first checks if a public TX for that request has been confirmed
	// on chain, so in in this context we will assume we have the private TX in memory from which we can determine the originating node for confirmation events.

	var contractAddress pldtypes.EthAddress
	deploy := confirmedTxn.ContractAddress != nil
	if deploy {
		// Creation of a new contract
		contractAddress = *confirmedTxn.ContractAddress
	} else {
		// Invoke of an existing contract
		contractAddress = confirmedTxn.PSC.Address()
	}

	sequencer, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), contractAddress, nil, nil)
	if err != nil {
		return err
	}

	if sequencer != nil {
		if deploy {
			// For a deploy we won't have tracked the transaction through the state machine, but we can load it ready for upcoming transactions and start
			// of with ourselves as the active coordinator
			sequencer.GetCoordinator().SetActiveCoordinatorNode(ctx, sMgr.nodeName)
		} else if sequencer.GetCoordinator().GetActiveCoordinatorNode(ctx) == sMgr.nodeName {
			mtx := sequencer.GetCoordinator().GetTransactionByID(ctx, confirmedTxn.TransactionID)
			if mtx == nil {
				log.L(ctx).Infof("Coordinator not tracking transaction ID %s", confirmedTxn.TransactionID)
				return nil
			}

			if from == nil {
				return fmt.Errorf("nil From address for confirmed transaction %s", confirmedTxn.TransactionID)
			}
			confirmedEvent := &coordinator.TransactionConfirmedEvent{
				TxID:         confirmedTxn.TransactionID,
				From:         from, // The base ledger signing address
				Hash:         confirmedTxn.OnChain.TransactionHash,
				RevertReason: confirmedTxn.RevertData,
				Nonce:        nonce.Uint64(),
			}
			confirmedEvent.EventTime = time.Now()

			sequencer.GetCoordinator().QueueEvent(ctx, confirmedEvent)

			// Forward the event to the originating node. This is only to update the originator's state machine, not for DB confirmation
			transportWriter := sequencer.GetTransportWriter()
			err = transportWriter.SendTransactionConfirmed(ctx, confirmedTxn.TransactionID, mtx.OriginatorNode(), &contractAddress, nonce.Uint64(), confirmedTxn.RevertData)
			if err != nil {
				return err
			}
			sMgr.metrics.IncConfirmedTransactions()
		}
	}

	return nil
}

func (sMgr *sequencerManager) HandleTransactionFailed(ctx context.Context, dbTX persistence.DBTX, failures []*components.PublicTxMatch) error {
	log.L(sMgr.ctx).Tracef("HandleTransactionFailed %d", len(failures))

	privateFailureReceipts := make([]*components.ReceiptInputWithOriginator, len(failures))

	for i, tx := range failures {
		// We calculate the failure message - all errors handled mapped internally here
		privateFailureReceipts[i] = &components.ReceiptInputWithOriginator{
			Originator:            tx.TransactionSender,
			DomainContractAddress: tx.TransactionContractAddress,
			ReceiptInput: components.ReceiptInput{
				ReceiptType:   components.RT_FailedOnChainWithRevertData,
				TransactionID: tx.TransactionID,
				OnChain: pldtypes.OnChainLocation{
					Type:             pldtypes.OnChainTransaction,
					TransactionHash:  tx.Hash,
					BlockNumber:      tx.BlockNumber,
					TransactionIndex: tx.BlockNumber,
				},
				RevertData: tx.RevertReason,
			},
		}
		contractAddress := tx.To

		sequencer, err := sMgr.LoadSequencer(ctx, dbTX, *contractAddress, nil, nil)
		if err != nil {
			return err
		}

		if sequencer != nil {
			mtx := sequencer.GetCoordinator().GetTransactionByID(ctx, tx.TransactionID)
			if mtx == nil {
				return fmt.Errorf("coordinator not tracking transaction ID %s", tx.TransactionID)
			}

			if tx.From == nil {
				return fmt.Errorf("nil From address for confirmed transaction %s", tx.TransactionID)
			}

			failedEvent := &coordinator.TransactionConfirmedEvent{
				TxID:         tx.TransactionID,
				From:         tx.From,
				Hash:         tx.Hash,
				RevertReason: tx.RevertReason,
				Nonce:        tx.Nonce,
			}
			failedEvent.EventTime = time.Now()

			sequencer.GetCoordinator().QueueEvent(ctx, failedEvent)

			// Forward the event to the originating node
			transportWriter := sequencer.GetTransportWriter()
			err = transportWriter.SendTransactionConfirmed(ctx, tx.TransactionID, mtx.OriginatorNode(), contractAddress, tx.Nonce, tx.RevertReason)
			if err != nil {
				// Log but continue for the other receipts
				log.L(sMgr.ctx).Errorf("failed to send transaction confirmed event to originating node %s: %v", mtx.OriginatorNode(), err)
			}

		}
		sMgr.metrics.IncRevertedTransactions()
	}

	// Distribute the receipts to the correct location - either local if we were the submitter, or remote.
	return sMgr.WriteOrDistributeReceiptsPostSubmit(ctx, dbTX, privateFailureReceipts)
}

func (sMgr *sequencerManager) BuildNullifiers(ctx context.Context, stateDistributions []*components.StateDistributionWithData) (nullifiers []*components.NullifierUpsert, err error) {

	nullifiers = []*components.NullifierUpsert{}
	err = sMgr.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for _, s := range stateDistributions {
			if s.NullifierAlgorithm == nil || s.NullifierVerifierType == nil || s.NullifierPayloadType == nil {
				log.L(ctx).Debugf("no nullifier required for state %s on node %s", s.StateID, sMgr.nodeName)
				continue
			}

			nullifier, err := sMgr.BuildNullifier(ctx, sMgr.components.KeyManager().KeyResolverForDBTX(dbTX), s)
			if err != nil {
				return err
			}

			nullifiers = append(nullifiers, nullifier)
		}
		return nil
	})
	return nullifiers, err
}

func (sMgr *sequencerManager) BuildNullifier(ctx context.Context, kr components.KeyResolver, s *components.StateDistributionWithData) (*components.NullifierUpsert, error) {
	// We need to call the signing engine with the local identity to build the nullifier
	log.L(ctx).Debugf("generating nullifier for state %s on node %s (algorithm=%s,verifierType=%s,payloadType=%s)",
		s.StateID, sMgr.nodeName, *s.NullifierAlgorithm, *s.NullifierVerifierType, *s.NullifierPayloadType)

	// We require a fully qualified identifier for the local node in this function
	identifier, node, err := pldtypes.PrivateIdentityLocator(s.IdentityLocator).Validate(ctx, "", false)
	if err != nil || node != sMgr.nodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierNotLocal)
	}

	// Call the signing engine to build the nullifier
	var nulliferBytes []byte
	mapping, err := kr.ResolveKey(ctx, identifier, *s.NullifierAlgorithm, *s.NullifierVerifierType)
	if err == nil {
		nulliferBytes, err = sMgr.components.KeyManager().Sign(ctx, mapping, *s.NullifierPayloadType, s.StateData.Bytes())
	}
	if err != nil || len(nulliferBytes) == 0 {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierFail, s.StateID)
	}
	return &components.NullifierUpsert{
		ID:    nulliferBytes,
		State: pldtypes.MustParseHexBytes(s.StateID),
	}, nil
}

func (sMgr *sequencerManager) CallPrivateSmartContract(ctx context.Context, call *components.ResolvedTransaction) (*abi.ComponentValue, error) {

	callTx := call.Transaction
	psc, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, sMgr.components.Persistence().NOTX(), *callTx.To)
	if err != nil {
		return nil, err
	}

	domainName := psc.Domain().Name()
	if callTx.Domain != "" && domainName != callTx.Domain {
		return nil, i18n.NewError(ctx, msgs.MsgPrivateTxMgrDomainMismatch, callTx.Domain, domainName, psc.Address())
	}
	callTx.Domain = domainName

	// Initialize the call, returning at list of required verifiers
	requiredVerifiers, err := psc.InitCall(ctx, call)
	if err != nil {
		return nil, err
	}

	// Do the verification in-line and synchronously for call (there is caching in the identity resolver)
	identityResolver := sMgr.components.IdentityResolver()
	verifiers := make([]*prototk.ResolvedVerifier, len(requiredVerifiers))
	for i, r := range requiredVerifiers {
		verifier, err := identityResolver.ResolveVerifier(ctx, r.Lookup, r.Algorithm, r.VerifierType)
		if err != nil {
			return nil, err
		}
		verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:       r.Lookup,
			Algorithm:    r.Algorithm,
			VerifierType: r.VerifierType,
			Verifier:     verifier,
		}
	}

	// Create a throwaway domain context for this call
	dCtx := sMgr.components.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
	defer dCtx.Close()

	// Do the actual call
	return psc.ExecCall(dCtx, sMgr.components.Persistence().NOTX(), call, verifiers)
}

func (sMgr *sequencerManager) WriteOrDistributeReceiptsPostSubmit(ctx context.Context, dbTX persistence.DBTX, receipts []*components.ReceiptInputWithOriginator) error {

	// There may be some public reverts that are unrecoverable. This function is potentially where we enforce that. For now, no public revert
	// is considered irrecoverable for private transactions so we don't persist a private TX failure receipt here, we assume that with enough
	// successful re-assembles we will end up with a successful private TX receipt. An assemle revert will still finalise the private TX with
	// a revert receipt.
	// sMgr.syncPoints.WriteOrDistributeReceipts(ctx, dbTX, receipts)

	return nil
}

func mapPreparedTransaction(tx *components.PrivateTransaction) *components.PreparedTransactionWithRefs {
	pt := &components.PreparedTransactionWithRefs{
		PreparedTransactionBase: &pldapi.PreparedTransactionBase{
			ID:       tx.ID,
			Domain:   tx.Domain,
			To:       &tx.Address,
			Metadata: tx.PreparedMetadata,
		},
	}
	for _, s := range tx.PostAssembly.InputStates {
		pt.StateRefs.Spent = append(pt.StateRefs.Spent, s.ID)
	}
	for _, s := range tx.PostAssembly.ReadStates {
		pt.StateRefs.Read = append(pt.StateRefs.Read, s.ID)
	}
	for _, s := range tx.PostAssembly.OutputStates {
		pt.StateRefs.Confirmed = append(pt.StateRefs.Confirmed, s.ID)
	}
	for _, s := range tx.PostAssembly.InfoStates {
		pt.StateRefs.Info = append(pt.StateRefs.Info, s.ID)
	}
	if tx.PreparedPublicTransaction != nil {
		pt.Transaction = *tx.PreparedPublicTransaction
	} else {
		pt.Transaction = *tx.PreparedPrivateTransaction
	}
	return pt
}
