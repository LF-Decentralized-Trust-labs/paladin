/*
 * Copyright © 2025 Kaleido, Inc.
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

package coordinator

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type SeqCoordinator interface {
	GetTransactionsReadyToDispatch(ctx context.Context) ([]*components.PrivateTransaction, error)
	GetTransactionByID(ctx context.Context, txID uuid.UUID) *transaction.Transaction
	GetActiveCoordinatorNode(ctx context.Context) string
	HandleEvent(ctx context.Context, event common.Event) error
	UpdateSenderNodePool(ctx context.Context, senderNode string)
	GetCurrentState() common.CoordinatorState
	Stop()
}

type coordinator struct {
	/* State */
	stateMachine                               *StateMachine
	activeCoordinatorNode                      string
	activeCoordinatorBlockHeight               uint64
	heartbeatIntervalsSinceStateChange         int
	transactionsByID                           map[uuid.UUID]*transaction.Transaction
	currentBlockHeight                         uint64
	activeCoordinatorsFlushPointsBySignerNonce map[string]*common.FlushPoint
	grapher                                    transaction.Grapher

	/* Config */
	blockRangeSize       int64
	contractAddress      *pldtypes.EthAddress
	blockHeightTolerance uint64
	closingGracePeriod   int // expressed as a multiple of heartbeat intervals
	requestTimeout       common.Duration
	assembleTimeout      common.Duration
	senderNodePool       []string // The (possibly changing) list of sender nodes
	nodeName             string

	/* Dependencies */
	domainAPI          components.DomainSmartContract
	messageSender      MessageSender
	clock              common.Clock
	engineIntegration  common.EngineIntegration
	emit               common.EmitEvent
	readyForDispatch   func(context.Context, *transaction.Transaction)
	coordinatorStarted func(contractAddress *pldtypes.EthAddress, coordinatorNode string)
	coordinatorIdle    func(contractAddress *pldtypes.EthAddress)
	heartbeatCtx       context.Context
	heartbeatCancel    context.CancelFunc
	metrics            metrics.DistributedSequencerMetrics

	/*Algorithms*/
	transactionSelector TransactionSelector
}

func NewCoordinator(
	ctx context.Context,
	domainAPI components.DomainSmartContract,
	messageSender MessageSender,
	senderNodePool []string,
	clock common.Clock,
	emit common.EmitEvent,
	engineIntegration common.EngineIntegration,
	requestTimeout,
	assembleTimeout common.Duration,
	blockRangeSize int64,
	contractAddress *pldtypes.EthAddress,
	blockHeightTolerance uint64,
	closingGracePeriod int,
	nodeName string,
	readyForDispatch func(context.Context, *transaction.Transaction),
	coordinatorStarted func(contractAddress *pldtypes.EthAddress, coordinatorNode string),
	coordinatorIdle func(contractAddress *pldtypes.EthAddress),
	metrics metrics.DistributedSequencerMetrics,
) (*coordinator, error) {
	c := &coordinator{
		heartbeatIntervalsSinceStateChange: 0,
		transactionsByID:                   make(map[uuid.UUID]*transaction.Transaction),
		domainAPI:                          domainAPI,
		messageSender:                      messageSender,
		blockRangeSize:                     blockRangeSize,
		contractAddress:                    contractAddress,
		blockHeightTolerance:               blockHeightTolerance,
		closingGracePeriod:                 closingGracePeriod,
		grapher:                            transaction.NewGrapher(ctx),
		clock:                              clock,
		requestTimeout:                     requestTimeout,
		assembleTimeout:                    assembleTimeout,
		engineIntegration:                  engineIntegration,
		emit:                               emit,
		readyForDispatch:                   readyForDispatch,
		coordinatorStarted:                 coordinatorStarted,
		coordinatorIdle:                    coordinatorIdle,
		nodeName:                           nodeName,
		metrics:                            metrics,
	}
	c.senderNodePool = make([]string, 0)
	// for _, member := range senderNodePool {
	// 	memberLocator := pldtypes.PrivateIdentityLocator(member)
	// 	memberNode, err := memberLocator.Node(ctx, false)
	// 	if err != nil {
	// 		log.L(ctx).Errorf("[Sequencer] error resolving node for member %s: %v", member, err)
	// 		return nil, err
	// 	}

	// 	memberIdentity, err := memberLocator.Identity(ctx)
	// 	if err != nil {
	// 		log.L(ctx).Errorf("[Sequencer] error resolving identity for member %s: %v", member, err)
	// 		return nil, err
	// 	}

	// 	if _, ok := c.senderNodePool[memberNode]; !ok {
	// 		c.senderNodePool[memberNode] = make([]string, 0)
	// 	}

	// 	c.senderNodePool[memberNode] = append(c.senderNodePool[memberNode], memberIdentity)

	// }
	c.InitializeStateMachine(State_Idle)
	c.transactionSelector = NewTransactionSelector(ctx, c)

	activeCoordinator, err := c.SelectNextActiveCoordinator(ctx)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error selecting next active coordinator: %v", err)
		return nil, err
	}
	c.activeCoordinatorNode = activeCoordinator
	return c, nil

}

