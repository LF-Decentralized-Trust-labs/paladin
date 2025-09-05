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
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator"
	coordinatorTx "github.com/kaleido-io/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/kaleido-io/paladin/core/internal/sequencer/metrics"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender"
	"github.com/kaleido-io/paladin/core/internal/sequencer/syncpoints"

	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/persistence"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"

	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type distributedSequencerManager struct {
	ctx                           context.Context
	ctxCancel                     func()
	config                        *pldconf.SequencerManagerConfig
	components                    components.AllComponents
	nodeName                      string
	sequencersLock                sync.RWMutex
	syncPoints                    syncpoints.SyncPoints
	subscribersLock               sync.Mutex
	metrics                       metrics.DistributedSequencerMetrics
	sequencers                    map[string]*distributedSequencer
	blockHeight                   int64
	engineIntegration             common.EngineIntegration
	targetActiveCoordinatorsLimit int // Max number of contracts this node aims to concurrently act as coordinator for. It could still efficiently respond to dispatch requests from other coordinators because the sender will remain in memory.
	targetActiveSequencersLimit   int // Max number of sequencers this node aims to retain in memory concurrently. Hitting this limit will cause an attempt to remove the lowest priority sequencer from memory, and hence require it to be recreated from persisted state if it is needed in the future
}

// Init implements Engine.
func (dSmgr *distributedSequencerManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	log.L(dSmgr.ctx).Info("[Sequencer] PreInit distributed sequencer manager")
	dSmgr.metrics = metrics.InitMetrics(dSmgr.ctx, c.MetricsManager().Registry())

	return &components.ManagerInitResult{
		PreCommitHandler: func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
			log.L(ctx).Debug("[Sequencer] SequencerMgr PreCommitHandler")
			latestBlockNumber := blocks[len(blocks)-1].Number
			dbTX.AddPostCommit(func(ctx context.Context) {
				log.L(ctx).Debugf("[Sequencer] SequencerMgr PostCommitHandler: %d", latestBlockNumber)
				dSmgr.OnNewBlockHeight(ctx, latestBlockNumber)
			})
			return nil
		},
	}, nil
}

func (dSmgr *distributedSequencerManager) PostInit(c components.AllComponents) error {
	log.L(dSmgr.ctx).Info("[Sequencer] PostInit distributed sequencer manager")
	dSmgr.components = c
	dSmgr.nodeName = dSmgr.components.TransportManager().LocalNodeName()
	dSmgr.syncPoints = syncpoints.NewSyncPoints(dSmgr.ctx, &dSmgr.config.Writer, c.Persistence(), c.TxManager(), c.PublicTxManager(), c.TransportManager())
	return nil
}

func (dSmgr *distributedSequencerManager) Start() error {
	log.L(dSmgr.ctx).Info("[Sequencer] Starting distributed sequencer manager")
	dSmgr.syncPoints.Start()

	return nil
}

func (dSmgr *distributedSequencerManager) Stop() {
	log.L(dSmgr.ctx).Info("[Sequencer] Stopping distributed sequencer manager")
}

func NewDistributedSequencerManager(ctx context.Context, config *pldconf.SequencerManagerConfig) components.SequencerManager {
	dSmgr := &distributedSequencerManager{
		config:                        config,
		sequencers:                    make(map[string]*distributedSequencer),
		targetActiveCoordinatorsLimit: 10, // MRW TODO configurable
		targetActiveSequencersLimit:   10, // MRW TODO configurable
	}
	dSmgr.ctx, dSmgr.ctxCancel = context.WithCancel(ctx)
	return dSmgr
}

func (dSmgr *distributedSequencerManager) OnNewBlockHeight(ctx context.Context, blockHeight int64) {
	log.L(dSmgr.ctx).Debugf("[Sequencer] block height now %d", blockHeight)
	dSmgr.blockHeight = blockHeight
}

// Synchronous function to submit a deployment request which is asynchronously processed
// Private transaction manager will receive a notification when the public transaction is confirmed
// (same as for invokes)
func (dSmgr *distributedSequencerManager) handleDeployTx(ctx context.Context, tx *components.PrivateContractDeploy) error {
	log.L(ctx).Debugf("[Sequencer] handling new private contract deploy transaction: %v", tx)
	if tx.Domain == "" {
		return i18n.NewError(ctx, msgs.MsgDomainNotProvided)
	}
	dSmgr.metrics.IncAssembledTransactions()
	dSmgr.metrics.IncDispatchedTransactions()

	domain, err := dSmgr.components.DomainManager().GetDomainByName(ctx, tx.Domain)
	log.L(ctx).Debugf("[Sequencer] got domain manager: %v", domain.Name())
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
	go dSmgr.deploymentLoop(log.WithLogField(dSmgr.ctx, "role", "deploy-loop"), domain, tx)

	return nil
}

