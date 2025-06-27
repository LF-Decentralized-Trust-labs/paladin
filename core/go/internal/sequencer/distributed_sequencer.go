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
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator"
	"github.com/kaleido-io/paladin/core/internal/sequencer/metrics"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender"
	"github.com/kaleido-io/paladin/core/internal/sequencer/syncpoints"
	"github.com/kaleido-io/paladin/core/internal/sequencer/transport"

	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/core/pkg/persistence"
	pbEngine "github.com/kaleido-io/paladin/core/pkg/proto/engine"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"google.golang.org/protobuf/proto"

	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type distributedSequencer struct {
	sender      sender.SeqSender
	coordinator coordinator.SeqCoordinator
}

type distributedSequencerManager struct {
	ctx             context.Context
	ctxCancel       func()
	config          *pldconf.DistributedSequencerManagerConfig
	components      components.AllComponents
	nodeName        string
	sequencersLock  sync.RWMutex
	syncPoints      syncpoints.SyncPoints
	subscribers     []components.PrivateTxEventSubscriber
	subscribersLock sync.Mutex
	metrics         metrics.DistributedSequencerMetrics
	sequencers      map[string]*distributedSequencer
	blockHeight     int64
}

// Init implements Engine.
func (d *distributedSequencerManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	log.L(d.ctx).Info("PreInit distributed sequencer manager")
	d.metrics = metrics.InitMetrics(d.ctx, c.MetricsManager().Registry())

	return &components.ManagerInitResult{
		// PreCommitHandler: func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
		// 	log.L(ctx).Debug("PrivateTxManager PreCommitHandler")
		// 	latestBlockNumber := blocks[len(blocks)-1].Number
		// 	dbTX.AddPostCommit(func(ctx context.Context) {
		// 		log.L(ctx).Debugf("PrivateTxManager PostCommitHandler: %d", latestBlockNumber)
		// 		p.OnNewBlockHeight(ctx, latestBlockNumber)
		// 	})
		// 	return nil
		// },
	}, nil
}

func (d *distributedSequencerManager) PostInit(c components.AllComponents) error {
	log.L(d.ctx).Info("PostInit distributed sequencer manager")
	d.components = c
	d.nodeName = d.components.TransportManager().LocalNodeName()
	d.syncPoints = syncpoints.NewSyncPoints(d.ctx, &d.config.Writer, c.Persistence(), c.TxManager(), c.PublicTxManager(), c.TransportManager())
	return nil
}

func (d *distributedSequencerManager) Start() error {
	log.L(d.ctx).Info("Starting distributed sequencer manager")
	d.syncPoints.Start()

	return nil
}

func (d *distributedSequencerManager) Stop() {
	log.L(d.ctx).Info("Stopping distributed sequencer manager")
}

func NewDistributedSequencerManager(ctx context.Context, config *pldconf.DistributedSequencerManagerConfig) components.DistributedSequencerManager {
	d := &distributedSequencerManager{
		config:      config,
		subscribers: make([]components.PrivateTxEventSubscriber, 0),
	}
	d.ctx, d.ctxCancel = context.WithCancel(ctx)
	return d
}

func (d *distributedSequencerManager) OnNewBlockHeight(ctx context.Context, blockHeight int64) {
	log.L(d.ctx).Debugf("Distributed sequencer manager block height now %d", blockHeight)
	d.blockHeight = blockHeight
}

// Synchronous function to submit a deployment request which is asynchronously processed
// Private transaction manager will receive a notification when the public transaction is confirmed
// (same as for invokes)
func (d *distributedSequencerManager) handleDeployTx(ctx context.Context, tx *components.PrivateContractDeploy) error {
	log.L(ctx).Debugf("Distributed sequencer handling new private contract deploy transaction: %v", tx)
	if tx.Domain == "" {
		return i18n.NewError(ctx, msgs.MsgDomainNotProvided)
	}
	d.metrics.IncAssembledTransactions()
	d.metrics.IncDispatchedTransactions()

	domain, err := d.components.DomainManager().GetDomainByName(ctx, tx.Domain)
	log.L(ctx).Debugf("Distributed sequencer got domain manager: %v", domain.Name())
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
	go d.deploymentLoop(log.WithLogField(d.ctx, "role", "deploy-loop"), domain, tx)

	return nil
}

