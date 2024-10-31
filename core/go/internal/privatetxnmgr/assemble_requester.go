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
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// AssembleCoordinator is a component that is responsible for coordinating the assembly of all transactions for a given domain contract instance
// requests to assemble transactions are accepted and the actual assembly is performed asynchronously
type AssembleCoordinator interface {
	Start()
	Stop()
	RequestAssemble(ctx context.Context, assemblingNode string, transactionID uuid.UUID, transactionInputs *components.TransactionInputs, transactionPreAssembly *components.TransactionPreAssembly, callbacks AssembleRequestCallbacks)
	Commit(requestID string, stateDistributions []*statedistribution.StateDistribution)
}

// TODO should the pass a channel or a callback to the requestor to signal that the request is complete?
type AssembleRequestCompleteCallback func(requestID string, postAssembly *components.TransactionPostAssembly)
type AssembleRequestFailedCallback func(requestID string, err error)
type AssembleRequestCallbacks struct {
	OnComplete AssembleRequestCompleteCallback
	OnFail     AssembleRequestFailedCallback
}

type assembleCoordinator struct {
	ctx                  context.Context
	nodeName             string
	requests             chan *assembleRequest
	stopProcess          chan bool
	commit               chan string
	components           components.AllComponents
	domainAPI            components.DomainSmartContract
	domainContext        components.DomainContext
	transportWriter      ptmgrtypes.TransportWriter
	contractAddress      tktypes.EthAddress
	sequencerEnvironment ptmgrtypes.SequencerEnvironment
	requestTimeout       time.Duration
	stateDistributer     statedistribution.StateDistributer
}

type assembleRequest struct {
	assemblingNode         string
	assembleCoordinator    *assembleCoordinator
	transactionID          uuid.UUID
	transactionInputs      *components.TransactionInputs
	transactionPreassembly *components.TransactionPreAssembly
	callbacks              AssembleRequestCallbacks
}

func NewAssembleCoordinator(ctx context.Context, nodeName string, maxPendingRequests int, components components.AllComponents, domainAPI components.DomainSmartContract, domainContext components.DomainContext, transportWriter ptmgrtypes.TransportWriter, contractAddress tktypes.EthAddress, sequencerEnvironment ptmgrtypes.SequencerEnvironment, requestTimeout time.Duration, stateDistributer statedistribution.StateDistributer) AssembleCoordinator {
	return &assembleCoordinator{
		ctx:                  ctx,
		nodeName:             nodeName,
		stopProcess:          make(chan bool, 1),
		requests:             make(chan *assembleRequest, maxPendingRequests),
		commit:               make(chan string, 1),
		components:           components,
		domainAPI:            domainAPI,
		domainContext:        domainContext,
		transportWriter:      transportWriter,
		contractAddress:      contractAddress,
		sequencerEnvironment: sequencerEnvironment,
		requestTimeout:       requestTimeout,
		stateDistributer:     stateDistributer,
	}
}

func (ac *assembleCoordinator) Commit(requestID string, stateDistributions []*statedistribution.StateDistribution) {

	log.L(ac.ctx).Debugf("AssembleCoordinator:Commit %s", requestID)
	ac.stateDistributer.DistributeStates(ac.ctx, stateDistributions)
	ac.commit <- requestID

}
func (ac *assembleCoordinator) Start() {
	log.L(ac.ctx).Info("Starting AssembleCoordinator")
	go func() {
		for {
			select {
			case req := <-ac.requests:
				requestID := uuid.New().String()
				if req.assemblingNode == "" || req.assemblingNode == ac.nodeName {
					req.processLocal(ac.ctx, requestID)
				} else {
					err := req.processRemote(ac.ctx, req.assemblingNode, requestID)
					if err != nil {
						log.L(ac.ctx).Errorf("AssembleCoordinator request failed: %s", err)
						continue
					}

				}
				//The actual response is processed on the sequencer event loop.  We just need to know when it is safe to proceed
				// to the next request
				ac.waitForDone(requestID)
			case <-ac.stopProcess:
				log.L(ac.ctx).Info("assembleCoordinator loop process stopped")
				return
			case <-ac.ctx.Done():
				log.L(ac.ctx).Info("AssembleCoordinator loop exit due to canceled context")
				return
			}
		}
	}()
}

func (ac *assembleCoordinator) waitForDone(requestID string) {
	log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone %s", requestID)

	// wait for the response or a timeout
	timeoutTimer := time.NewTimer(ac.requestTimeout)
out:
	for {
		select {
		case response := <-ac.commit:
			if response == requestID {
				log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone received notification of completion %s", requestID)
				break out
			} else {
				// the response was not for this request, must have been an old request that we have already timed out
				log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone received spurious response %s. Continue to wait for %s", response, requestID)
			}
		case <-ac.ctx.Done():
			log.L(ac.ctx).Info("AssembleCoordinator:waitForDone loop exit due to canceled context")
			return
		case <-timeoutTimer.C:
			log.L(ac.ctx).Errorf("AssembleCoordinator:waitForDone request timeout for request %s", requestID)
			//sequencer event loop is responsible for requesting a new assemble
			break
		}
	}
	log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone done %s", requestID)

}

