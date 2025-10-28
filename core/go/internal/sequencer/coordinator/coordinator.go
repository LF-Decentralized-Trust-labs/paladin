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
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/syncpoints"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/transport"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

// The coordinator event loop is the only thread-safe way to access state about transactions externally.
// These structures allow an external caller (e.g. another component - but currently just another part of
// the distributed sequencer) to request data from the sequencer which it will respond to in between other
// updates to its state machine(s). This is not necessarily the optimal way to introspect the sequencer's
// state in a thread-safe way, but the initial implementation focuses on thread safety over optimal performance.
type QueryRequest struct {
	Type     Query
	Response chan any
}
type Query string

const (
	QueryTypeDispatchedTransactions Query = "dispatched_transactions"
)

func (tt Query) Enum() pldtypes.Enum[Query] {
	return pldtypes.Enum[Query](tt)
}

func (tt Query) Options() []string {
	return []string{
		string(QueryTypeDispatchedTransactions),
	}
}

type SeqCoordinator interface {
	// Asynchronously update the state machine by queueing an event to be processed. Most
	// callers should use this interface.
	QueueEvent(ctx context.Context, event common.Event)

	// Synchronously update the state machine by processing this event. Primarily used for testing the state machine.
	ProcessEvent(ctx context.Context, event common.Event) error

	// Manage the state of the coordinator
	GetActiveCoordinatorNode(ctx context.Context) string
	SetActiveCoordinatorNode(ctx context.Context, coordinatorNode string)
	GetCurrentState() State
	UpdateOriginatorNodePool(ctx context.Context, originatorNode string)

	// Transactions being sequencer (here or elsewhere)
	GetTransactionsReadyToDispatch(ctx context.Context) ([]*components.PrivateTransaction, error)
	GetTransactionByID(ctx context.Context, txID uuid.UUID) *transaction.Transaction

	// Lifecycle
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
	contractAddress                *pldtypes.EthAddress
	blockHeightTolerance           uint64
	closingGracePeriod             int // expressed as a multiple of heartbeat intervals
	requestTimeout                 common.Duration
	assembleTimeout                common.Duration
	originatorNodePool             []string // The (possibly changing) list of originator nodes
	nodeName                       string
	coordinatorSelectionBlockRange uint64
	maxInflightTransactions        int
	maxDispatchAhead               int

	/* Dependencies */
	domainAPI          components.DomainSmartContract
	transportWriter    transport.TransportWriter
	clock              common.Clock
	engineIntegration  common.EngineIntegration
	syncPoints         syncpoints.SyncPoints
	readyForDispatch   func(context.Context, *transaction.Transaction)
	coordinatorStarted func(contractAddress *pldtypes.EthAddress, coordinatorNode string)
	coordinatorIdle    func(contractAddress *pldtypes.EthAddress)
	heartbeatCtx       context.Context
	heartbeatCancel    context.CancelFunc
	metrics            metrics.DistributedSequencerMetrics

	/*Algorithms*/
	transactionSelector TransactionSelector

	/* Event loop */
	coordinatorEvents chan common.Event
	externalQueries   chan QueryRequest
	stopEventLoop     chan struct{}

	/* Dispatch loop */
	dispatchQueue    chan *transaction.Transaction
	stopDispatchLoop chan struct{}
}