func (d *distributedSequencerManager) deploymentLoop(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) {
	log.L(ctx).Info("Distributed sequencer starting deployment loop")

	var err error

	// Resolve keys synchronously on this go routine so that we can return an error if any key resolution fails
	tx.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.RequiredVerifiers))
	for i, v := range tx.RequiredVerifiers {
		// TODO: This is a synchronous cross-node exchange, done sequentially for each verifier.
		// Potentially needs to move to an event-driven model like on invocation.
		verifier, resolveErr := d.components.IdentityResolver().ResolveVerifier(ctx, v.Lookup, v.Algorithm, v.VerifierType)
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
		err = d.evaluateDeployment(ctx, domain, tx)
	}
	if err != nil {
		log.L(ctx).Errorf("Error evaluating deployment: %s", err)
		return
	}
	log.L(ctx).Info("Distributed sequencer deployment completed successfully. ")
}

func (d *distributedSequencerManager) evaluateDeployment(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) error {

	// TODO there is a lot of common code between this and the Dispatch function in the sequencer. should really move some of it into a common place
	// and use that as an opportunity to refactor to be more readable

	err := domain.PrepareDeploy(ctx, tx)
	if err != nil {
		return d.revertDeploy(ctx, tx, err)
	}

	publicTransactionEngine := d.components.PublicTxManager()

	// The signer needs to be in our local node or it's an error
	identifier, node, err := pldtypes.PrivateIdentityLocator(tx.Signer).Validate(ctx, d.nodeName, true)
	if err != nil {
		return err
	}
	if node != d.nodeName {
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerNonLocalSigningAddr, tx.Signer)
	}

	keyMgr := d.components.KeyManager()
	resolvedAddrs, err := keyMgr.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
	if err != nil {
		return d.revertDeploy(ctx, tx, err)
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
		log.L(ctx).Debug("Distributed sequencer manager deploying by invoking a base ledger contract")

		data, err := tx.InvokeTransaction.FunctionABI.EncodeCallDataCtx(ctx, tx.InvokeTransaction.Inputs)
		if err != nil {
			return d.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrEncodeCallDataFailed))
		}
		publicTXs[0].Data = pldtypes.HexBytes(data)
		publicTXs[0].To = &tx.InvokeTransaction.To

	} else if tx.DeployTransaction != nil {
		//TODO
		return d.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "DeployTransaction not implemented"))
	} else {
		return d.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "Neither InvokeTransaction nor DeployTransaction set"))
	}

	for _, pubTx := range publicTXs {
		err := publicTransactionEngine.ValidateTransaction(ctx, d.components.Persistence().NOTX(), pubTx)
		if err != nil {
			return d.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, "PrepareSubmissionBatch failed"))
		}
	}
	log.L(ctx).Debug("Distributed sequencer manager validated transaction")

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

	log.L(ctx).Debug("Distributed sequencer manager persisting deploy dispatch batch")
	// as this is a deploy we specify the null address
	err = d.syncPoints.PersistDeployDispatchBatch(ctx, dispatchBatch)
	if err != nil {
		log.L(ctx).Errorf("Error persisting batch: %s", err)
		return d.revertDeploy(ctx, tx, err)
	}

	// MRW TODO - publish local event
	d.publishToSubscribers(ctx, &components.TransactionDispatchedEvent{
		TransactionID:  tx.ID.String(),
		Nonce:          uint64(0), /*TODO*/
		SigningAddress: tx.Signer,
	})

	return nil
}