func (ac *assembleCoordinator) Stop() {
	// try to send an item in `stopProcess` channel, which has a buffer of 1
	// if it already has an item in the channel, this function does nothing
	select {
	case ac.stopProcess <- true:
	default:
	}

}

// TODO really need to figure out the separation between PrivateTxManager and DomainManager
// to allow us to do the assemble on a separate thread and without worrying about locking the PrivateTransaction objects
// we copy the pertinent structures out of the PrivateTransaction and pass them to the assemble thread
// and then use them to create another private transaction object that is passed to the domain manager which then just unpicks it again
func (ac *assembleCoordinator) RequestAssemble(ctx context.Context, assemblingNode string, transactionID uuid.UUID, transactionInputs *components.TransactionInputs, transactionPreAssembly *components.TransactionPreAssembly, callbacks AssembleRequestCallbacks) {

	ac.requests <- &assembleRequest{
		assemblingNode:         assemblingNode,
		assembleCoordinator:    ac,
		transactionID:          transactionID,
		callbacks:              callbacks,
		transactionInputs:      transactionInputs,
		transactionPreassembly: transactionPreAssembly,
	}
	log.L(ctx).Debugf("RequestAssemble: assemble request for %s queued", transactionID)

}

func (req *assembleRequest) processLocal(ctx context.Context, requestID string) {
	log.L(ctx).Debug("assembleRequest:processLocal")
	// we are the node that is responsible for assembling this transaction
	readTX := req.assembleCoordinator.components.Persistence().DB() // no DB transaction required here

	transaction := &components.PrivateTransaction{
		ID:          req.transactionID,
		Inputs:      req.transactionInputs,
		PreAssembly: req.transactionPreassembly,
	}

	err := req.assembleCoordinator.domainAPI.AssembleTransaction(req.assembleCoordinator.domainContext, readTX, transaction)
	if err != nil {
		req.callbacks.OnFail(requestID, err)
		return
	}
	if transaction.PostAssembly == nil {
		// This is most likely a programming error in the domain
		err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "AssembleTransaction returned nil PostAssembly")
		req.callbacks.OnFail(requestID, err)
		return
	}

	// Some validation that we are confident we can execute the given attestation plan
	for _, attRequest := range transaction.PostAssembly.AttestationPlan {
		switch attRequest.AttestationType {
		case prototk.AttestationType_ENDORSE:
		case prototk.AttestationType_SIGN:
		case prototk.AttestationType_GENERATE_PROOF:
			errorMessage := "AttestationType_GENERATE_PROOF is not implemented yet"
			log.L(ctx).Error(errorMessage)
			err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
			req.callbacks.OnFail(requestID, err)

		default:
			errorMessage := fmt.Sprintf("Unsupported attestation type: %s", attRequest.AttestationType)
			log.L(ctx).Error(errorMessage)
			err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
			req.callbacks.OnFail(requestID, err)
		}
	}

	log.L(ctx).Debug("assembleRequest:processLocal complete")

	req.callbacks.OnComplete(requestID, transaction.PostAssembly)

}

func (req *assembleRequest) processRemote(ctx context.Context, assemblingNode string, requestID string) error {

	//Assemble may require a call to another node ( in the case we have been delegated to coordinate transaction for other nodes)
	//Usually, they will get sent to us already assembled but there may be cases where we need to re-assemble
	// so this needs to be an async step
	// however, there must be only one assemble in progress at a time or else there is a risk that 2 transactions could chose to spend the same state
	//   (TODO - maybe in future, we could further optimize this and allow multiple assembles to be in progress if we can assert that they are not presented with the same available states)
	//   However, before we do that, we really need to sort out the separation of concerns between the domain manager, state store and private transaction manager and where the responsibility to single thread the assembly stream(s) lies

	log.L(ctx).Debugf("assembleRequest:processRemote requestID %s", requestID)

	stateLocksJSON, err := req.assembleCoordinator.domainContext.ExportStateLocks()
	if err != nil {
		req.callbacks.OnFail(requestID, err)
		return err
	}

	contractAddressString := req.assembleCoordinator.contractAddress.String()
	blockHeight := req.assembleCoordinator.sequencerEnvironment.GetBlockHeight()
	log.L(ctx).Debugf("assembleRequest:processRemote Assembling transaction %s on node %s", req.transactionID.String(), assemblingNode)

	//send a request to the node that is responsible for assembling this transaction
	err = req.assembleCoordinator.transportWriter.SendAssembleRequest(ctx, assemblingNode, requestID, req.transactionID, contractAddressString, req.transactionInputs, req.transactionPreassembly, stateLocksJSON, blockHeight)
	if err != nil {
		log.L(ctx).Errorf("assembleRequest:processRemote error from sendAssembleRequest: %s", err)
		req.callbacks.OnFail(requestID, err)
		return err
	}
	return nil
}
