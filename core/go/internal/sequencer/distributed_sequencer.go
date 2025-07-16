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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/coordinator"
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
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
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
func (d *distributedSequencerManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	log.L(d.ctx).Info("PreInit distributed sequencer manager")
	d.metrics = metrics.InitMetrics(d.ctx, c.MetricsManager().Registry())

	return &components.ManagerInitResult{
		PreCommitHandler: func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
			log.L(ctx).Debug("SequencerMgr PreCommitHandler")
			latestBlockNumber := blocks[len(blocks)-1].Number
			dbTX.AddPostCommit(func(ctx context.Context) {
				log.L(ctx).Debugf("SequencerMgr PostCommitHandler: %d", latestBlockNumber)
				d.OnNewBlockHeight(ctx, latestBlockNumber)
			})
			return nil
		},
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

func NewDistributedSequencerManager(ctx context.Context, config *pldconf.SequencerManagerConfig) components.SequencerManager {
	d := &distributedSequencerManager{
		config:     config,
		sequencers: make(map[string]*distributedSequencer),
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

	return nil
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

func (d *distributedSequencerManager) ProcessPrivateTransactionConfirmed(ctx context.Context, confirmedTxn *components.TxCompletion) {
	log.L(d.ctx).Infof("Distributed sequencer manager handling confirmed transaction %s", confirmedTxn.TransactionID)

	contractAddress := pldtypes.EthAddress{}
	if confirmedTxn.PSC != nil {
		contractAddress = confirmedTxn.PSC.Address()
	}

	seq, err := d.getSequencerForContract(ctx, d.components.Persistence().NOTX(), contractAddress, nil, nil)
	if err != nil {
		// MRW TODO - deploys happen without a dedicated sequencer, so this isn't a hard error. We ought
		// to validate if the transaction being confirmed is a deploy, but leaving as-is for now.
		log.L(ctx).Warnf("failed to obtain sequencer to pass transaction confirmed event to %v:", err)
		return
	}

	log.L(ctx).Infof("Handing off TX confirmed event to distributed sequencer coordinator")

	confirmedEvent := &coordinator.TransactionConfirmedEvent{
		From:         confirmedTxn.OnChain.Source,
		Hash:         confirmedTxn.ReceiptInput.OnChain.TransactionHash,
		RevertReason: confirmedTxn.ReceiptInput.RevertData,
	}

	seq.coordinator.HandleEvent(ctx, confirmedEvent)
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

	sequencer, err := d.getSequencerForContract(ctx, dbTX, contractAddr, domainAPI, tx)
	if err != nil {
		return err
	}

	if tx.PostAssembly == nil {
		// MRW TODO - adopted from private TX mgr behaviour. Is this suitable for distributed sequencer?
		//if we don't know the candidate nodes, and the transaction hasn't been assembled yet, then we can't select a coordinator so just assume we are the coordinator
		// until we get the transaction assembled and then re-evaluate
		log.L(ctx).Debug("DistributedSequencerManager: handleNewTx assembly not yet completed - using local node for assembly")
		sequencer.sender.SetActiveCoordinator(ctx, d.nodeName)
	}

	// txList := []*components.PrivateTransaction{tx}

	// MRW TODO - determine our sender's identity?
	// senderIdentity, _ := d.getTXCommittee(ctx, tx)

	// log.L(ctx).Debugf("DistributedSequencerManager: delegating TX %s to ourselves %s", tx.ID, senderIdentity[0])
	// txDelegatedEvent := &coordinator.TransactionsDelegatedEvent{
	// 	Sender:             senderIdentity[0],
	// 	Transactions:       txList,
	// 	SendersBlockHeight: uint64(d.blockHeight),
	// }
	txCreatedEvent := &sender.TransactionCreatedEvent{
		Transaction: tx,
	}

	sequencer.sender.HandleEvent(ctx, txCreatedEvent)

	return nil
}

func (d *distributedSequencerManager) getTXCommittee(ctx context.Context, tx *components.PrivateTransaction) ([]string, error) {
	candidateNodesMap := make(map[string]struct{})

	if tx.PostAssembly == nil || len(tx.PostAssembly.AttestationPlan) == 0 {
		identities := make([]string, 0, 1)
		identities = append(identities, "member@node1")
		return identities, nil
	}

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

func (d *distributedSequencerManager) DistributeStates(ctx context.Context, stateDistributions []*components.StateDistributionWithData) {
	log.L(d.ctx).Debugf("Distributed sequencer distribute states request")
}

func (d *distributedSequencerManager) GetBlockHeight() int64 {
	return d.blockHeight
}

func (d *distributedSequencerManager) GetNodeName() string {
	return d.nodeName
}

func (d *distributedSequencerManager) getSequencerForContract(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (*distributedSequencer, error) {
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
	log.L(d.ctx).Debug("Distributed sequencer manager HandleNewEvent (currently not implemented)")
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

func (p *distributedSequencerManager) BuildNullifiers(ctx context.Context, stateDistributions []*components.StateDistributionWithData) (nullifiers []*components.NullifierUpsert, err error) {

	nullifiers = []*components.NullifierUpsert{}
	err = p.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for _, s := range stateDistributions {
			if s.NullifierAlgorithm == nil || s.NullifierVerifierType == nil || s.NullifierPayloadType == nil {
				log.L(ctx).Debugf("No nullifier required for state %s on node %s", s.StateID, p.nodeName)
				continue
			}

			nullifier, err := p.BuildNullifier(ctx, p.components.KeyManager().KeyResolverForDBTX(dbTX), s)
			if err != nil {
				return err
			}

			nullifiers = append(nullifiers, nullifier)
		}
		return nil
	})
	return nullifiers, err
}

func (p *distributedSequencerManager) BuildNullifier(ctx context.Context, kr components.KeyResolver, s *components.StateDistributionWithData) (*components.NullifierUpsert, error) {
	// We need to call the signing engine with the local identity to build the nullifier
	log.L(ctx).Infof("Generating nullifier for state %s on node %s (algorithm=%s,verifierType=%s,payloadType=%s)",
		s.StateID, p.nodeName, *s.NullifierAlgorithm, *s.NullifierVerifierType, *s.NullifierPayloadType)

	// We require a fully qualified identifier for the local node in this function
	identifier, node, err := pldtypes.PrivateIdentityLocator(s.IdentityLocator).Validate(ctx, "", false)
	if err != nil || node != p.nodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierNotLocal)
	}

	// Call the signing engine to build the nullifier
	var nulliferBytes []byte
	mapping, err := kr.ResolveKey(ctx, identifier, *s.NullifierAlgorithm, *s.NullifierVerifierType)
	if err == nil {
		nulliferBytes, err = p.components.KeyManager().Sign(ctx, mapping, *s.NullifierPayloadType, s.StateData.Bytes())
	}
	if err != nil || len(nulliferBytes) == 0 {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierFail, s.StateID)
	}
	return &components.NullifierUpsert{
		ID:    nulliferBytes,
		State: pldtypes.MustParseHexBytes(s.StateID),
	}, nil
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