// For now, this is here to help with testing but it seems like it could be useful thing to have
// in the future if we want to have an eventing interface but at such time we would need to put more effort
// into the reliability of the event delivery or maybe there is only a consumer of the event and it is responsible
// for managing multiple subscribers and durability etc...
func (d *distributedSequencerManager) Subscribe(ctx context.Context, subscriber components.PrivateTxEventSubscriber) {
	d.subscribersLock.Lock()
	defer d.subscribersLock.Unlock()
	//TODO implement this
	d.subscribers = append(d.subscribers, subscriber)
}

func (d *distributedSequencerManager) publishToSubscribers(ctx context.Context, event components.PrivateTxEvent) {
	log.L(ctx).Debugf("Publishing event to subscribers")
	d.subscribersLock.Lock()
	defer d.subscribersLock.Unlock()
	for _, subscriber := range d.subscribers {
		subscriber(event)
	}
}

func (d *distributedSequencerManager) revertDeploy(ctx context.Context, tx *components.PrivateContractDeploy, err error) error {
	deployError := i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerDeployError)

	var tryFinalize func()
	tryFinalize = func() {
		d.syncPoints.QueueTransactionFinalize(ctx, tx.Domain, pldtypes.EthAddress{}, tx.From, tx.ID, deployError.Error(),
			func(ctx context.Context) {
				log.L(ctx).Debugf("Finalized deployment transaction: %s", tx.ID)
			},
			func(ctx context.Context, err error) {
				log.L(ctx).Errorf("Error finalizing deployment: %s", err)
				tryFinalize()
			})
	}
	tryFinalize()
	return deployError

}