func NewCoordinator(
	ctx context.Context,
	contractAddress *pldtypes.EthAddress,
	domainAPI components.DomainSmartContract,
	transportWriter transport.TransportWriter,
	clock common.Clock,
	engineIntegration common.EngineIntegration,
	syncPoints syncpoints.SyncPoints,
	requestTimeout,
	assembleTimeout common.Duration,
	blockRangeSize uint64,
	blockHeightTolerance uint64,
	closingGracePeriod int,
	maxInflightTransactions int,
	maxDispatchAhead int,
	nodeName string,
	metrics metrics.DistributedSequencerMetrics,
	readyForDispatch func(context.Context, *transaction.Transaction),
	coordinatorStarted func(contractAddress *pldtypes.EthAddress, coordinatorNode string),
	coordinatorIdle func(contractAddress *pldtypes.EthAddress),
) (*coordinator, error) {
	c := &coordinator{
		heartbeatIntervalsSinceStateChange: 0,
		transactionsByID:                   make(map[uuid.UUID]*transaction.Transaction),
		domainAPI:                          domainAPI,
		transportWriter:                    transportWriter,
		contractAddress:                    contractAddress,
		blockHeightTolerance:               blockHeightTolerance,
		closingGracePeriod:                 closingGracePeriod,
		maxInflightTransactions:            maxInflightTransactions,
		maxDispatchAhead:                   maxDispatchAhead,
		grapher:                            transaction.NewGrapher(ctx),
		clock:                              clock,
		requestTimeout:                     requestTimeout,
		assembleTimeout:                    assembleTimeout,
		engineIntegration:                  engineIntegration,
		syncPoints:                         syncPoints,
		readyForDispatch:                   readyForDispatch,
		coordinatorStarted:                 coordinatorStarted,
		coordinatorIdle:                    coordinatorIdle,
		nodeName:                           nodeName,
		metrics:                            metrics,
		coordinatorEvents:                  make(chan common.Event, 50), // TODO >1 only required for sqlite coarse-grained locks. Should this be DB-dependent?
		externalQueries:                    make(chan QueryRequest, 10),
		stopEventLoop:                      make(chan struct{}),
		coordinatorSelectionBlockRange:     blockRangeSize,
		dispatchQueue:                      make(chan *transaction.Transaction, maxInflightTransactions),
		stopDispatchLoop:                   make(chan struct{}),
	}
	c.originatorNodePool = make([]string, 0)
	c.InitializeStateMachine(State_Idle)
	c.transactionSelector = NewTransactionSelector(ctx, c)

	// Start event processing loop
	go c.eventLoop(ctx)

	// Start dispatch queue loop
	go c.dispatchLoop(ctx)

	return c, nil

}

func (c *coordinator) eventLoop(ctx context.Context) {
	for {
		log.L(ctx).Debugf("coordinator event loop waiting for next event")
		select {
		case query := <-c.externalQueries:
			log.L(ctx).Debugf("coordinator handling external query %s", query.Type)
			if query.Type == QueryTypeDispatchedTransactions {
				dispatchingTransactionCount := len(c.getTransactionsInStates(ctx, []transaction.State{transaction.State_Dispatched, transaction.State_Submitted}))
				log.L(ctx).Tracef("coordinator responding to %s: %d dispatched transactions", query.Type, dispatchingTransactionCount)
				query.Response <- dispatchingTransactionCount
			} else {
				log.L(ctx).Errorf("%s", i18n.NewError(ctx, msgs.MsgSequencerInternalError, "external query type unknown").Error())
			}
		case event := <-c.coordinatorEvents:
			log.L(ctx).Debugf("coordinator pulled event from the queue: %s", event.TypeString())
			err := c.ProcessEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("error processing event: %v", err)
			}
		case <-c.stopEventLoop:
			log.L(ctx).Infof("coordinator event loop cancelled")
			return
		}
	}
}

func (c *coordinator) dispatchLoop(ctx context.Context) {
	for {
		log.L(ctx).Debugf("coordinator dispatch loop waiting for next TX to dispatch, max dispatch ahead: %d", c.maxDispatchAhead)
		select {
		case tx := <-c.dispatchQueue:
			log.L(ctx).Debugf("coordinator pulled transaction %s from the dispatch queue", tx.ID.String())
			// Got the next TX, but can't dispatch too far ahead in case an earlier TX reverts on-chain
			coordinatorQuery := QueryRequest{
				Type:     QueryTypeDispatchedTransactions,
				Response: make(chan any),
			}

			log.L(ctx).Debugf("coordinator asking for number of transactions currently in dispatch")
		waitForDispatch:
			for true {
				queryStart := time.Now() // While this mechanism is proving out, capture query times
				c.externalQueries <- coordinatorQuery
				select {
				case response := <-coordinatorQuery.Response:
					queryDuration := time.Since(queryStart)
					log.L(ctx).Debugf("coordinator query response %d us", queryDuration.Microseconds())
					inFlight := response.(int)
					if inFlight < c.maxDispatchAhead {
						log.L(ctx).Debugf("inFlight TX count %d < maxDispatchAhead %d - OK to dispatch", inFlight, c.maxDispatchAhead)
						break waitForDispatch
					}
				case <-ctx.Done():
					log.L(ctx).Infof("coordinator dispatch loop cancelled during query")
					return
				}

				// There are too many transactions making their way to the base ledger. If we're hitting this frequently
				// then the node has probably been configured to optimise for base-ledger safety rather than optimal
				// throughput
				time.Sleep(100 * time.Millisecond)
			}
			log.L(ctx).Debugf("coordinator submitting transaction %s for dispatch", tx.ID.String())
			c.readyForDispatch(ctx, tx)
		case <-c.stopDispatchLoop:
			log.L(ctx).Infof("coordinator dispatch loop stopped")
			return
		case <-ctx.Done():
			log.L(ctx).Infof("coordinator dispatch loop cancelled")
			return
		}
	}
}

