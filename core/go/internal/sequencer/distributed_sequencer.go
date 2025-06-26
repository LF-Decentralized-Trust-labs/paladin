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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender"

	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/core/pkg/persistence"
	pbEngine "github.com/kaleido-io/paladin/core/pkg/proto/engine"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"google.golang.org/protobuf/proto"

	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type distributedSequencerManager struct {
	ctx        context.Context
	ctxCancel  func()
	config     *pldconf.DistributedSequencerManagerConfig
	components components.AllComponents
	nodeName   string
}

// Init implements Engine.
func (d *distributedSequencerManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
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
	d.components = c
	d.nodeName = d.components.TransportManager().LocalNodeName()
	return nil
}

func (p *distributedSequencerManager) Start() error {
	return nil
}

func (p *distributedSequencerManager) Stop() {
}

func NewDistributedSequencerManager(ctx context.Context, config *pldconf.DistributedSequencerManagerConfig) components.DistributedSequencerManager {
	d := &distributedSequencerManager{
		config: config,
	}
	d.ctx, d.ctxCancel = context.WithCancel(ctx)
	return d
}

func (d *distributedSequencerManager) OnNewBlockHeight(ctx context.Context, blockHeight int64) {
}

//func (d *distributedSequencerManager) getSequencerForContract(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (oc *Sequencer, err error) {
//	return nil, nil
// if domainAPI == nil {
// 	domainAPI, err = p.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
// 	if err != nil {
// 		log.L(ctx).Errorf("Failed to get domain smart contract for contract address %s: %s", contractAddr, err)
// 		return nil, err
// 	}
// }

// readlock := true
// p.sequencersLock.RLock()
// defer func() {
// 	if readlock {
// 		p.sequencersLock.RUnlock()
// 	}
// }()
// if p.sequencers[contractAddr.String()] == nil {
// 	//swap the read lock for a write lock
// 	p.sequencersLock.RUnlock()
// 	readlock = false
// 	p.sequencersLock.Lock()
// 	defer p.sequencersLock.Unlock()
// 	//double check in case another goroutine has created the sequencer while we were waiting for the write lock
// 	if p.sequencers[contractAddr.String()] == nil {
// 		transportWriter := NewTransportWriter(domainAPI.Domain().Name(), &contractAddr, p.nodeName, p.components.TransportManager())
// 		publisher := NewPublisher(p, contractAddr.String())

// 		endorsementGatherer, err := p.getEndorsementGathererForContract(ctx, dbTX, contractAddr)
// 		if err != nil {
// 			log.L(ctx).Errorf("Failed to get endorsement gatherer for contract %s: %s", contractAddr.String(), err)
// 			return nil, err
// 		}

// 		newSequencer, err := NewSequencer(
// 			p.ctx,
// 			p,
// 			p.nodeName,
// 			contractAddr,
// 			&p.config.Sequencer,
// 			p.components,
// 			domainAPI,
// 			endorsementGatherer,
// 			publisher,
// 			p.syncPoints,
// 			p.components.IdentityResolver(),
// 			transportWriter,
// 			confutil.DurationMin(p.config.RequestTimeout, 0, *pldconf.PrivateTxManagerDefaults.RequestTimeout),
// 			p.blockHeight,
// 		)
// 		if err != nil {
// 			log.L(ctx).Errorf("Failed to create sequencer for contract %s: %s", contractAddr.String(), err)
// 			return nil, err
// 		}
// 		p.sequencers[contractAddr.String()] = newSequencer

// 		sequencerDone, err := p.sequencers[contractAddr.String()].Start(ctx)
// 		if err != nil {
// 			log.L(ctx).Errorf("Failed to start sequencer for contract %s: %s", contractAddr.String(), err)
// 			return nil, err
// 		}

// 		go func() {

// 			<-sequencerDone
// 			log.L(ctx).Infof("Sequencer for contract %s has stopped", contractAddr.String())
// 			p.sequencersLock.Lock()
// 			defer p.sequencersLock.Unlock()
// 			delete(p.sequencers, contractAddr.String())
// 		}()
// 	}
// }
// return p.sequencers[contractAddr.String()], nil
// }

func (d *distributedSequencerManager) HandleNewTx(ctx context.Context, dbTX persistence.DBTX, txi *components.ValidatedTransaction) error {
	// tx := txi.Transaction
	// if tx.To == nil {
	// 	if txi.Transaction.SubmitMode.V() != pldapi.SubmitModeAuto {
	// 		return i18n.NewError(ctx, msgs.MsgPrivateTxMgrPrepareNotSupportedDeploy)
	// 	}
	// 	return d.handleDeployTx(ctx, &components.PrivateContractDeploy{
	// 		ID:     *tx.ID,
	// 		Domain: tx.Domain,
	// 		From:   tx.From,
	// 		Inputs: tx.Data,
	// 	})
	// }
	// intent := prototk.TransactionSpecification_SEND_TRANSACTION
	// if txi.Transaction.SubmitMode.V() == pldapi.SubmitModeExternal {
	// 	intent = prototk.TransactionSpecification_PREPARE_TRANSACTION
	// }
	// if txi.Function == nil || txi.Function.Definition == nil {
	// 	return i18n.NewError(ctx, msgs.MsgPrivateTxMgrFunctionNotProvided)
	// }
	// return p.handleNewTx(ctx, dbTX, &components.PrivateTransaction{
	// 	ID:      *tx.ID,
	// 	Domain:  tx.Domain,
	// 	Address: *tx.To,
	// 	Intent:  intent,
	// }, &txi.ResolvedTransaction)
	return nil
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
	// log.L(ctx).Debugf("Handling new transaction: %v", tx)

	// contractAddr := *localTx.Transaction.To
	// emptyAddress := pldtypes.EthAddress{}
	// if contractAddr == emptyAddress {
	// 	return i18n.NewError(ctx, msgs.MsgContractAddressNotProvided)
	// }

	// domainAPI, err := p.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
	// if err != nil {
	// 	return err
	// }

	// domainName := domainAPI.Domain().Name()
	// if localTx.Transaction.Domain != "" && domainName != localTx.Transaction.Domain {
	// 	return i18n.NewError(ctx, msgs.MsgPrivateTxMgrDomainMismatch, localTx.Transaction.Domain, domainName, domainAPI.Address())
	// }
	// localTx.Transaction.Domain = domainName

	// err = domainAPI.InitTransaction(ctx, tx, localTx)
	// if err != nil {
	// 	return err
	// }

	// if tx.PreAssembly == nil {
	// 	return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "PreAssembly is nil")
	// }

	// oc, err := p.getSequencerForContract(ctx, dbTX, contractAddr, domainAPI, tx)
	// if err != nil {
	// 	return err
	// }
	// queued := oc.ProcessNewTransaction(ctx, tx)
	// if queued {
	// 	log.L(ctx).Debugf("Transaction with ID %s queued in database", tx.ID)
	// }
	return nil
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
	// p.sequencersLock.RLock()
	// defer p.sequencersLock.RUnlock()
	// targetSequencer := p.sequencers[event.GetContractAddress()]
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
