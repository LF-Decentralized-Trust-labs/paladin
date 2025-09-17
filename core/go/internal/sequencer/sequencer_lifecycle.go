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
	"sort"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/sender"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/syncpoints"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/transport"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

func (dSeq *distributedSequencer) senderEventHandler(event common.Event) {
	log.L(dSeq.ctx).Debugf("[Sequencer] handing off TX-emitted event to sequencer sender: %+v", event)
	dSeq.sender.HandleEvent(dSeq.ctx, event)
}

func (dSeq *distributedSequencer) coordinatorEventHandler(event common.Event) {
	log.L(dSeq.ctx).Debugf("[Sequencer] handing off TX-emitted event to sequencer coordinator: %+v", event)
	dSeq.coordinator.HandleEvent(dSeq.ctx, event)
}

type DistributedSequencer interface {
	GetCoordinator() coordinator.SeqCoordinator
	GetSender() sender.SeqSender
	GetTransportWriter() transport.TransportWriter
}

type distributedSequencer struct {
	ctx             context.Context
	sender          sender.SeqSender
	transportWriter transport.TransportWriter
	coordinator     coordinator.SeqCoordinator
	contractAddress string
	lastTXTime      time.Time
	// lastCallTime time.Time // MRW TODO - this isn't really a sequencer-relevant metric?
}

func (dSeq *distributedSequencer) GetCoordinator() coordinator.SeqCoordinator {
	return dSeq.coordinator
}

func (dSeq *distributedSequencer) GetSender() sender.SeqSender {
	return dSeq.sender
}

func (dSeq *distributedSequencer) GetTransportWriter() transport.TransportWriter {
	return dSeq.transportWriter
}

func Log(ctx context.Context, msg string) {
	log.L(ctx).Debugf("[Sequencer] %s", msg)
}