func (c *coordinator) sendHandoverRequest(ctx context.Context) {
	err := c.transportWriter.SendHandoverRequest(ctx, c.activeCoordinatorNode, c.contractAddress)
	if err != nil {
		log.L(ctx).Errorf("error sending handover request: %v", err)
	}
}

func (c *coordinator) GetActiveCoordinatorNode(ctx context.Context) string {
	if c.activeCoordinatorNode == "" {
		// If we don't yet have an active coordinator, select one based on the appropriate algorithm for the contract type
		activeCoordinator, err := c.selectNextActiveCoordinator(ctx)
		if err != nil {
			log.L(ctx).Errorf("error selecting next active coordinator: %v", err)
			return ""
		}
		c.activeCoordinatorNode = activeCoordinator
	}
	return c.activeCoordinatorNode
}

func (c *coordinator) SetActiveCoordinatorNode(ctx context.Context, coordinatorNode string) {
	log.L(ctx).Debugf("set active coordinator for contract %s: %s", c.contractAddress.String(), coordinatorNode)
	c.activeCoordinatorNode = coordinatorNode
}

func (c *coordinator) selectNextActiveCoordinator(ctx context.Context) (string, error) {
	coordinator := ""
	if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_STATIC {
		// E.g. Noto
		if c.domainAPI.ContractConfig().GetStaticCoordinator() == "" {
			return "", fmt.Errorf("static coordinator mode is configured but static coordinator node is not set")
		}
		log.L(ctx).Debugf("coordinator %s selected as next active coordinator in static coordinator mode", c.domainAPI.ContractConfig().GetStaticCoordinator())
		// If the static coordinator returns a fully qualified identity extract just the node name
		if strings.Contains(c.domainAPI.ContractConfig().GetStaticCoordinator(), "@") {
			parts := strings.Split(c.domainAPI.ContractConfig().GetStaticCoordinator(), "@")
			coordinator = parts[1]
		} else {
			coordinator = c.domainAPI.ContractConfig().GetStaticCoordinator()
		}
	} else if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_ENDORSER {
		// E.g. Pente
		// Make a fair choice about the next coordinator, but for now just choose the node this request arrived at
		if len(c.originatorNodePool) == 0 {
			coordinator = c.nodeName
			log.L(ctx).Warnf("coordinator %s selected based on empty originator pool (local node)", coordinator)
		} else {
			// Round block number down to the nearest block range (e.g. block 1012, 1013, 1014 etc. all become 1000 for hashing)
			effectiveBlockNumber := c.currentBlockHeight - (c.currentBlockHeight % c.coordinatorSelectionBlockRange)

			// Take a numeric hash of the identities using the current block range
			h := fnv.New32a()
			h.Write([]byte(strconv.FormatUint(effectiveBlockNumber, 10)))
			coordinator = c.originatorNodePool[int(h.Sum32())%len(c.originatorNodePool)]
			log.L(ctx).Debugf("coordinator %s selected based on hash modulus of the originator pool", coordinator)
		}
	} else if c.domainAPI.ContractConfig().GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_SENDER {
		// E.g. Zeto
		log.L(ctx).Debugf("coordinator %s selected as next active coordinator in originator coordinator mode", c.nodeName)
		coordinator = c.nodeName
	}

	log.L(ctx).Debugf("selected active coordinator for contract %s: %s", c.contractAddress.String(), coordinator)

	if strings.Contains(coordinator, "@") || coordinator == "" {
		return "", i18n.NewError(ctx, msgs.MsgSequencerInternalError, "coordinator node is invalid")
	}

	return coordinator, nil
}

func (c *coordinator) UpdateOriginatorNodePool(ctx context.Context, originatorNode string) {
	if strings.Contains(originatorNode, "@") {
		log.L(ctx).Errorf("originator ID provided, originator node expected: %s", originatorNode)
	}
	log.L(ctx).Debugf("updating originator node pool with node %s", originatorNode)
	if !slices.Contains(c.originatorNodePool, originatorNode) {
		c.originatorNodePool = append(c.originatorNodePool, originatorNode)
		slices.Sort(c.originatorNodePool)
	}
}

