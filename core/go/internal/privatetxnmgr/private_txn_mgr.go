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

package privatetxnmgr

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/syncpoints"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	pbEngine "github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/proto/engine"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

type privateTxManager struct {
	ctx                  context.Context
	ctxCancel            func()
	config               *pldconf.PrivateTxManagerConfig
	sequencers           map[string]*Sequencer
	sequencersLock       sync.RWMutex
	endorsementGatherers map[string]ptmgrtypes.EndorsementGatherer
	components           components.AllComponents
	nodeName             string
	subscribers          []components.PrivateTxEventSubscriber
	subscribersLock      sync.Mutex
	syncPoints           syncpoints.SyncPoints
	blockHeight          int64
}

// Init implements Engine.
func (p *privateTxManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	return &components.ManagerInitResult{
		PreCommitHandler: func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
			log.L(ctx).Debug("PrivateTxManager PreCommitHandler")
			latestBlockNumber := blocks[len(blocks)-1].Number
			dbTX.AddPostCommit(func(ctx context.Context) {
				log.L(ctx).Debugf("PrivateTxManager PostCommitHandler: %d", latestBlockNumber)
				p.OnNewBlockHeight(ctx, latestBlockNumber)
			})
			return nil
		},
	}, nil
}

func (p *privateTxManager) PostInit(c components.AllComponents) error {
	p.components = c
	p.nodeName = p.components.TransportManager().LocalNodeName()
	p.syncPoints = syncpoints.NewSyncPoints(p.ctx, &p.config.Writer, c.Persistence(), c.TxManager(), c.PublicTxManager(), c.TransportManager())
	return nil
}

func (p *privateTxManager) Start() error {
	p.syncPoints.Start()
	return nil
}

func (p *privateTxManager) Stop() {
}

func NewPrivateTransactionMgr(ctx context.Context, config *pldconf.PrivateTxManagerConfig) components.PrivateTxManager {
	p := &privateTxManager{
		config:               config,
		sequencers:           make(map[string]*Sequencer),
		endorsementGatherers: make(map[string]ptmgrtypes.EndorsementGatherer),
		subscribers:          make([]components.PrivateTxEventSubscriber, 0),
	}
	p.ctx, p.ctxCancel = context.WithCancel(ctx)
	return p
}

func (p *privateTxManager) OnNewBlockHeight(ctx context.Context, blockHeight int64) {
	p.blockHeight = blockHeight

	p.sequencersLock.RLock()
	defer p.sequencersLock.RUnlock()
	for _, sequencer := range p.sequencers {
		sequencer.OnNewBlockHeight(ctx, blockHeight)
	}
}