func (dMgr *distributedSequencerManager) LoadSequencer(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (DistributedSequencer, error) {
	var err error
	if domainAPI == nil {
		domainAPI, err = dMgr.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
		if err != nil {
			// Treat as a valid case, let the caller decide if it is or not
			log.L(ctx).Infof("[Sequencer] no sequencer found for contract %s, assuming contract deploy: %s", contractAddr, err)
			return nil, nil
		}
	}

	readlock := true
	dMgr.sequencersLock.RLock()
	defer func() {
		if readlock {
			dMgr.sequencersLock.RUnlock()
		}
	}()
	if dMgr.sequencers[contractAddr.String()] == nil {
		//swap the read lock for a write lock
		dMgr.sequencersLock.RUnlock()
		readlock = false
		dMgr.sequencersLock.Lock()
		defer dMgr.sequencersLock.Unlock()

		//double check in case another goroutine has created the sequencer while we were waiting for the write lock
		if dMgr.sequencers[contractAddr.String()] == nil {
			// log.L(ctx).Debugf("Creating sequencer for contract address %s", contractAddr.String())

			log.L(log.WithComponent(ctx, common.SUBCOMP_MISC)).Debugf("creating sequencer for contract address %s", contractAddr.String())

			// Are we handing this off to the sequencer now?
			// Locally we store mappings of contract address to sender/coordinator pair

			// Do we have space for another sequencer?
			if dMgr.targetActiveSequencersLimit > 0 && len(dMgr.sequencers) > dMgr.targetActiveSequencersLimit {
				log.L(ctx).Debugf("Max concurrent sequencers reached, stopping lowest priority sequencer")
				dMgr.stopLowestPrioritySequencer(ctx)
			}

			dMgr.metrics.SetActiveSequencers(len(dMgr.sequencers))

			if tx == nil {
				//err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "No TX provided to create distributed sequencer")
				//return nil, err
				log.L(ctx).Debugf("No TX provided to create distributed sequencer")
			}

			domainAPI, err := dMgr.components.DomainManager().GetSmartContractByAddress(ctx, dMgr.components.Persistence().NOTX(), contractAddr)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] failed to get domain API for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}
			senderNodePool, err := dMgr.getInitialSenderNodePool(ctx, tx, domainAPI)

			if err != nil {
				log.L(ctx).Errorf("[Sequencer] failed to get transaction committee for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			if domainAPI == nil {
				err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "No domain provided to create distributed sequencer")
				log.L(ctx).Error(err)
				return nil, err
			}
			dCtx := dMgr.components.StateManager().NewDomainContext(dMgr.ctx, domainAPI.Domain(), contractAddr)

			transportWriter := transport.NewTransportWriter(domainAPI.Domain().Name(), &contractAddr, dMgr.nodeName, dMgr.components.TransportManager(), dMgr.HandlePaladinMsg)
			dMgr.engineIntegration = common.NewEngineIntegration(dMgr.ctx, dMgr.components, dMgr.nodeName, domainAPI, dCtx, dMgr, dMgr)

			sequencer := &distributedSequencer{
				ctx:             dMgr.ctx,
				contractAddress: contractAddr.String(),
				transportWriter: transportWriter,
			}

			sender, err := sender.NewSender(dMgr.ctx, dMgr.nodeName, transportWriter, common.RealClock(), sequencer.senderEventHandler, dMgr.engineIntegration, 10, &contractAddr, 15000, 10, dMgr.metrics)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] failed to create distributed sequencer sender for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			// MRW TODO - config these values
			reqTimeout := time.Duration(60) * time.Second
			assembleTimeout := time.Duration(60) * time.Second
			coordinator, err := coordinator.NewCoordinator(dMgr.ctx,
				domainAPI,
				transportWriter,
				senderNodePool,
				common.RealClock(),
				sequencer.coordinatorEventHandler,
				dMgr.engineIntegration,
				reqTimeout,
				assembleTimeout,
				10,
				&contractAddr,
				50,
				4, // MRW TODO - make configurable
				dMgr.nodeName,
				func(ctx context.Context, t *coordTransaction.Transaction) {
					// MRW TODO - move to sequencer module?
					log.L(ctx).Debugf("Transaction %s ready for dispatch", t.ID.String())
					log.L(ctx).Debugf("Call syncpoints to handle submission of transaction %s", t.ID.String())

					domainAPI, err := dMgr.components.DomainManager().GetSmartContractByAddress(ctx, dMgr.components.Persistence().NOTX(), contractAddr)
					if err != nil {
						log.L(ctx).Errorf("Error getting domain API for contract %s: %s", contractAddr.String(), err)
						return
					}

					// MRW TODO - do we need to factor in coordinator selection here?
					// coordinatorSelection := domainAPI.ContractConfig().GetCoordinatorSelection()
					submitterSelection := domainAPI.ContractConfig().GetSubmitterSelection()

					if submitterSelection == prototk.ContractConfig_SUBMITTER_COORDINATOR {
						log.L(ctx).Info("Deciding on public TX signer. Submitter selection is SUBMITTER_COORDINATOR")
						for _, endorsement := range t.PostAssembly.Endorsements {
							log.L(ctx).Infof("Checking endorsement %+v", endorsement)
							for _, constraint := range endorsement.Constraints {
								log.L(ctx).Infof("Checking constraint %+v", constraint)
								if constraint == prototk.AttestationResult_ENDORSER_MUST_SUBMIT {
									t.Signer = endorsement.Verifier.Lookup
									log.L(ctx).Infof("Found constraint ENDORSER_MUST_SUBMIT. Setting signer to %s", t.Signer)
									break
								}
							}
						}
					}
					// MRW TODO - is there an else/if here for ContractConfig_SUBMITTER_SENDER or is this handled by the current default of randomly generate an address?

					// MRW TODO - What is the correct signer to set here?
					if t.Signer == "" {
						log.L(ctx).Infof("Transaction %s has no signer. Allocating random signer", t.ID.String())
						t.Signer = fmt.Sprintf("domains.%s.submit.%s", contractAddr, uuid.New())
					}

					log.L(ctx).Infof("Transaction %s allocated signer %s", t.ID.String(), t.Signer)

					// Log transaction signatures
					for _, signature := range t.PostAssembly.Signatures {
						log.L(ctx).Infof("Transaction %s has signature %+v", t.ID.String(), signature)
					}

					// Log transaction endorsements
					for _, endorsement := range t.PostAssembly.Endorsements {
						log.L(ctx).Infof("Transaction %s has endorsement %+v", t.ID.String(), endorsement)
					}

					log.L(ctx).Infof("Preparing transaction %s which has %d endorsements", t.ID.String(), len(t.PostAssembly.Endorsements))
					// Need to prepate the transaction
					readTX := dMgr.components.Persistence().NOTX() // no DB transaction required here
					log.L(ctx).Infof("Preparing transaction %s", t.ID.String())
					err = domainAPI.PrepareTransaction(dCtx, readTX, t.PrivateTransaction)
					if err != nil {
						log.L(ctx).Errorf("Error preparing transaction %s: %s", t.ID.String(), err)
						return
					}

					log.L(ctx).Infof("Creating dispatch batch transaction %s", t.ID.String())
					dispatchBatch := &syncpoints.DispatchBatch{
						PublicDispatches: make([]*syncpoints.PublicDispatch, 0),
					}

					preparedTxnDistributions := make([]*components.PreparedTransactionWithRefs, 0)

					// MRW TODO - make this a for loop over the complete list of transactions
					preparedTransaction := t.PrivateTransaction
					publicTransactionsToSend := make([]*components.PrivateTransaction, 0)
					sequence := &syncpoints.PublicDispatch{}
					stateDistributions := make([]*components.StateDistribution, 0)
					localStateDistributions := make([]*components.StateDistributionWithData, 0)

					hasPublicTransaction := preparedTransaction.PreparedPublicTransaction != nil
					hasPrivateTransaction := preparedTransaction.PreparedPrivateTransaction != nil
					switch {
					case preparedTransaction.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPublicTransaction && !hasPrivateTransaction:
						log.L(ctx).Infof("Result of transaction %s is a public transaction (gas=%d)", preparedTransaction.ID, *preparedTransaction.PreparedPublicTransaction.PublicTxOptions.Gas)
						publicTransactionsToSend = append(publicTransactionsToSend, preparedTransaction)
						sequence.PrivateTransactionDispatches = append(sequence.PrivateTransactionDispatches, &syncpoints.DispatchPersisted{
							PrivateTransactionID: t.ID.String(),
						})
					case preparedTransaction.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPrivateTransaction && !hasPublicTransaction:
						log.L(ctx).Infof("Result of transaction %s is a chained private transaction", preparedTransaction.ID)
						validatedPrivateTx, err := dMgr.components.TxManager().PrepareChainedPrivateTransaction(ctx, dMgr.components.Persistence().NOTX(), t.PreAssembly.TransactionSpecification.From, t.ID, t.Domain, &contractAddr, preparedTransaction.PreparedPrivateTransaction, pldapi.SubmitModeAuto)
						if err != nil {
							log.L(ctx).Errorf("Error preparing transaction %s: %s", preparedTransaction.ID, err)
							// TODO: this is just an error situation for one transaction - this function is a batch function
							return
						}
						dispatchBatch.PrivateDispatches = append(dispatchBatch.PrivateDispatches, validatedPrivateTx)
					case preparedTransaction.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION && (hasPublicTransaction || hasPrivateTransaction):
						log.L(ctx).Infof("Result of transaction %s is a prepared transaction public=%t private=%t", preparedTransaction.ID, hasPublicTransaction, hasPrivateTransaction)
						preparedTransactionWithRefs := mapPreparedTransaction(preparedTransaction)
						dispatchBatch.PreparedTransactions = append(dispatchBatch.PreparedTransactions, preparedTransactionWithRefs)

						// The prepared transaction needs to end up on the node that is able to submit it.
						preparedTxnDistributions = append(preparedTxnDistributions, preparedTransactionWithRefs)

					default:
						err = i18n.NewError(ctx, msgs.MsgPrivateTxMgrInvalidPrepareOutcome, preparedTransaction.ID, preparedTransaction.Intent, hasPublicTransaction, hasPrivateTransaction)
						log.L(ctx).Errorf("Error preparing transaction %s: %s", preparedTransaction.ID, err)
						// TODO: this is just an error situation for one transaction - this function is a batch function
						return
					}

					stateDistributionBuilder := common.NewStateDistributionBuilder(dMgr.components, t.PrivateTransaction)
					sds, err := stateDistributionBuilder.Build(ctx, t.PrivateTransaction)
					if err != nil {
						log.L(ctx).Errorf("Error getting state distributions: %s", err)
					}

					for _, sd := range sds.Remote {
						log.L(ctx).Infof("Adding remote state distribution %+v", sd.StateDistribution)
						stateDistributions = append(stateDistributions, &sd.StateDistribution)
					}
					localStateDistributions = append(localStateDistributions, sds.Local...)

					//Now we have the payloads, we can prepare the submission
					publicTransactionEngine := dMgr.components.PublicTxManager()

					// we may or may not have any transactions to send depending on the submit mode
					if len(publicTransactionsToSend) == 0 {
						log.L(ctx).Debugf("No public transactions to send for signing address %s", t.Signer)
					} else {

						signers := make([]string, len(publicTransactionsToSend))
						for i, pt := range publicTransactionsToSend {
							unqualifiedSigner, err := pldtypes.PrivateIdentityLocator(pt.Signer).Identity(ctx)
							if err != nil {
								err = i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, err)
								log.L(ctx).Error(err)
								return
							}

							signers[i] = unqualifiedSigner
						}
						keyMgr := dMgr.components.KeyManager()
						resolvedAddrs, err := keyMgr.ResolveEthAddressBatchNewDatabaseTX(ctx, signers)
						if err != nil {
							log.L(ctx).Errorf("Failed to resolve signers for public transactions: %s", err)
							return
						}

						publicTXs := make([]*components.PublicTxSubmission, len(publicTransactionsToSend))
						for i, pt := range publicTransactionsToSend {
							log.L(ctx).Debugf("DispatchTransactions: creating PublicTxSubmission from %s", pt.Signer)
							publicTXs[i] = &components.PublicTxSubmission{
								Bindings: []*components.PaladinTXReference{{TransactionID: pt.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
								PublicTxInput: pldapi.PublicTxInput{
									From:            resolvedAddrs[i],
									To:              &contractAddr,
									PublicTxOptions: pt.PreparedPublicTransaction.PublicTxOptions,
								},
							}

							// TODO: This aligning with submission in public Tx manage
							data, err := pt.PreparedPublicTransaction.ABI[0].EncodeCallDataJSONCtx(ctx, pt.PreparedPublicTransaction.Data)
							if err != nil {
								log.L(ctx).Errorf("Failed to encode call data for public transaction %s: %s", pt.ID, err)
								return
							}
							publicTXs[i].Data = pldtypes.HexBytes(data)

							log.L(ctx).Infof("Validating public transaction %s", pt.ID.String())
							err = publicTransactionEngine.ValidateTransaction(ctx, dMgr.components.Persistence().NOTX(), publicTXs[i])
							if err != nil {
								log.L(ctx).Errorf("Failed to encode call data for public transaction %s: %s", pt.ID, err)
								return
							}
						}
						sequence.PublicTxs = publicTXs
						dispatchBatch.PublicDispatches = append(dispatchBatch.PublicDispatches, sequence)

					}

					// Determine if there are any local nullifiers that need to be built and put into the domain context
					// before we persist the dispatch batch
					log.L(ctx).Infof("Building nullifiers for local state distributions (%d)", len(localStateDistributions))
					localNullifiers, err := dMgr.BuildNullifiers(ctx, localStateDistributions)
					if err == nil && len(localNullifiers) > 0 {
						err = dCtx.UpsertNullifiers(localNullifiers...)
					}
					if err != nil {
						log.L(ctx).Errorf("Error building nullifiers: %s", err)
						return
					}

					log.L(ctx).Infof("Persisting & deploying batch. %d public transactions, %d private transactions, %d prepared transactions", len(dispatchBatch.PublicDispatches), len(dispatchBatch.PrivateDispatches), len(dispatchBatch.PreparedTransactions))
					err = dMgr.syncPoints.PersistDispatchBatch(dCtx, contractAddr, dispatchBatch, stateDistributions, preparedTxnDistributions)
					if err != nil {
						log.L(ctx).Errorf("Error persisting batch: %s", err)
						return
					}

					err = transportWriter.SendDispatched(ctx, t.Sender(), uuid.New(), t.PreAssembly.TransactionSpecification)
					if err != nil {
						log.L(ctx).Errorf("Failed to send dispatched event for transaction %s: %s", t.ID, err)
						return
					}

					// We also need to trigger ourselves for any private TX we chained
					if len(dispatchBatch.PrivateDispatches) > 0 {
						log.L(ctx).Infof("[Sequencer] Chaining %d private transactions", len(dispatchBatch.PrivateDispatches))
						if err := dMgr.components.TxManager().ChainPrivateTransactions(ctx, dMgr.components.Persistence().NOTX(), dispatchBatch.PrivateDispatches); err != nil {
							log.L(ctx).Errorf("Sequencer failed to notify private TX manager for chained transaction")
						}
					}
				},
				func(contractAddress *pldtypes.EthAddress, coordinatorNode string) {
					// A new coordinator started, it might be us or it might be another node.
					// Update metrics and check if we need to stop one to stay within the configured max active coordinators
					dMgr.updateActiveCoordinators(dMgr.ctx)

					// The sender needs to know where to delegate transactions to
					sender.SetActiveCoordinator(dMgr.ctx, coordinatorNode)
				},
				func(contractAddress *pldtypes.EthAddress) {
					// A new coordinator became idle, update metrics
					dMgr.updateActiveCoordinators(dMgr.ctx)
				},
				dMgr.metrics,
			)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] failed to create distributed sequencer coordinator for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			// Until anything changes, start by setting the sender's delegate to the initial choice of coordinator
			sender.SetActiveCoordinator(dMgr.ctx, coordinator.GetActiveCoordinatorNode(dMgr.ctx))

			log.L(log.WithComponent(ctx, common.SUBCOMP_LIFECYCLE)).Debugf("created  | %s", contractAddr.String())
			//log.L(ctx).Infof("[SeqLifecycle] | created | %s", contractAddr.String())
			sequencer.sender = sender
			sequencer.coordinator = coordinator
			dMgr.sequencers[contractAddr.String()] = sequencer

			if tx != nil {
				dMgr.sequencers[contractAddr.String()].lastTXTime = time.Now()
			}
			return sequencer, nil
		}
	}

	if tx != nil {
		dMgr.sequencers[contractAddr.String()].lastTXTime = time.Now()
	}

	return dMgr.sequencers[contractAddr.String()], nil
}