// TODO consider renaming to setDelegatedTransactionsForOriginator to make it clear that we expect originators to include all inflight transactions in every delegation request and therefore this is
// a replace, not an add.  Need to finalize the decision about whether we expect the originator to include all inflight delegated transactions in every delegation request. Currently the code assumes we do so need to make the spec clear on that point and
// record a decision record to explain why.  Every  time we come back to this point, we will be tempted to reverse that decision so we need to make sure we have a record of the known consequences.
// originator must be a fully qualified identity locator otherwise an error will be returned
func (c *coordinator) addToDelegatedTransactions(ctx context.Context, originator string, transactions []*components.PrivateTransaction) error {
	var previousTransaction *transaction.Transaction
	for _, txn := range transactions {

		if len(c.transactionsByID) >= c.maxInflightTransactions {
			return i18n.NewError(ctx, msgs.MsgSequencerMaxInflightTransactions, c.maxInflightTransactions)
		}

		newTransaction, err := transaction.NewTransaction(
			ctx,
			originator,
			txn,
			c.transportWriter,
			c.clock,
			c.ProcessEvent,
			c.engineIntegration,
			c.syncPoints,
			c.requestTimeout,
			c.assembleTimeout,
			c.closingGracePeriod,
			c.grapher,
			c.metrics,
			c.queueForDispatch,
			func(ctx context.Context, t *transaction.Transaction, to, from transaction.State) {
				// TX state changed, check if we need to be selecting the next transaction for this sequencer
				//TODO the following logic should be moved to the state machine so that all the rules are in one place
				if c.stateMachine.currentState == State_Active {
					if from == transaction.State_Assembling || to == transaction.State_Pooled {
						err := c.selectNextTransaction(ctx, &TransactionStateTransitionEvent{
							TransactionID: t.ID,
							From:          from,
							To:            to,
						})
						if err != nil {
							log.L(ctx).Errorf("error selecting next transaction after transaction %s moved from %s to %s: %v", t.ID.String(), from.String(), to.String(), err)
							//TODO figure out how to get this to the abend handler
						}
					}
				}
			},
			func(ctx context.Context) {
				// TX cleaned up after confirmation & sufficient heartbeats
				delete(c.transactionsByID, txn.ID)
				c.metrics.DecCoordinatingTransactions()
				err := c.grapher.Forget(txn.ID)

				if err != nil {
					log.L(ctx).Errorf("error forgetting transaction %s: %v", txn.ID.String(), err)
				}
				log.L(ctx).Debugf("transaction %s cleaned up", txn.ID.String())
			},
		)
		if err != nil {
			log.L(ctx).Errorf("error creating transaction: %v", err)
			return err
		}

		if previousTransaction != nil {
			newTransaction.SetPreviousTransaction(ctx, previousTransaction)
			previousTransaction.SetNextTransaction(ctx, newTransaction)
		}
		c.transactionsByID[txn.ID] = newTransaction
		c.metrics.IncCoordinatingTransactions()
		previousTransaction = newTransaction

		receivedEvent := &transaction.ReceivedEvent{}
		receivedEvent.TransactionID = txn.ID
		err = c.transactionsByID[txn.ID].HandleEvent(ctx, receivedEvent)
		if err != nil {
			log.L(ctx).Errorf("error handling ReceivedEvent for transaction %s: %v", txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func (c *coordinator) queueForDispatch(ctx context.Context, txn *transaction.Transaction) {
	c.dispatchQueue <- txn
}

func (c *coordinator) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := c.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("ignoring event because transaction not known to this coordinator %s", event.GetTransactionID().String())
	}
	return nil
}

func (c *coordinator) propagateEventToAllTransactions(ctx context.Context, event common.Event) error {
	for _, txn := range c.transactionsByID {
		err := txn.HandleEvent(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("error handling event %v for transaction %s: %v", event.Type(), txn.ID.String(), err)
			return err
		}
	}
	return nil
}

func (c *coordinator) getTransactionsInStates(ctx context.Context, states []transaction.State) []*transaction.Transaction {
	//TODO this could be made more efficient by maintaining a separate index of transactions for each state but that is error prone so
	// deferring until we have a comprehensive test suite to catch errors
	log.L(ctx).Debugf("getting transactions in states: %+v", states)
	matchingStates := make(map[transaction.State]bool)
	for _, state := range states {
		matchingStates[state] = true
	}

	log.L(ctx).Debugf("looking across %d transactions for those in states: %+v", len(c.transactionsByID), states)
	matchingTxns := make([]*transaction.Transaction, 0, len(c.transactionsByID))
	for _, txn := range c.transactionsByID {
		if matchingStates[txn.GetState()] {
			log.L(ctx).Debugf("found transaction %s in state %s", txn.ID.String(), txn.GetState())
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
			log.L(ctx).Tracef("Tracked TX ID %s", txn.ID.String())
		}
		if txn != nil && txn.GetSignerAddress() != nil {
			log.L(ctx).Tracef("Tracked TX ID %s signer address '%s'", txn.ID.String(), txn.GetSignerAddress().String())
		}
		if txn.GetSignerAddress() != nil && *txn.GetSignerAddress() == *signer && txn.GetNonce() != nil && *(txn.GetNonce()) == nonce {
			return txn
		}
	}
	return nil
}

func (c *coordinator) confirmDispatchedTransaction(ctx context.Context, txId uuid.UUID, from *pldtypes.EthAddress, nonce uint64, hash pldtypes.Bytes32, revertReason pldtypes.HexBytes) (bool, error) {
	log.L(ctx).Debugf("we currently have %d transactions to handle, confirming that dispatched TX %s is in our list", len(c.transactionsByID), txId.String())

	// First check whether it is one that we have been coordinating
	if dispatchedTransaction := c.findTransactionBySignerNonce(ctx, from, nonce); dispatchedTransaction != nil {
		if dispatchedTransaction.GetLatestSubmissionHash() == nil || *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
			// Is this not the transaction that we are looking for?
			// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
			// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
			log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected", dispatchedTransaction.ID.String())
		}
		event := &transaction.ConfirmedEvent{
			Hash:         hash,
			RevertReason: revertReason,
			Nonce:        nonce,
		}
		event.TransactionID = dispatchedTransaction.ID
		event.EventTime = time.Now()
		err := dispatchedTransaction.HandleEvent(ctx, event)
		if err != nil {
			log.L(ctx).Errorf("error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
			return false, err
		}
		return true, nil
	}

	for _, dispatchedTransaction := range c.transactionsByID {
		if dispatchedTransaction.ID == txId {
			if dispatchedTransaction.GetLatestSubmissionHash() == nil {
				// Is this not the transaction that we are looking for?
				// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
				// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
				log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected. Dispatch hash nil, confirmed hash %s", dispatchedTransaction.ID.String(), hash.String())
			} else if *(dispatchedTransaction.GetLatestSubmissionHash()) != hash {
				// Is this not the transaction that we are looking for?
				// We have missed a submission?  Or is it possible that an earlier submission has managed to get confirmed?
				// It is interesting so we log it but either way,  this must be the transaction that we are looking for because we can't re-use a nonce
				log.L(ctx).Debugf("transaction %s confirmed with a different hash than expected. Dispatch hash %s, confirmed hash %s", dispatchedTransaction.ID.String(), dispatchedTransaction.GetLatestSubmissionHash(), hash.String())
			}
			event := &transaction.ConfirmedEvent{
				Hash:         hash,
				RevertReason: revertReason,
				Nonce:        nonce,
			}
			event.TransactionID = txId
			event.EventTime = time.Now()

			log.L(ctx).Debugf("Confirming dispatched TX %s", txId.String())
			err := dispatchedTransaction.HandleEvent(ctx, event)
			if err != nil {
				log.L(ctx).Errorf("error handling ConfirmedEvent for transaction %s: %v", dispatchedTransaction.ID.String(), err)
				return false, err
			}
			return true, nil
		}
	}
	log.L(ctx).Infof("failed to find a transaction submitted by signer %s", from.String())
	return false, nil

}

func (c *coordinator) confirmMonitoredTransaction(_ context.Context, from *pldtypes.EthAddress, nonce uint64) {
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
// have an active sequencer for the contract address since it may be the only originator that can honour dispatch
// requests from another coordinator, but this node is no longer acting as the coordinator.
func (c *coordinator) Stop() {
	log.L(context.Background()).Infof("stopping coordinator for contract %s", c.contractAddress.String())

	// MRW TODO - The state machine doesn't really have a "please take over from me" path. Not a current priority
	// but clean "please take over" path may be needed in the future
	c.stopEventLoop <- struct{}{}
	c.stopDispatchLoop <- struct{}{}
}

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (c *coordinator) GetCurrentState() State {
	return c.stateMachine.currentState
}