func (p *privateTxManager) getSequencerForContract(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract) (oc *Sequencer, err error) {

	if domainAPI == nil {
		domainAPI, err = p.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
		if err != nil {
			log.L(ctx).Errorf("Failed to get domain smart contract for contract address %s: %s", contractAddr, err)
			return nil, err
		}
	}

	readlock := true
	p.sequencersLock.RLock()
	defer func() {
		if readlock {
			p.sequencersLock.RUnlock()
		}
	}()
	if p.sequencers[contractAddr.String()] == nil {
		//swap the read lock for a write lock
		p.sequencersLock.RUnlock()
		readlock = false
		p.sequencersLock.Lock()
		defer p.sequencersLock.Unlock()
		//double check in case another goroutine has created the sequencer while we were waiting for the write lock
		if p.sequencers[contractAddr.String()] == nil {
			transportWriter := NewTransportWriter(domainAPI.Domain().Name(), &contractAddr, p.nodeName, p.components.TransportManager())
			publisher := NewPublisher(p, contractAddr.String())

			endorsementGatherer, err := p.getEndorsementGathererForContract(ctx, dbTX, contractAddr)
			if err != nil {
				log.L(ctx).Errorf("Failed to get endorsement gatherer for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			newSequencer, err := NewSequencer(
				p.ctx,
				p,
				p.nodeName,
				contractAddr,
				&p.config.Sequencer,
				p.components,
				domainAPI,
				endorsementGatherer,
				publisher,
				p.syncPoints,
				p.components.IdentityResolver(),
				transportWriter,
				confutil.DurationMin(p.config.RequestTimeout, 0, *pldconf.PrivateTxManagerDefaults.RequestTimeout),
				p.blockHeight,
			)
			if err != nil {
				log.L(ctx).Errorf("Failed to create sequencer for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}
			p.sequencers[contractAddr.String()] = newSequencer

			sequencerDone, err := p.sequencers[contractAddr.String()].Start(ctx)
			if err != nil {
				log.L(ctx).Errorf("Failed to start sequencer for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			go func() {

				<-sequencerDone
				log.L(ctx).Infof("Sequencer for contract %s has stopped", contractAddr.String())
				p.sequencersLock.Lock()
				defer p.sequencersLock.Unlock()
				delete(p.sequencers, contractAddr.String())
			}()
		}
	}
	return p.sequencers[contractAddr.String()], nil
}

func (p *privateTxManager) getEndorsementGathererForContract(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress) (ptmgrtypes.EndorsementGatherer, error) {
	// We need to have this as a function of the PrivateTransactionManager rather than a function of the sequencer because the endorsement gatherer is needed
	// even if we don't have a sequencer.  e.g. maybe the transaction is being coordinated by another node and this node has just been asked to endorse it
	// in that case, we need to make sure that we are using the domainContext provided by the endorsement request
	domainSmartContract, err := p.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
	if err != nil {
		return nil, err
	}
	if p.endorsementGatherers[contractAddr.String()] == nil {
		// TODO: Consider scope of state in privateTxManager threading model
		dCtx := p.components.StateManager().NewDomainContext(p.ctx /* background context */, domainSmartContract.Domain(), contractAddr)
		endorsementGatherer := NewEndorsementGatherer(p.components.Persistence(), domainSmartContract, dCtx, p.components.KeyManager())
		p.endorsementGatherers[contractAddr.String()] = endorsementGatherer
	}
	return p.endorsementGatherers[contractAddr.String()], nil
}

func (p *privateTxManager) HandleNewTx(ctx context.Context, dbTX persistence.DBTX, txi *components.ValidatedTransaction) error {
	tx := txi.Transaction
	if tx.To == nil {
		if txi.Transaction.SubmitMode.V() != pldapi.SubmitModeAuto {
			return i18n.NewError(ctx, msgs.MsgPrivateTxMgrPrepareNotSupportedDeploy)
		}
		return p.handleDeployTx(ctx, &components.PrivateContractDeploy{
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
	return p.handleNewTx(ctx, dbTX, &components.PrivateTransaction{
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
func (p *privateTxManager) handleNewTx(ctx context.Context, dbTX persistence.DBTX, tx *components.PrivateTransaction, localTx *components.ResolvedTransaction) error {
	log.L(ctx).Debugf("Handling new transaction: %v", tx)

	contractAddr := *localTx.Transaction.To
	emptyAddress := pldtypes.EthAddress{}
	if contractAddr == emptyAddress {
		return i18n.NewError(ctx, msgs.MsgContractAddressNotProvided)
	}

	domainAPI, err := p.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
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

	oc, err := p.getSequencerForContract(ctx, dbTX, contractAddr, domainAPI)
	if err != nil {
		return err
	}
	queued := oc.ProcessNewTransaction(ctx, tx)
	if queued {
		log.L(ctx).Debugf("Transaction with ID %s queued in database", tx.ID)
	}
	return nil
}

func (p *privateTxManager) validateDelegatedTransaction(ctx context.Context, tx *components.PrivateTransaction) error {
	log.L(ctx).Debugf("Validating delegated transaction: %v", tx)
	if tx.Domain == "" {
		return i18n.NewError(ctx, msgs.MsgDomainNotProvided)
	}

	emptyAddress := pldtypes.EthAddress{}
	if tx.Address == emptyAddress {
		return i18n.NewError(ctx, msgs.MsgContractAddressNotProvided)
	}

	if tx.PreAssembly == nil {
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "PreAssembly is nil")
	}
	return nil

}

func (p *privateTxManager) handleDelegatedTransaction(ctx context.Context, dbTX persistence.DBTX, delegationBlockHeight int64, delegatingNodeName string, delegationId string, tx *components.PrivateTransaction) error {
	log.L(ctx).Debugf("Handling delegated transaction: %v", tx)

	domainAPI, err := p.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, tx.Address)
	if err != nil {
		log.L(ctx).Errorf("handleDelegatedTransaction: Failed to get domain smart contract for contract address %s: %s", tx.Address, err)
		return err
	}
	sequencer, err := p.getSequencerForContract(ctx, dbTX, tx.Address, domainAPI)
	if err != nil {
		return err
	}
	queued := sequencer.ProcessInFlightTransaction(ctx, tx, &delegationBlockHeight)
	if queued {
		log.L(ctx).Debugf("Delegated Transaction with ID %s queued in database", tx.ID)
	}
	err = sequencer.transportWriter.SendDelegationRequestAcknowledgment(ctx, delegatingNodeName, delegationId, p.nodeName, tx.ID.String())
	if err != nil {
		log.L(ctx).Errorf("Failed to send delegation request acknowledgment: %s", err)
		// if we can't send the acknowledgment, the sender will retry
	}
	return nil
}

// Synchronous function to submit a deployment request which is asynchronously processed
// Private transaction manager will receive a notification when the public transaction is confirmed
// (same as for invokes)
func (p *privateTxManager) handleDeployTx(ctx context.Context, tx *components.PrivateContractDeploy) error {
	log.L(ctx).Debugf("Handling new private contract deploy transaction: %v", tx)
	if tx.Domain == "" {
		return i18n.NewError(ctx, msgs.MsgDomainNotProvided)
	}

	domain, err := p.components.DomainManager().GetDomainByName(ctx, tx.Domain)
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
	go p.deploymentLoop(log.WithLogField(p.ctx, "role", "deploy-loop"), domain, tx)

	return nil
}

func (p *privateTxManager) deploymentLoop(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) {
	log.L(ctx).Info("Starting deployment loop")

	var err error

	// Resolve keys synchronously on this go routine so that we can return an error if any key resolution fails
	tx.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.RequiredVerifiers))
	for i, v := range tx.RequiredVerifiers {
		// TODO: This is a synchronous cross-node exchange, done sequentially for each verifier.
		// Potentially needs to move to an event-driven model like on invocation.
		verifier, resolveErr := p.components.IdentityResolver().ResolveVerifier(ctx, v.Lookup, v.Algorithm, v.VerifierType)
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
		err = p.evaluateDeployment(ctx, domain, tx)
	}
	if err != nil {
		log.L(ctx).Errorf("Error evaluating deployment: %s", err)
		return
	}
	log.L(ctx).Info("Deployment completed successfully. ")
}

func (p *privateTxManager) revertDeploy(ctx context.Context, tx *components.PrivateContractDeploy, err error) error {
	deployError := i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerDeployError)

	var tryFinalize func()
	tryFinalize = func() {
		p.syncPoints.QueueTransactionFinalize(ctx, tx.Domain, pldtypes.EthAddress{}, tx.From, tx.ID, deployError.Error(),
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

func (p *privateTxManager) evaluateDeployment(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) error {

	// TODO there is a lot of common code between this and the Dispatch function in the sequencer. should really move some of it into a common place
	// and use that as an opportunity to refactor to be more readable

	err := domain.PrepareDeploy(ctx, tx)
	if err != nil {
		return p.revertDeploy(ctx, tx, err)
	}

	publicTransactionEngine := p.components.PublicTxManager()

	// The signer needs to be in our local node or it's an error
	identifier, node, err := pldtypes.PrivateIdentityLocator(tx.Signer).Validate(ctx, p.nodeName, true)
	if err != nil {
		return err
	}
	if node != p.nodeName {
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerNonLocalSigningAddr, tx.Signer)
	}

	keyMgr := p.components.KeyManager()
	resolvedAddrs, err := keyMgr.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
	if err != nil {
		return p.revertDeploy(ctx, tx, err)
	}

	publicTXs := []*components.PublicTxSubmission{
		{
			Bindings: []*components.PaladinTXReference{
				{
					TransactionID:              tx.ID,
					TransactionType:            pldapi.TransactionTypePrivate.Enum(),
					TransactionSender:          tx.From,
					TransactionContractAddress: "", // no contract address for deployment transactions
				},
			},
			PublicTxInput: pldapi.PublicTxInput{
				From:            resolvedAddrs[0],
				PublicTxOptions: pldapi.PublicTxOptions{}, // TODO: Consider propagation from paladin transaction input
			},
		},
	}

	if tx.InvokeTransaction != nil {
		log.L(ctx).Debug("Deploying by invoking a base ledger contract")

		data, err := tx.InvokeTransaction.FunctionABI.EncodeCallDataCtx(ctx, tx.InvokeTransaction.Inputs)
		if err != nil {
			return p.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrEncodeCallDataFailed))
		}
		publicTXs[0].Data = pldtypes.HexBytes(data)
		publicTXs[0].To = &tx.InvokeTransaction.To

	} else if tx.DeployTransaction != nil {
		//TODO
		return p.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "DeployTransaction not implemented"))
	} else {
		return p.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "Neither InvokeTransaction nor DeployTransaction set"))
	}

	for _, pubTx := range publicTXs {
		err := publicTransactionEngine.ValidateTransaction(ctx, p.components.Persistence().NOTX(), pubTx)
		if err != nil {
			return p.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, "PrepareSubmissionBatch failed"))
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
	err = p.syncPoints.PersistDeployDispatchBatch(ctx, dispatchBatch)
	if err != nil {
		log.L(ctx).Errorf("Error persisting batch: %s", err)
		return p.revertDeploy(ctx, tx, err)
	}

	p.publishToSubscribers(ctx, &components.TransactionDispatchedEvent{
		TransactionID:  tx.ID.String(),
		Nonce:          uint64(0), /*TODO*/
		SigningAddress: tx.Signer,
	})

	return nil

}

func (p *privateTxManager) GetTxStatus(ctx context.Context, domainAddress string, txID uuid.UUID) (status components.PrivateTxStatus, err error) {
	// this returns status that we happen to have in memory at the moment and might be useful for debugging

	p.sequencersLock.RLock()
	defer p.sequencersLock.RUnlock()
	targetSequencer := p.sequencers[domainAddress]
	if targetSequencer == nil {
		return components.PrivateTxStatus{
			TxID:   txID.String(),
			Status: "unknown",
		}, nil

	} else {
		return targetSequencer.GetTxStatus(ctx, txID)
	}

}

func (p *privateTxManager) getSequencerIfActive(ctx context.Context, domainAddress string) *Sequencer {
	p.sequencersLock.RLock()
	defer p.sequencersLock.RUnlock()
	return p.sequencers[domainAddress]
}

func (p *privateTxManager) HandleNewEvent(ctx context.Context, event ptmgrtypes.PrivateTransactionEvent) {
	p.sequencersLock.RLock()
	defer p.sequencersLock.RUnlock()
	targetSequencer := p.sequencers[event.GetContractAddress()]
	if targetSequencer == nil { // this is an event that belongs to a contract that's not in flight, throw it away and rely on the engine to trigger the action again when the sequencer is wake up. (an enhanced version is to add weight on queueing an sequencer)
		log.L(ctx).Warnf("Ignored %T event for domain contract %s and transaction %s . If this happens a lot, check the sequencer idle timeout is set to a reasonable number", event, event.GetContractAddress(), event.GetTransactionID())
	} else {
		targetSequencer.HandleEvent(ctx, event)
	}
}

func (p *privateTxManager) handleEndorsementRequest(ctx context.Context, messagePayload []byte, replyTo string) {
	endorsementRequest := &pbEngine.EndorsementRequest{}
	err := proto.Unmarshal(messagePayload, endorsementRequest)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal endorsement request: %s", err)
		return
	}
	contractAddressString := endorsementRequest.ContractAddress
	contractAddress, err := pldtypes.ParseEthAddress(contractAddressString)
	if err != nil {
		log.L(ctx).Errorf("Failed to parse contract address %s: %s", contractAddressString, err)
		return
	}

	endorsementGatherer, err := p.getEndorsementGathererForContract(ctx, p.components.Persistence().NOTX(), *contractAddress)
	if err != nil {
		log.L(ctx).Errorf("Failed to get endorsement gatherer for contract address %s: %s", contractAddressString, err)
		return
	}

	//TODO the following is temporary code to unmarshal the fields of the endorsement request
	// what we really should be doing is importing the tkproto messages but need to figure out the build
	// magic to make that work

	transactionSpecificationAny := endorsementRequest.GetTransactionSpecification()
	transactionSpecification := &prototk.TransactionSpecification{}
	err = transactionSpecificationAny.UnmarshalTo(transactionSpecification)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal transaction specification: %s", err)
		return
	}

	attestationRequestAny := endorsementRequest.GetAttestationRequest()
	attestationRequest := &prototk.AttestationRequest{}
	err = attestationRequestAny.UnmarshalTo(attestationRequest)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal attestation request: %s", err)
		return
	}

	verifiersAny := endorsementRequest.GetVerifiers()
	verifiers := make([]*prototk.ResolvedVerifier, len(verifiersAny))
	for i, v := range verifiersAny {
		verifiers[i] = &prototk.ResolvedVerifier{}
		err = v.UnmarshalTo(verifiers[i])
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal attestation request: %s", err)
			return
		}
	}

	signatures := make([]*prototk.AttestationResult, len(endorsementRequest.GetSignatures()))
	for i, s := range endorsementRequest.GetSignatures() {
		signatures[i] = &prototk.AttestationResult{}
		err = s.UnmarshalTo(signatures[i])
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal attestation request: %s", err)
			return
		}
	}

	inputStates := make([]*prototk.EndorsableState, len(endorsementRequest.GetInputStates()))
	for i, s := range endorsementRequest.GetInputStates() {
		inputStates[i] = &prototk.EndorsableState{}
		err = s.UnmarshalTo(inputStates[i])
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal attestation request: %s", err)
			return
		}
	}

	readStates := make([]*prototk.EndorsableState, len(endorsementRequest.GetReadStates()))
	for i, s := range endorsementRequest.GetReadStates() {
		readStates[i] = &prototk.EndorsableState{}
		err = s.UnmarshalTo(readStates[i])
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal attestation request: %s", err)
			return
		}
	}

	outputStates := make([]*prototk.EndorsableState, len(endorsementRequest.GetOutputStates()))
	for i, s := range endorsementRequest.GetOutputStates() {
		outputStates[i] = &prototk.EndorsableState{}
		err = s.UnmarshalTo(outputStates[i])
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal attestation request: %s", err)
			return
		}
	}

	infoStates := make([]*prototk.EndorsableState, len(endorsementRequest.GetInfoStates()))
	for i, s := range endorsementRequest.GetInfoStates() {
		infoStates[i] = &prototk.EndorsableState{}
		err = s.UnmarshalTo(infoStates[i])
		if err != nil {
			log.L(ctx).Errorf("Failed to unmarshal attestation request: %s", err)
			return
		}
	}

	endorsement, revertReason, err := endorsementGatherer.GatherEndorsement(ctx,
		transactionSpecification,
		verifiers,
		signatures,
		inputStates,
		readStates,
		outputStates,
		infoStates,
		endorsementRequest.GetParty(),
		attestationRequest)
	if err != nil {
		log.L(ctx).Errorf("Failed to gather endorsement: %s", err)
		return
	}

	endorsementAny, err := anypb.New(endorsement)
	if err != nil {
		log.L(ctx).Errorf("Failed marshal endorsement: %s", err)
		return
	}

	endorsementResponse := &pbEngine.EndorsementResponse{
		IdempotencyKey:         endorsementRequest.IdempotencyKey,
		ContractAddress:        contractAddressString,
		TransactionId:          endorsementRequest.TransactionId,
		Endorsement:            endorsementAny,
		RevertReason:           revertReason,
		Party:                  endorsementRequest.Party,
		AttestationRequestName: attestationRequest.Name,
	}
	endorsementResponseBytes, err := proto.Marshal(endorsementResponse)
	if err != nil {
		log.L(ctx).Errorf("Failed to marshal endorsement response: %s", err)
		return
	}

	err = p.components.TransportManager().Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "EndorsementResponse",
		Payload:     endorsementResponseBytes,
		Node:        replyTo,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
	})
	if err != nil {
		log.L(ctx).Errorf("Failed to send endorsement response: %s", err)
		return
	}
}