// Must be called within the sequencer's write lock
func (dMgr *distributedSequencerManager) stopLowestPrioritySequencer(ctx context.Context) {
	log.L(ctx).Debugf("[Sequencer] max concurrent sequencers reached, stopping lowest priority sequencer")

	// If any sequencers are already closing we can wait for them to close instead of stopping a different one
	for _, sequencer := range dMgr.sequencers {
		if sequencer.coordinator.GetCurrentState() == coordinator.State_Flush ||
			sequencer.coordinator.GetCurrentState() == coordinator.State_Closing {

			// To avoid blocking the start of new sequencer that has caused us to purge the lowest priority one,
			// we don't wait for the closing ones to complete. The aim is to allow the node to remain stable while
			// still being responsive to new contract activity so a closing sequencer is allowed to page out in its
			// own time.
			log.L(ctx).Debugf("[Sequencer] coordinator %s is closing, waiting for it to close", sequencer.contractAddress)
			return
		} else if sequencer.coordinator.GetCurrentState() == coordinator.State_Idle ||
			sequencer.coordinator.GetCurrentState() == coordinator.State_Observing {
			// This sequencer is already idle or observing so we can page it out immediately

			log.L(log.WithComponent(ctx, common.SUBCOMP_LIFECYCLE)).Debugf("unloading | %s", sequencer.contractAddress)
			sequencer.sender.Stop()
			delete(dMgr.sequencers, sequencer.contractAddress)
			return
		}
	}

	// Order existing sequencers by LRU time
	sequencers := make([]*distributedSequencer, 0)
	for _, sequencer := range dMgr.sequencers {
		sequencers = append(sequencers, sequencer)
	}
	sort.Slice(sequencers, func(i, j int) bool {
		return sequencers[i].lastTXTime.Before(sequencers[j].lastTXTime)
	})

	// Stop the lowest priority sequencer by emitting an event and waiting for it to move to closed
	sequencers[0].coordinator.Stop()
	sequencers[0].sender.Stop()
	log.L(log.WithComponent(ctx, common.SUBCOMP_LIFECYCLE)).Debugf("unloading | %s", sequencers[0].contractAddress)
	delete(dMgr.sequencers, sequencers[0].contractAddress)
}