func (c *coordinator) sendHandoverRequest(ctx context.Context) {
	c.messageSender.SendHandoverRequest(ctx, c.activeCoordinatorNode, c.contractAddress)
}

func (c *coordinator) GetActiveCoordinatorNode(ctx context.Context) string {
	return c.activeCoordinatorNode
}

func (c *coordinator) SelectNextActiveCoordinator(ctx context.Context) (string, error) {
	coordinator := ""
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_STATIC {
		// E.g. Noto
		log.L(ctx).Infof("[Sequencer] selecting next active coordinator in static coordinator mode")
		if c.domainAPI.ContractConfig().GetStaticCoordinator() == "" {
			return "", fmt.Errorf("Static coordinator mode is configured but static coordinator node is not set")
		}
		log.L(ctx).Infof("[Sequencer] selected next active coordinator in static coordinator mode: %s", c.domainAPI.ContractConfig().GetStaticCoordinator())
		coordinator = c.domainAPI.ContractConfig().GetStaticCoordinator()
	}
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_ENDORSER {
		// E.g. Pente
		// Make a fair choice about the next coordinator, but for now just choose the node this request arrived at
		log.L(ctx).Infof("[Sequencer] selecting next active coordinator in endorser coordinator mode")
		log.L(ctx).Infof("[Sequencer] selected next active coordinator in endorser coordinator mode: %s", c.nodeName)
		coordinator = c.nodeName
	}
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_SENDER {
		// E.g. Zeto
		log.L(ctx).Infof("[Sequencer] selecting next active coordinator in sender coordinator mode")
		log.L(ctx).Infof("[Sequencer] selected next active coordinator in sender coordinator mode: %s", c.nodeName)
		coordinator = c.nodeName
	}
	// Strip everything after the @ to get the node node
	coordinatorNode := strings.Split(coordinator, "@")
	if len(coordinatorNode) > 1 {
		return coordinatorNode[1], nil
	}

	if coordinatorNode[0] == "" {
		return "", fmt.Errorf("coordinator node not found")
	}
	return coordinatorNode[0], nil
}

func (c *coordinator) UpdateSenderNodePool(ctx context.Context, senderNode string) {
	log.L(ctx).Infof("[Sequencer] updating sender node pool with %s", senderNode)
	if !slices.Contains(c.senderNodePool, senderNode) {
		c.senderNodePool = append(c.senderNodePool, senderNode)
	}
	//c.senderNodePool[senderNode] = append(c.senderNodePool[senderNode], senderNode)
}