func (p *privateTxManager) handleDelegationRequest(ctx context.Context, messagePayload []byte, replyTo string) {
	delegationRequest := &pbEngine.DelegationRequest{}
	err := proto.Unmarshal(messagePayload, delegationRequest)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal delegation request: %s", err)
		return
	}

	transaction := new(components.PrivateTransaction)
	err = json.Unmarshal(delegationRequest.PrivateTransaction, &transaction)

	//before persisting the transaction, we validate it and send a rejection message if it is invalid
	if err == nil {
		err = p.validateDelegatedTransaction(ctx, transaction)
	}
	if err != nil {
		log.L(ctx).Errorf("Failed to validate delegated transaction: %s", err)
		//TODO send a negative acknowledgement
		return
	}

	//TODO not quite figured out how to receive an assembled transaction because it will have been assembled
	// in the domain context of the sender.  In some cases, it will be using committed states so that will be ok.
	// for now, in the interest of simplicity, we just trash the PostAssembly and start again
	transaction.PostAssembly = nil
	err = p.handleDelegatedTransaction(ctx, p.components.Persistence().NOTX(), delegationRequest.BlockHeight, replyTo, delegationRequest.DelegationId, transaction)
	if err != nil {
		log.L(ctx).Errorf("Failed to handle delegated transaction: %s", err)
		// do not send an ack and let the sender retry
		return
	}
}