func (dSmgr *distributedSequencerManager) deploymentLoop(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) {
	log.L(ctx).Info("[Sequencer] starting deployment loop")

	var err error

	// Resolve keys synchronously on this go routine so that we can return an error if any key resolution fails
	tx.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.RequiredVerifiers))
	for i, v := range tx.RequiredVerifiers {
		// TODO: This is a synchronous cross-node exchange, done sequentially for each verifier.
		// Potentially needs to move to an event-driven model like on invocation.
		verifier, resolveErr := dSmgr.components.IdentityResolver().ResolveVerifier(ctx, v.Lookup, v.Algorithm, v.VerifierType)
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
		err = dSmgr.evaluateDeployment(ctx, domain, tx)
	}
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error evaluating deployment: %s", err)
		return
	}
	log.L(ctx).Info("[Sequencer] deployment completed successfully. ")
}

func (dSmgr *distributedSequencerManager) evaluateDeployment(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) error {

	// TODO there is a lot of common code between this and the Dispatch function in the sequencer. should really move some of it into a common place
	// and use that as an opportunity to refactor to be more readable

	err := domain.PrepareDeploy(ctx, tx)
	if err != nil {
		return dSmgr.revertDeploy(ctx, tx, err)
	}

	publicTransactionEngine := dSmgr.components.PublicTxManager()

	// The signer needs to be in our local node or it's an error
	identifier, node, err := pldtypes.PrivateIdentityLocator(tx.Signer).Validate(ctx, dSmgr.nodeName, true)
	if err != nil {
		return err
	}
	if node != dSmgr.nodeName {
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerNonLocalSigningAddr, tx.Signer)
	}

	keyMgr := dSmgr.components.KeyManager()
	resolvedAddrs, err := keyMgr.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
	if err != nil {
		return dSmgr.revertDeploy(ctx, tx, err)
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
		log.L(ctx).Debug("[Sequencer] deploying by invoking a base ledger contract")

		data, err := tx.InvokeTransaction.FunctionABI.EncodeCallDataCtx(ctx, tx.InvokeTransaction.Inputs)
		if err != nil {
			return dSmgr.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrEncodeCallDataFailed))
		}
		publicTXs[0].Data = pldtypes.HexBytes(data)
		publicTXs[0].To = &tx.InvokeTransaction.To

	} else if tx.DeployTransaction != nil {
		// MRW TODO
		return dSmgr.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "[Sequencer] deployTransaction not implemented"))
	} else {
		return dSmgr.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "[Sequencer] neither InvokeTransaction nor DeployTransaction set"))
	}

	for _, pubTx := range publicTXs {
		err := publicTransactionEngine.ValidateTransaction(ctx, dSmgr.components.Persistence().NOTX(), pubTx)
		if err != nil {
			return dSmgr.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, "[Sequencer] PrepareSubmissionBatch failed"))
		}
	}
	log.L(ctx).Debug("[Sequencer] validated transaction")

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

	log.L(ctx).Debug("[Sequencer] persisting deploy dispatch batch")
	// as this is a deploy we specify the null address
	err = dSmgr.syncPoints.PersistDeployDispatchBatch(ctx, dispatchBatch)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error persisting batch: %s", err)
		return dSmgr.revertDeploy(ctx, tx, err)
	}

	return nil
}

func (dSmgr *distributedSequencerManager) revertDeploy(ctx context.Context, tx *components.PrivateContractDeploy, err error) error {
	deployError := i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerDeployError)

	var tryFinalize func()
	tryFinalize = func() {
		dSmgr.syncPoints.QueueTransactionFinalize(ctx, tx.Domain, pldtypes.EthAddress{}, tx.From, tx.ID, deployError.Error(),
			func(ctx context.Context) {
				log.L(ctx).Debugf("[Sequencer] finalized deployment transaction: %s", tx.ID)
			},
			func(ctx context.Context, err error) {
				log.L(ctx).Errorf("[Sequencer] error finalizing deployment: %s", err)
				tryFinalize()
			})
	}
	tryFinalize()
	return deployError

}