func (d *distributedSequencerManager) HandleNewTx(ctx context.Context, dbTX persistence.DBTX, txi *components.ValidatedTransaction) error {
	log.L(d.ctx).Info("Distributed sequencer manager HandleNewTx")
	tx := txi.Transaction
	if tx.To == nil {
		if txi.Transaction.SubmitMode.V() != pldapi.SubmitModeAuto {
			return i18n.NewError(ctx, msgs.MsgPrivateTxMgrPrepareNotSupportedDeploy)
		}
		return d.handleDeployTx(ctx, &components.PrivateContractDeploy{
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
	log.L(d.ctx).Info("Distributed sequencer non-deploy transaction")
	return d.handleNewTx(ctx, dbTX, &components.PrivateTransaction{
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
func (d *distributedSequencerManager) handleNewTx(ctx context.Context, dbTX persistence.DBTX, tx *components.PrivateTransaction, localTx *components.ResolvedTransaction) error {
	log.L(ctx).Debugf("DistributedSequencerManager: Handling new transaction: %v", tx)

	contractAddr := *localTx.Transaction.To
	emptyAddress := pldtypes.EthAddress{}
	if contractAddr == emptyAddress {
		return i18n.NewError(ctx, msgs.MsgContractAddressNotProvided)
	}

	domainAPI, err := d.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
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

	sequencer, err := d.startSequencerForContract(ctx, dbTX, contractAddr, domainAPI, tx)
	if err != nil {
		return err
	}

	txEvent := &sender.TransactionCreatedEvent{
		Transaction: tx,
	}

	sequencer.sender.HandleEvent(ctx, txEvent)

	return nil
}

func (d *distributedSequencerManager) getTXCommittee(ctx context.Context, tx *components.PrivateTransaction) ([]string, error) {
	candidateNodesMap := make(map[string]struct{})
	identities := make([]string, 0, len(tx.PostAssembly.AttestationPlan))
	for _, attestationPlan := range tx.PostAssembly.AttestationPlan {
		if attestationPlan.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attestationPlan.Parties {
				identity, node, err := pldtypes.PrivateIdentityLocator(party).Validate(ctx, d.nodeName, false)
				if err != nil {
					log.L(ctx).Errorf("SelectCoordinatorNode: Error resolving node for party %s: %s", party, err)
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

func (d *distributedSequencerManager) eventHandler(event common.Event) {
	log.L(d.ctx).Debugf("Distributed sequencer event: %+v", event)
}

func (d *distributedSequencerManager) DistributeStates(ctx context.Context, stateDistributions []*components.StateDistributionWithData) {
	log.L(d.ctx).Debugf("Distributed sequencer distribute states request")
}

func (d *distributedSequencerManager) GetBlockHeight() int64 {
	return d.blockHeight
}

func (d *distributedSequencerManager) GetNodeName() string {
	return d.nodeName
}

func (d *distributedSequencerManager) startSequencerForContract(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (*distributedSequencer, error) {

	if domainAPI == nil {
		domainAPI, err := d.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
		if err != nil {
			log.L(ctx).Errorf("Failed to get domain smart contract for contract address %s: %s", contractAddr, err)
			return nil, err
		}
		log.L(ctx).Debugf("Domain API retrieved: %s", domainAPI.Domain().Name())
	}

	readlock := true
	d.sequencersLock.RLock()
	defer func() {
		if readlock {
			d.sequencersLock.RUnlock()
		}
	}()
	if d.sequencers[contractAddr.String()] == nil {
		//swap the read lock for a write lock
		d.sequencersLock.RUnlock()
		readlock = false
		d.sequencersLock.Lock()
		defer d.sequencersLock.Unlock()
		//double check in case another goroutine has created the sequencer while we were waiting for the write lock
		if d.sequencers[contractAddr.String()] == nil {
			// Are we handing this off to the sequencer now?
			// Locally we store mappings of contract address to sender/coordinator pair

			committee, err := d.getTXCommittee(ctx, tx)
			if err != nil {
				log.L(ctx).Errorf("Failed to get transaction committee for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}
			dCtx := d.components.StateManager().NewDomainContext(d.ctx, domainAPI.Domain(), contractAddr)
			transportWriter := transport.NewTransportWriter(domainAPI.Domain().Name(), &contractAddr, d.nodeName, d.components.TransportManager())
			engineIntegration := common.NewEngineIntegration(d.ctx, d.components, domainAPI, dCtx, d, d)

			sender, err := sender.NewSender(d.ctx, d.nodeName, transportWriter, committee, common.RealClock(), d.eventHandler, engineIntegration, 10, &contractAddr, 15000, 10)
			if err != nil {
				log.L(ctx).Errorf("Failed to create distributed sequencer sender for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			coordinator, err := coordinator.NewCoordinator(d.ctx, transportWriter, committee, common.RealClock(), d.eventHandler, engineIntegration, 60, 60, 10, &contractAddr, 50, 3)
			if err != nil {
				log.L(ctx).Errorf("Failed to create distributed sequencer coordinator for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			sequencer := &distributedSequencer{
				sender:      sender,
				coordinator: coordinator,
			}
			d.sequencers[contractAddr.String()] = sequencer
			// publisher := NewPublisher(d, contractAddr.String())

			// endorsementGatherer, err := p.getEndorsementGathererForContract(ctx, dbTX, contractAddr)
			// if err != nil {
			// 	log.L(ctx).Errorf("Failed to get endorsement gatherer for contract %s: %s", contractAddr.String(), err)
			// 	return nil, err
			// }

			// newSequencer, err := NewSequencer(
			// 	d.ctx,
			// 	d,
			// 	d.nodeName,
			// 	contractAddr,
			// 	&p.config.Sequencer,
			// 	d.components,
			// 	domainAPI,
			// 	endorsementGatherer,
			// 	publisher,
			// 	d.syncPoints,
			// 	d.components.IdentityResolver(),
			// 	transportWriter,
			// 	confutil.DurationMin(p.config.RequestTimeout, 0, *pldconf.PrivateTxManagerDefaults.RequestTimeout),
			// 	d.blockHeight,
			// )
			if err != nil {
				log.L(ctx).Errorf("Failed to create sequencer for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			return sequencer, nil
		}
	}
	return nil, nil
}

// func (d *distributedSequencerManager) GetTxStatus(ctx context.Context, domainAddress string, txID uuid.UUID) (status components.PrivateTxStatus, err error) { // MRW TODO - what type of TX status does this return?
// this returns status that we happen to have in memory at the moment and might be useful for debugging

// d.sequencersLock.RLock()
// defer d.sequencersLock.RUnlock()
// targetSequencer := d.sequencers[domainAddress]
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

func (d *distributedSequencerManager) HandleNewEvent(ctx context.Context, event string) error {
	// d.sequencersLock.RLock()
	// defer d.sequencersLock.RUnlock()
	// targetSequencer := d.sequencers[event.GetContractAddress()]
	// if targetSequencer == nil { // this is an event that belongs to a contract that's not in flight, throw it away and rely on the engine to trigger the action again when the sequencer is wake up. (an enhanced version is to add weight on queueing an sequencer)
	// 	log.L(ctx).Warnf("Ignored %T event for domain contract %s and transaction %s . If this happens a lot, check the sequencer idle timeout is set to a reasonable number", event, event.GetContractAddress(), event.GetTransactionID())
	// } else {
	// 	targetSequencer.HandleEvent(ctx, event)
	// }
	return nil
}

func (d *distributedSequencerManager) CallPrivateSmartContract(ctx context.Context, call *components.ResolvedTransaction) (*abi.ComponentValue, error) {

	callTx := call.Transaction
	psc, err := d.components.DomainManager().GetSmartContractByAddress(ctx, d.components.Persistence().NOTX(), *callTx.To)
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
	identityResolver := d.components.IdentityResolver()
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
	dCtx := d.components.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
	defer dCtx.Close()

	// Do the actual call
	return psc.ExecCall(dCtx, d.components.Persistence().NOTX(), call, verifiers)
}

func (d *distributedSequencerManager) handleCoordinatorHeartbeatNotification(ctx context.Context, messagePayload []byte) {
	log.L(d.ctx).Debug("Distributed sequencer handleCoordinatorHeartbeatNotification")

	heartbeatNotification := &pbEngine.CoordinatorHeartbeatNotification{}
	err := proto.Unmarshal(messagePayload, heartbeatNotification)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal heartbeatNotification: %s", err)
		return
	}

	from := heartbeatNotification.From
	if from == "" {
		log.L(ctx).Errorf("Failed to handle coordinator heartbeat - from field not set")
		return
	}

	contractAddressString := heartbeatNotification.ContractAddress
	contractAddress, err := pldtypes.ParseEthAddress(contractAddressString)
	if err != nil {
		log.L(ctx).Errorf("Failed to parse contract address from coordinator heartbeat: %s", err)
		return
	}

	coordinatorSnapshot := &common.CoordinatorSnapshot{}
	err = json.Unmarshal(heartbeatNotification.CoordinatorSnapshot, coordinatorSnapshot)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal coordinatorSnapshot: %s", err)
		return
	}

	heartbeatEvent := &sender.HeartbeatReceivedEvent{}
	heartbeatEvent.From = from
	heartbeatEvent.ContractAddress = contractAddress
	heartbeatEvent.CoordinatorSnapshot = *coordinatorSnapshot

	fmt.Println(heartbeatEvent)

	// _, err = d.getSequencerForContract(ctx, p.components.Persistence().NOTX(), *contractAddress, nil, nil)
	// if err != nil {
	// 	log.L(ctx).Errorf("failed to obtain sequencer to pass heartbeat eventct %v:", err)
	// 	return
	// }

	// MRW TODO - does event handling exist here?
	// seq.coordinatorSelector.HandleCoordinatorEvent(ctx, heartbeatEvent)
}