func (p *privateTxManager) handleDelegationRequestAcknowledgment(ctx context.Context, messagePayload []byte) {
	delegationRequestAcknowledgment := &pbEngine.DelegationRequestAcknowledgment{}
	err := proto.Unmarshal(messagePayload, delegationRequestAcknowledgment)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal delegation request acknowledgment: %s", err)
		return
	}

	p.HandleNewEvent(ctx, &ptmgrtypes.TransactionDelegationAcknowledgedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   delegationRequestAcknowledgment.TransactionId,
			ContractAddress: delegationRequestAcknowledgment.ContractAddress,
		},
		DelegationRequestID: delegationRequestAcknowledgment.DelegationId,
	})

}

func (p *privateTxManager) handleEndorsementResponse(ctx context.Context, messagePayload []byte) {

	endorsementResponse := &pbEngine.EndorsementResponse{}
	err := proto.Unmarshal(messagePayload, endorsementResponse)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal endorsementResponse: %s", err)
		return
	}
	contractAddressString := endorsementResponse.ContractAddress

	var revertReason *string
	if endorsementResponse.GetRevertReason() != "" {
		revertReason = confutil.P(endorsementResponse.GetRevertReason())
	}
	endorsement := &prototk.AttestationResult{}
	err = endorsementResponse.GetEndorsement().UnmarshalTo(endorsement)
	if err != nil {
		// TODO this is only temporary until we stop using anypb in EndorsementResponse
		log.L(ctx).Errorf("Wrong type received in EndorsementResponse")
		return
	}

	p.HandleNewEvent(ctx, &ptmgrtypes.TransactionEndorsedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   endorsementResponse.TransactionId,
			ContractAddress: contractAddressString,
		},
		RevertReason:           revertReason,
		Endorsement:            endorsement,
		Party:                  endorsementResponse.Party,
		AttestationRequestName: endorsementResponse.AttestationRequestName,
		IdempotencyKey:         endorsementResponse.IdempotencyKey,
	})

}