func (dSmgr *distributedSequencerManager) HandleNewTx(ctx context.Context, dbTX persistence.DBTX, txi *components.ValidatedTransaction) error {
	log.L(dSmgr.ctx).Info("[Sequencer] handle new TX")
	tx := txi.Transaction
	if tx.To == nil {
		if txi.Transaction.SubmitMode.V() != pldapi.SubmitModeAuto {
			return i18n.NewError(ctx, msgs.MsgPrivateTxMgrPrepareNotSupportedDeploy)
		}
		return dSmgr.handleDeployTx(ctx, &components.PrivateContractDeploy{
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
	log.L(dSmgr.ctx).Info("[Sequencer] handling non-deploy transaction")
	return dSmgr.handleNewTx(ctx, dbTX, &components.PrivateTransaction{
		ID:      *tx.ID,
		Domain:  tx.Domain,
		Address: *tx.To,
		Intent:  intent,
	}, &txi.ResolvedTransaction)
}

// HandleNewTx synchronously receives a new transaction submission
// TODO this should really be a 2 (or 3?) phase handshake with
//   - Pre submit phase to validate the inputs
//   - Submit phase to persist the record of the submission as part of a database transaction that is coordinated by the caller
//   - Post submit phase to clean up any locks / resources that were held during the submission after the database transaction has been committed ( given that we cannot be sure on completeion of phase 2 that the transaction will be committed)
//
// We are currently proving out this pattern on the boundary of the private transaction manager and the public transaction manager and once that has settled, we will implement the same pattern here.
// In the meantime, we a single function to submit a transaction and there is currently no persistence of the submission record.  It is all held in memory only
func (dSmgr *distributedSequencerManager) handleNewTx(ctx context.Context, dbTX persistence.DBTX, tx *components.PrivateTransaction, localTx *components.ResolvedTransaction) error {
	log.L(ctx).Debugf("[Sequencer] handling new transaction: %v", tx)

	contractAddr := *localTx.Transaction.To
	emptyAddress := pldtypes.EthAddress{}
	if contractAddr == emptyAddress {
		return i18n.NewError(ctx, msgs.MsgContractAddressNotProvided)
	}

	domainAPI, err := dSmgr.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
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
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "[Sequencer] PreAssembly is nil")
	}

	sequencer, err := dSmgr.LoadSequencer(ctx, dbTX, contractAddr, domainAPI, tx)
	if err != nil {
		return err
	}

	if tx.PostAssembly == nil {
		// MRW TODO - adopted from private TX mgr behaviour. Is this suitable for distributed sequencer?
		//if we don't know the candidate nodes, and the transaction hasn't been assembled yet, then we can't select a coordinator so just assume we are the coordinator
		// until we get the transaction assembled and then re-evaluate
		log.L(ctx).Debug("[Sequencer] assembly not yet completed - using local node for assembly")
		sequencer.GetSender().SetActiveCoordinator(ctx, dSmgr.nodeName)
	}

	// txList := []*components.PrivateTransaction{tx}

	// MRW TODO - determine our sender's identity?
	// senderIdentity, _ := dSmgr.getTXCommittee(ctx, tx)

	// log.L(ctx).Debugf("DistributedSequencerManager: delegating TX %s to ourselves %s", tx.ID, senderIdentity[0])
	// txDelegatedEvent := &coordinator.TransactionsDelegatedEvent{
	// 	Sender:             senderIdentity[0],
	// 	Transactions:       txList,
	// 	SendersBlockHeight: uint64(d.blockHeight),
	// }

	// MRW for Thursday. It looks like there might be a timing issue, where this sender hasn't stored the transaction before the coordinator
	// asks us to assemble it.

	// MRW TODO - logging through the issue with occasional transactions not proceeding
	log.L(ctx).Debugf("[Sequencer] handling new transaction by creating a TransactionCreatedEvent: %s", tx.ID)
	txCreatedEvent := &sender.TransactionCreatedEvent{
		Transaction: tx,
	}

	dbTX.AddPostCommit(func(ctx context.Context) {
		log.L(ctx).Debugf("[Sequencer] passing TransactionCreatedEvent to the sequencer sender")
		sequencer.GetSender().HandleEvent(ctx, txCreatedEvent)
		dSmgr.metrics.IncAcceptedTransactions()
	})

	return nil
}

func (dSmgr *distributedSequencerManager) getTXCommittee(ctx context.Context, tx *components.PrivateTransaction) ([]string, error) {
	candidateNodesMap := make(map[string]struct{})

	if tx.PostAssembly == nil || len(tx.PostAssembly.AttestationPlan) == 0 {
		identities := make([]string, 0, 1)
		// MRW TODO - temporary hard coding. Probably next thing to sort
		identities = append(identities, "member1@node1")
		return identities, nil
	}

	identities := make([]string, 0, len(tx.PostAssembly.AttestationPlan))
	for _, attestationPlan := range tx.PostAssembly.AttestationPlan {
		if attestationPlan.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attestationPlan.Parties {
				identity, node, err := pldtypes.PrivateIdentityLocator(party).Validate(ctx, dSmgr.nodeName, false)
				if err != nil {
					log.L(ctx).Errorf("[Sequencer] error resolving node for party %s: %s", party, err)
					return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, err)
				}
				candidateNodesMap[node] = struct{}{}
				identities = append(identities, fmt.Sprintf("%s@%s", identity, node))
			}
		}
	}
	candidateNodes := make([]string, 0, len(candidateNodesMap))
	for candidateNode := range candidateNodesMap {
		candidateNodes = append(candidateNodes, candidateNode)
	}
	return candidateNodes, nil
}