// TODO consider renaming to setDelegatedTransactionsForSender to make it clear that we expect senders to include all inflight transactions in every delegation request and therefore this is
// a replace, not an add.  Need to finalize the decision about whether we expect the sender to include all inflight delegated transactions in every delegation request. Currently the code assumes we do so need to make the spec clear on that point and
// record a decision record to explain why.  Every  time we come back to this point, we will be tempted to reverse that decision so we need to make sure we have a record of the known consequences.
// sender must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(ctx context.Context, sender string, transactions []*components.PrivateTransaction) error {

	//TODO should remove any transactions from the same sender that we already have but are not in this list

	var previousTransaction *transaction.Transaction
	for _, txn := range transactions {
		newTransaction, err := transaction.NewTransaction(
			ctx,
			sender,
			txn,
			c.messageSender,
			c.clock,
			c.emit,
			c.engineIntegration,
			c.requestTimeout,
			c.assembleTimeout,
			c.closingGracePeriod,
			c.grapher,
			func(ctx context.Context, t *transaction.Transaction, to, from transaction.State) {
				//callback function to notify us when the transaction changes state
				log.L(ctx).Debugf("[Sequencer] transaction %s moved from %s to %s", t.ID.String(), from.String(), to.String())
				//TODO the following logic should be moved to the state machine so that all the rules are in one place
				if c.stateMachine.currentState == State_Active {
					if from == transaction.State_Assembling {
						err := c.selectNextTransaction(ctx, &TransactionStateTransitionEvent{
							TransactionID: t.ID,
							From:          from,
							To:            to,
						})
						if err != nil {
							log.L(ctx).Errorf("[Sequencer] error selecting next transaction after transaction %s moved from %s to %s: %v", t.ID.String(), from.String(), to.String(), err)
							//TODO figure out how to get this to the abend handler
						}
					}
				}
			},
			func(ctx context.Context) {
				//callback function to notify us when the transaction is cleaned up
				delete(c.transactionsByID, txn.ID)
				err := c.grapher.Forget(txn.ID)
				if err != nil {
					log.L(ctx).Errorf("[Sequencer] error forgetting transaction %s: %v", txn.ID.String(), err)
				}
				log.L(ctx).Debugf("[Sequencer] transaction %s cleaned up", txn.ID.String())
			},
			c.readyForDispatch,
			c.metrics,
		)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] error creating transaction: %v", err)
			return err
		}

		if previousTransaction != nil {
			newTransaction.SetPreviousTransaction(ctx, previousTransaction)
			previousTransaction.SetNextTransaction(ctx, newTransaction)
		}
		c.transactionsByID[txn.ID] = newTransaction
		previousTransaction = newTransaction

		receivedEvent := &transaction.ReceivedEvent{}
		receivedEvent.TransactionID = txn.ID
		err = c.transactionsByID[txn.ID].HandleEvent(context.Background(), receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func (c *coordinator) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := c.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("[Sequencer] ignoring event because transaction not known to this coordinator %s", event.GetTransactionID().String())
	}
	return nil
}

func (c *coordinator) propagateEventToAllTransactions(ctx context.Context, event common.Event) error {
	for _, txn := range c.transactionsByID {
		err := txn.HandleEvent(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] error handling event %v for transaction %s: %v", event.Type(), txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func (c *coordinator) getTransactionsInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	log.L(ctx).Infof("[Sequencer] getting transactions in states: %+v", states)
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}

	log.L(ctx).Infof("[Sequencer] found %d transactions in states: %+v", len(c.transactionsByID), states)
	matchingTxns := make([]*transaction.Transaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if matchingStates[txn.GetState()] {
			log.L(ctx).Infof("[Sequencer] found transaction %s in state %s", txn.ID.String(), txn.GetState())
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

func (c *coordinator) getTransactionsNotInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	nonMatchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		nonMatchingStates[state] = true
	}
	matchingTxns := make([]*transaction.Transaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if !nonMatchingStates[txn.GetState()] {
			matchingTxns = append(matchingTxns, txn)
		}
	}
	return matchingTxns
}

// MRW TODO - is there a reason we need to find by nonce and not by TX ID?
func (c *coordinator) findTransactionBySignerNonce(ctx context.Context, signer *pldtypes.EthAddress, nonce uint64) *transaction.Transaction {
	//TODO this would be more efficient by maintaining a separate index but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	for _, txn := range c.transactionsByID {
		if txn != nil {
			log.L(ctx).Infof("[Sequencer] Tracked TX ID %s", txn.ID.String())
		}
		if txn != nil && txn.GetSignerAddress() != nil {
			log.L(ctx).Infof("[Sequencer] Tracked TX ID %s signer address '%s'", txn.ID.String(), txn.GetSignerAddress().String())
		}
		if txn.GetSignerAddress() != nil && *txn.GetSignerAddress() == *signer && txn.GetNonce() != nil && *(txn.GetNonce()) == nonce {
			return txn
		}
	}
	return nil
}