func (p *privateTxManager) sendAssembleError(ctx context.Context, node string, assembleRequestId string, contractAddress string, transactionID string, err error) {

	assembleError := &pbEngine.AssembleError{
		ContractAddress:   contractAddress,
		AssembleRequestId: assembleRequestId,
		TransactionId:     transactionID,
		ErrorMessage:      err.Error(),
	}
	assembleErrorBytes, err := proto.Marshal(assembleError)
	if err != nil {
		log.L(ctx).Errorf("Failed to marshal assemble error: %s", err)
		return
	}

	log.L(ctx).Infof("Sending Assemble Error: ContractAddress: %s, TransactionId: %s, AssembleRequestId %s, Error: %s", contractAddress, transactionID, assembleRequestId, assembleError.ErrorMessage)

	err = p.components.TransportManager().Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "AssembleError",
		Payload:     assembleErrorBytes,
		Node:        node,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
	})
	if err != nil {
		log.L(ctx).Errorf("Failed to send  assemble error: %s", err)
		return
	}
}

func (p *privateTxManager) handleAssembleRequest(ctx context.Context, messagePayload []byte, replyTo string) {

	assembleRequest := &pbEngine.AssembleRequest{}
	err := proto.Unmarshal(messagePayload, assembleRequest)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal assembleRequest: %s", err)
		return
	}

	transactionIDString := assembleRequest.TransactionId
	transactionID, err := uuid.Parse(transactionIDString)
	if err != nil {
		log.L(ctx).Errorf("Failed to parse transaction ID: %s", err)
		return
	}

	contractAddressString := assembleRequest.ContractAddress
	contractAddress, err := pldtypes.ParseEthAddress(contractAddressString)
	if err != nil {
		log.L(ctx).Errorf("Failed to parse contract address: %s", err)
		return
	}

	// now we have enough info from the request, at least to send an error if we can't proceed
	// but until this point any errors result in a silent failure and we assume the coordinator will eventually timeout
	// and retry the request

	preAssembly := &components.TransactionPreAssembly{}
	err = json.Unmarshal(assembleRequest.PreAssembly, preAssembly)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal preAssembly: %s", err)
		p.sendAssembleError(ctx, replyTo, assembleRequest.AssembleRequestId, assembleRequest.ContractAddress, assembleRequest.TransactionId, err)
		return
	}

	sequencer, err := p.getSequencerForContract(ctx, p.components.Persistence().NOTX(), *contractAddress, nil) // this is just to make sure the sequencer is running
	if err != nil {
		log.L(ctx).Errorf("Failed to get sequencer for contract address %s: %s", contractAddressString, err)
		p.sendAssembleError(ctx, replyTo, assembleRequest.AssembleRequestId, assembleRequest.ContractAddress, assembleRequest.TransactionId, err)
		return
	}

	postAssembly, err := sequencer.assembleForRemoteCoordinator(ctx, transactionID, preAssembly, assembleRequest.StateLocks, assembleRequest.BlockHeight)
	if err != nil {
		log.L(ctx).Errorf("Failed to assemble for coordinator: %s", err)
		p.sendAssembleError(ctx, replyTo, assembleRequest.AssembleRequestId, assembleRequest.ContractAddress, assembleRequest.TransactionId, err)
		return
	}

	postAssemblyBytes, err := json.Marshal(postAssembly)
	if err != nil {
		log.L(ctx).Errorf("Failed to marshal post assembly: %s", err)
		p.sendAssembleError(ctx, replyTo, assembleRequest.AssembleRequestId, assembleRequest.ContractAddress, assembleRequest.TransactionId, err)
		return
	}

	//Send success assemble response.  This is a best can do effort, and no attempt to make the response delivery reliable
	// in worst case scenario, the coordinator will time out and retry the request

	assembleResponse := &pbEngine.AssembleResponse{
		ContractAddress:   contractAddressString,
		AssembleRequestId: assembleRequest.AssembleRequestId,
		TransactionId:     transactionIDString,
		PostAssembly:      postAssemblyBytes,
	}
	assembleResponseBytes, err := proto.Marshal(assembleResponse)
	if err != nil {
		log.L(ctx).Errorf("Failed to marshal assemble response: %s", err)
		return
	}

	err = p.components.TransportManager().Send(ctx, &components.FireAndForgetMessageSend{
		MessageType: "AssembleResponse",
		Payload:     assembleResponseBytes,
		Node:        replyTo,
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
	})
	if err != nil {
		log.L(ctx).Errorf("Failed to send assemble response: %s", err)
		//Try to send an error to at least free up the coordinator but it is very possible the error fails to send for the same reason
		// and we will need to rely on timeout and retry on the coordinator side
		p.sendAssembleError(ctx, replyTo, assembleRequest.AssembleRequestId, assembleRequest.ContractAddress, assembleRequest.TransactionId, err)
		return
	}

	log.L(ctx).Debug("handleAssembleRequest sent assemble response")

}