func (dSmgr *distributedSequencerManager) DistributeStates(ctx context.Context, stateDistributions []*components.StateDistributionWithData) {
	log.L(dSmgr.ctx).Debugf("[Sequencer] distribute states request (not implemented)")
}

func (dSmgr *distributedSequencerManager) GetBlockHeight() int64 {
	return dSmgr.blockHeight
}

func (dSmgr *distributedSequencerManager) GetNodeName() string {
	return dSmgr.nodeName
}

// func (dSmgr *distributedSequencerManager) getSequencerForContract(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (*distributedSequencer, error) {
// 	return nil, nil
// }

// func (dSmgr *distributedSequencerManager) GetTxStatus(ctx context.Context, domainAddress string, txID uuid.UUID) (status components.PrivateTxStatus, err error) { // MRW TODO - what type of TX status does this return?
// this returns status that we happen to have in memory at the moment and might be useful for debugging

// dSmgr.sequencersLock.RLock()
// defer dSmgr.sequencersLock.RUnlock()
// targetSequencer := dSmgr.sequencers[domainAddress]
// if targetSequencer == nil {
// 	return components.PrivateTxStatus{
// 		TxID:   txID.String(),
// 		Status: "unknown",
// 	}, nil

// } else {
// 	return targetSequencer.GetTxStatus(ctx, txID)
// }
// MRW TODO - what does this look like for distributed sequencer?
//	return nil, nil
// }

func (dSmgr *distributedSequencerManager) HandleNewEvent(ctx context.Context, event string) error {
	log.L(dSmgr.ctx).Debug("[Sequencer] HandleNewEvent (not implemented)")
	// dSmgr.sequencersLock.RLock()
	// defer dSmgr.sequencersLock.RUnlock()
	// targetSequencer := dSmgr.sequencers[event.GetContractAddress()]
	// if targetSequencer == nil { // this is an event that belongs to a contract that's not in flight, throw it away and rely on the engine to trigger the action again when the sequencer is wake up. (an enhanced version is to add weight on queueing an sequencer)
	// 	log.L(ctx).Warnf("Ignored %T event for domain contract %s and transaction %s . If this happens a lot, check the sequencer idle timeout is set to a reasonable number", event, event.GetContractAddress(), event.GetTransactionID())
	// } else {
	// 	targetSequencer.HandleEvent(ctx, event)
	// }
	return nil
}

func (dSmgr *distributedSequencerManager) HandleTransactionCollected(ctx context.Context, signerAddress string, contractAddress string, txID uuid.UUID) error {
	log.L(dSmgr.ctx).Debug("[Sequencer] HandleTransactionCollected")

	// Get the sequencer for the signer address
	sequencer, err := dSmgr.LoadSequencer(ctx, dSmgr.components.Persistence().NOTX(), *pldtypes.MustEthAddress(contractAddress), nil, nil)
	if err != nil {
		return err
	}
	// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
	// so accept the fact that there may not be a sequencer for this public TX submission
	if sequencer != nil {
		collectedEvent := &coordinatorTx.CollectedEvent{
			BaseEvent: coordinatorTx.BaseEvent{
				TransactionID: txID,
			},
			SignerAddress: *pldtypes.MustEthAddress(signerAddress),
		}

		return sequencer.GetCoordinator().HandleEvent(ctx, collectedEvent)
	}

	return nil
}