func (c *coordinator) confirmDispatchedTransaction(ctx context.Context, txId uuid.UUID, from *pldtypes.EthAddress, nonce uint64, hash pldtypes.Bytes32, revertReason pldtypes.HexBytes) (bool, error) {
	log.L(ctx).Infof("[Sequencer] confirmDispatchedTransaction - we currently have %d transactions to handle", len(c.transactionsByID))
	// First check whether it is one that we have been coordinating
	if dispatchedTransaction := c.findTransactionBySignerNonce(ctx, from, nonce); dispatchedTransaction != nil {
		if dispatchedTransaction.GetLatestSubmissionHash() == nil || *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
			// Is this not the transaction that we are looking for?
			// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
			// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
			log.L(ctx).Debugf("[Sequencer] transaction %s confirmed with a different hash than expected", dispatchedTransaction.ID.String())
		}
		event := &transaction.ConfirmedEvent{
			Hash:         hash,
			RevertReason: revertReason,
		}
		event.TransactionID = dispatchedTransaction.ID
		event.EventTime = time.Now()
		err := dispatchedTransaction.HandleEvent(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("[Sequencer] error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
			return false, err
		}
		return true, nil
	}

	// MRW TODO - fallback while deciding whether we can confirm transactions by ID rather than signer
	for _, dispatchedTransaction := range c.transactionsByID {
		if dispatchedTransaction.ID == txId {
			if dispatchedTransaction.GetLatestSubmissionHash() == nil {
				// Is this not the transaction that we are looking for?
				// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
				// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
				log.L(ctx).Debugf("[Sequencer] transaction %s confirmed with a different hash than expected. Dispatch hash nil, confirmed hash %s", dispatchedTransaction.ID.String(), hash.String())
			} else if *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
				// Is this not the transaction that we are looking for?
				// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
				// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
				log.L(ctx).Debugf("[Sequencer] transaction %s confirmed with a different hash than expected. Dispatch hash %s, confirmed hash %s", dispatchedTransaction.ID.String(), dispatchedTransaction.GetLatestSubmissionHash(), hash.String())
			}
			event := &transaction.ConfirmedEvent{
				Hash:         hash,
				RevertReason: revertReason,
			}
			event.TransactionID = txId
			err := dispatchedTransaction.HandleEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
				return false, err
			}
			return true, nil
		}
	}
	log.L(ctx).Infof("[Sequencer] confirmDispatchedTransaction - failed to find a transaction submitted by signer %s", from.String())
	return false, nil

}

func (c *coordinator) confirmMonitoredTransaction(ctx context.Context, from *pldtypes.EthAddress, nonce uint64) {
	if flushPoint := c.activeCoordinatorsFlushPointsBySignerNonce[fmt.Sprintf("%s:%d", from.String(), nonce)]; flushPoint != nil {
		//We do not remove the flushPoint from the list because there is a chance that the coordinator hasn't seen this confirmation themselves and
		// when they send us the next heartbeat, it will contain this FlushPoint so it would get added back into the list and we would not see the confirmation again
		flushPoint.Confirmed = true
	}
}

func ptrTo[T any](v T) *T {
	return &v
}

// A coordinator may be required to stop if this node has reached its capacity. The node may still need to
// have an active sequencer for the contract address since it may be the only sender that can honour dispatch
// requests from another coordinator, but this node is no longer acting as the coordinator.
func (c *coordinator) Stop() {
	log.L(context.Background()).Infof("[Sequencer] stopping coordinator for contract %s", c.contractAddress.String())

	// Opt-out of being the coordinator
	handoverRequestEvent := &HandoverRequestEvent{}
	handoverRequestEvent.EventTime = time.Now()
	handoverRequestEvent.Requester = c.activeCoordinatorNode
	c.HandleEvent(context.Background(), handoverRequestEvent)
}

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (c *coordinator) GetCurrentState() common.CoordinatorState {
	return common.CoordinatorState(c.stateMachine.currentState.String())
}