func (p *privateTxManager) handleAssembleResponse(ctx context.Context, messagePayload []byte) {
	log.L(ctx).Debug("handleAssembleResponse")
	assembleResponse := &pbEngine.AssembleResponse{}
	err := proto.Unmarshal(messagePayload, assembleResponse)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal assembleResponse: %s", err)
		return
	}
	contractAddressString := assembleResponse.ContractAddress
	transactionIDString := assembleResponse.TransactionId

	postAssemblyJSON := assembleResponse.PostAssembly
	postAssembly := &components.TransactionPostAssembly{}
	err = json.Unmarshal(postAssemblyJSON, postAssembly)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal postAssembly: %s", err)
		//we at least know the transaction ID and contract address so we can communicate
		// this as a failed assemble to let the coordinator know to stop waiting
		p.HandleNewEvent(ctx, &ptmgrtypes.TransactionAssembleFailedEvent{
			PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
				TransactionID:   transactionIDString,
				ContractAddress: contractAddressString,
			},
			Error:             err.Error(),
			AssembleRequestID: assembleResponse.AssembleRequestId,
		})
		return
	}

	p.HandleNewEvent(ctx, &ptmgrtypes.TransactionAssembledEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   transactionIDString,
			ContractAddress: contractAddressString,
		},
		PostAssembly:      postAssembly,
		AssembleRequestID: assembleResponse.AssembleRequestId,
	})
}