func (dSmgr *distributedSequencerManager) HandleTransactionConfirmed(ctx context.Context, transactionSender string, confirmedTxn *components.TxCompletion, nonce uint64) error {
	log.L(dSmgr.ctx).Infof("[Sequencer] handling confirmed transaction %s", confirmedTxn.TransactionID)

	contractAddress := pldtypes.EthAddress{}
	if confirmedTxn.PSC != nil {
		contractAddress = confirmedTxn.PSC.Address()
	}

	sequencer, err := dSmgr.LoadSequencer(ctx, dSmgr.components.Persistence().NOTX(), contractAddress, nil, nil)
	if err != nil {
		// MRW TODO - deploys happen without a dedicated sequencer, so this isn't a hard error. We ought
		// to validate if the transaction being confirmed is a deploy, but leaving as-is for now.
		log.L(dSmgr.ctx).Warnf("[Sequencer] failed to obtain sequencer to pass transaction confirmed event to %v:", err)
		return err
	}

	log.L(dSmgr.ctx).Infof("[Sequencer] handing TX confirmed event to coordinator")

	// MRW TODO - currently working around mapping an event to originator to signing address. The sequencers
	// tracks inflight transactions by signing address not signer ID.
	if confirmedTxn.From == "" {
		return fmt.Errorf("[Sequencer]no From address for confirmed transaction %s", confirmedTxn.TransactionID)
	}
	signer, err := dSmgr.components.IdentityResolver().ResolveVerifier(ctx, confirmedTxn.From, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	if err != nil {
		return fmt.Errorf("[Sequencer] error resolving verifier for confirmed transaction: %s", err)
	}

	confirmedEvent := &coordinator.TransactionConfirmedEvent{
		TxID:         confirmedTxn.TransactionID,
		From:         pldtypes.MustEthAddress(signer),
		Hash:         confirmedTxn.ReceiptInput.OnChain.TransactionHash,
		RevertReason: confirmedTxn.ReceiptInput.RevertData,
	}

	coordErr := sequencer.GetCoordinator().HandleEvent(ctx, confirmedEvent)
	if coordErr != nil {
		return coordErr
	}

	// Forward the event to the sender
	transportWriter := sequencer.GetTransportWriter()
	transportWriter.SendTransactionConfirmed(ctx, confirmedTxn.TransactionID, transactionSender, &contractAddress, nonce, confirmedTxn.ReceiptInput.RevertData)

	dSmgr.metrics.IncConfirmedTransactions()

	return nil
}

func (dSmgr *distributedSequencerManager) HandleNonceAssigned(ctx context.Context, transactionSender string, nonce uint64, contractAddress string, txID uuid.UUID) error {
	log.L(dSmgr.ctx).Debug("[Sequencer] HandleNonceAssigned")

	// Get the sequencer for the signer address
	sequencer, err := dSmgr.LoadSequencer(ctx, dSmgr.components.Persistence().NOTX(), *pldtypes.MustEthAddress(contractAddress), nil, nil)
	if err != nil {
		return err
	}
	// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
	// so accept the fact that there may not be a sequencer for this public TX submission
	if sequencer != nil {
		coordinatorNonceAllocatedEvent := &coordinatorTx.NonceAllocatedEvent{
			BaseEvent: coordinatorTx.BaseEvent{
				TransactionID: txID,
			},
			Nonce: nonce,
		}

		coordErr := sequencer.GetCoordinator().HandleEvent(ctx, coordinatorNonceAllocatedEvent)

		if coordErr != nil {
			return coordErr
		}

		// Forward the event to the sender
		transportWriter := sequencer.GetTransportWriter()
		transportWriter.SendNonceAssigned(ctx, txID, transactionSender, pldtypes.MustEthAddress(contractAddress), nonce)

		return nil
	}

	return nil
}

func (dSmgr *distributedSequencerManager) HandlePublicTXSubmission(ctx context.Context, transactionSender string, txHash *pldtypes.Bytes32, contractAddress string, txID uuid.UUID) error {
	log.L(dSmgr.ctx).Debugf("[Sequencer] HandlePublicTXSubmission, hash: %s", txHash)

	sequencer, err := dSmgr.LoadSequencer(ctx, dSmgr.components.Persistence().NOTX(), *pldtypes.MustEthAddress(contractAddress), nil, nil)
	if err != nil {
		return err
	}

	// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
	// so accept the fact that there may not be a sequencer for this public TX submission
	if sequencer != nil {
		coordinatorSubmittedEvent := &coordinatorTx.SubmittedEvent{
			BaseEvent: coordinatorTx.BaseEvent{
				TransactionID: txID,
			},
			SubmissionHash: *txHash,
		}
		coordErr := sequencer.GetCoordinator().HandleEvent(ctx, coordinatorSubmittedEvent)

		if coordErr != nil {
			return coordErr
		}

		// Forward the event to the sender
		transportWriter := sequencer.GetTransportWriter()
		transportWriter.SendTransactionSubmitted(ctx, txID, transactionSender, pldtypes.MustEthAddress(contractAddress), txHash)
	}
	return nil
}

func (dSmgr *distributedSequencerManager) BuildNullifiers(ctx context.Context, stateDistributions []*components.StateDistributionWithData) (nullifiers []*components.NullifierUpsert, err error) {

	nullifiers = []*components.NullifierUpsert{}
	err = dSmgr.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for _, s := range stateDistributions {
			if s.NullifierAlgorithm == nil || s.NullifierVerifierType == nil || s.NullifierPayloadType == nil {
				log.L(ctx).Debugf("[Sequencer] no nullifier required for state %s on node %s", s.StateID, dSmgr.nodeName)
				continue
			}

			nullifier, err := dSmgr.BuildNullifier(ctx, dSmgr.components.KeyManager().KeyResolverForDBTX(dbTX), s)
			if err != nil {
				return err
			}

			nullifiers = append(nullifiers, nullifier)
		}
		return nil
	})
	return nullifiers, err
}