func (dMgr *distributedSequencerManager) updateActiveCoordinators(ctx context.Context) {
	// log.L(ctx).Debugf("[Sequencer] checking number of concurrent coordinators")
	log.L(log.WithComponent(ctx, common.SUBCOMP_MISC)).Debugf("checking number of concurrent coordinators")

	readlock := true
	dMgr.sequencersLock.RLock()
	defer func() {
		if readlock {
			dMgr.sequencersLock.RUnlock()
		}
	}()

	activeCoordinators := 0
	// If any sequencers are already closing we can wait for them to close instead of stopping a different one
	for _, sequencer := range dMgr.sequencers {
		log.L(ctx).Debugf("[Sequencer] coordinator %s state %s", sequencer.contractAddress, sequencer.coordinator.GetCurrentState())
		if sequencer.coordinator.GetCurrentState() == coordinator.State_Active {
			log.L(ctx).Debugf("[Sequencer] coordinator %s is active", sequencer.contractAddress)
			activeCoordinators++
		}
	}

	dMgr.metrics.SetActiveCoordinators(activeCoordinators)

	log.L(ctx).Debugf("[Sequencer] %d coordinators currently active", activeCoordinators)

	if activeCoordinators >= dMgr.targetActiveCoordinatorsLimit {
		log.L(ctx).Debugf("[Sequencer] max concurrent coordinators reached, asking the lowest priority coordinator to hand over to another node")
		// Order existing sequencers by LRU time
		sequencers := make([]*distributedSequencer, 0)
		for _, sequencer := range dMgr.sequencers {
			sequencers = append(sequencers, sequencer)
		}
		sort.Slice(sequencers, func(i, j int) bool {
			return sequencers[i].lastTXTime.Before(sequencers[j].lastTXTime)
		})

		// Stop the lowest priority coordinator by emitting an event asking it to handover to another coordinator
		sequencers[0].coordinator.Stop()
	}
}