func (p *privateTxManager) handleAssembleError(ctx context.Context, messagePayload []byte) {
	assembleError := &pbEngine.AssembleError{}
	err := proto.Unmarshal(messagePayload, assembleError)
	if err != nil {
		log.L(ctx).Errorf("Failed to unmarshal assembleError: %s", err)
		return
	}
	contractAddressString := assembleError.ContractAddress
	transactionIDString := assembleError.TransactionId

	log.L(ctx).Infof("Received Assemble Error: ContractAddress: %s, TransactionId: %s, AssembleRequestId %s, Error: %s", contractAddressString, transactionIDString, assembleError.AssembleRequestId, assembleError.ErrorMessage)

	p.HandleNewEvent(ctx, &ptmgrtypes.TransactionAssembleFailedEvent{
		PrivateTransactionEventBase: ptmgrtypes.PrivateTransactionEventBase{
			TransactionID:   transactionIDString,
			ContractAddress: contractAddressString,
		},
		AssembleRequestID: assembleError.AssembleRequestId,
		Error:             assembleError.ErrorMessage,
	})
}

// For now, this is here to help with testing but it seems like it could be useful thing to have
// in the future if we want to have an eventing interface but at such time we would need to put more effort
// into the reliability of the event delivery or maybe there is only a consumer of the event and it is responsible
// for managing multiple subscribers and durability etc...
func (p *privateTxManager) Subscribe(ctx context.Context, subscriber components.PrivateTxEventSubscriber) {
	p.subscribersLock.Lock()
	defer p.subscribersLock.Unlock()
	//TODO implement this
	p.subscribers = append(p.subscribers, subscriber)
}