func (dSmgr *distributedSequencerManager) BuildNullifier(ctx context.Context, kr components.KeyResolver, s *components.StateDistributionWithData) (*components.NullifierUpsert, error) {
	// We need to call the signing engine with the local identity to build the nullifier
	log.L(ctx).Infof("[Sequencer] generating nullifier for state %s on node %s (algorithm=%s,verifierType=%s,payloadType=%s)",
		s.StateID, dSmgr.nodeName, *s.NullifierAlgorithm, *s.NullifierVerifierType, *s.NullifierPayloadType)

	// We require a fully qualified identifier for the local node in this function
	identifier, node, err := pldtypes.PrivateIdentityLocator(s.IdentityLocator).Validate(ctx, "", false)
	if err != nil || node != dSmgr.nodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierNotLocal)
	}

	// Call the signing engine to build the nullifier
	var nulliferBytes []byte
	mapping, err := kr.ResolveKey(ctx, identifier, *s.NullifierAlgorithm, *s.NullifierVerifierType)
	if err == nil {
		nulliferBytes, err = dSmgr.components.KeyManager().Sign(ctx, mapping, *s.NullifierPayloadType, s.StateData.Bytes())
	}
	if err != nil || len(nulliferBytes) == 0 {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierFail, s.StateID)
	}
	return &components.NullifierUpsert{
		ID:    nulliferBytes,
		State: pldtypes.MustParseHexBytes(s.StateID),
	}, nil
}

func (dSmgr *distributedSequencerManager) CallPrivateSmartContract(ctx context.Context, call *components.ResolvedTransaction) (*abi.ComponentValue, error) {

	callTx := call.Transaction
	psc, err := dSmgr.components.DomainManager().GetSmartContractByAddress(ctx, dSmgr.components.Persistence().NOTX(), *callTx.To)
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

	for _, r := range requiredVerifiers {
		log.L(ctx).Debugf("[Sequencer] required verifier %s", r.Lookup)
	}

	// Do the verification in-line and synchronously for call (there is caching in the identity resolver)
	identityResolver := dSmgr.components.IdentityResolver()
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
	dCtx := dSmgr.components.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
	defer dCtx.Close()

	// Do the actual call
	return psc.ExecCall(dCtx, dSmgr.components.Persistence().NOTX(), call, verifiers)
}

// MRW TODO - move to sequencer module?
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