func (p *privateTxManager) publishToSubscribers(ctx context.Context, event components.PrivateTxEvent) {
	log.L(ctx).Debugf("Publishing event to subscribers")
	p.subscribersLock.Lock()
	defer p.subscribersLock.Unlock()
	for _, subscriber := range p.subscribers {
		subscriber(event)
	}
}

func (p *privateTxManager) NotifyFailedPublicTx(ctx context.Context, dbTX persistence.DBTX, failures []*components.PublicTxMatch) error {
	// TODO: We have processing we need to do here to resubmit
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
	}
	// Distribute the receipts to the correct location - either local if we were the submitter, or remote.
	return p.WriteOrDistributeReceiptsPostSubmit(ctx, dbTX, privateFailureReceipts)
}

// We get called post-commit by the indexer in the domain when transaction confirmations have been recorded,
// at which point it is important for us to remove transactions from our Domain Context in-memory buffer.
// This might also unblock significant extra processing for more transactions.
func (p *privateTxManager) PrivateTransactionConfirmed(ctx context.Context, receipt *components.TxCompletion) {
	log.L(ctx).Infof("private TX manager notified of transaction confirmation %s deploy=%t",
		receipt.TransactionID, receipt.PSC == nil)
	if receipt.PSC != nil {
		seq, err := p.getSequencerForContract(ctx, p.components.Persistence().NOTX(), receipt.PSC.Address(), receipt.PSC)
		if err != nil {
			log.L(ctx).Errorf("failed to obtain sequence to process receipts on contract %s: %s", receipt.PSC.Address(), err)
			return
		}
		seq.publisher.PublishTransactionConfirmedEvent(ctx, receipt.TransactionID.String())
	}
}

func (p *privateTxManager) CallPrivateSmartContract(ctx context.Context, call *components.ResolvedTransaction) (*abi.ComponentValue, error) {

	callTx := call.Transaction
	psc, err := p.components.DomainManager().GetSmartContractByAddress(ctx, p.components.Persistence().NOTX(), *callTx.To)
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
	identityResolver := p.components.IdentityResolver()
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
	dCtx := p.components.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
	defer dCtx.Close()

	// Do the actual call
	return psc.ExecCall(dCtx, p.components.Persistence().NOTX(), call, verifiers)
}

func (p *privateTxManager) BuildStateDistributions(ctx context.Context, tx *components.PrivateTransaction) (*components.StateDistributionSet, error) {
	return newStateDistributionBuilder(p.components, tx).Build(ctx)
}

func (p *privateTxManager) WriteOrDistributeReceiptsPostSubmit(ctx context.Context, dbTX persistence.DBTX, receipts []*components.ReceiptInputWithOriginator) error {

	// For any failures in a post submission, it basically invalidates the whole working state of our in-memory sequencer.
	// In this version of the engine, we simply unload the whole engine.
	// This is like a restart of the Paladin engine - and means anything in-flight is aborted.
	// New transactions will load a fresh engine.
	// TODO: See https://github.com/LF-Decentralized-Trust-labs/paladin/pull/673 for work on the more comprehensive stateful sequencer.

	for _, r := range receipts {
		if r.ReceiptType != components.RT_Success && r.DomainContractAddress != "" {
			seq := p.getSequencerIfActive(ctx, r.DomainContractAddress)
			if seq != nil {
				log.L(ctx).Errorf("Due to transaction error the sequencer for smart contract %s in domain %s is STOPPING", seq.contractAddress, r.Domain)
				seq.Stop()
			}
		}
	}

	return p.syncPoints.WriteOrDistributeReceipts(ctx, dbTX, receipts)
}
