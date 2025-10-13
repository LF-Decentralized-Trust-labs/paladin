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
	"strings"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
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

// Components needing to interact with the sequencer can make certain calls into
// the coordinator, the originator, or the transport writer
type Sequencer interface {
	GetCoordinator() coordinator.SeqCoordinator
	GetSender() sender.SeqSender
	GetTransportWriter() transport.TransportWriter
}

func (seq *sequencer) GetCoordinator() coordinator.SeqCoordinator {
	return seq.coordinator
}

func (seq *sequencer) GetSender() sender.SeqSender {
	return seq.sender
}

func (seq *sequencer) GetTransportWriter() transport.TransportWriter {
	return seq.transportWriter
}

// An instance of a sequencer (one instance per domain contract)
type sequencer struct {
	ctx context.Context

	// The 3 main components of the sequencer
	sender          sender.SeqSender
	transportWriter transport.TransportWriter
	coordinator     coordinator.SeqCoordinator

	// Sequencer attributes
	contractAddress string
	lastTXTime      time.Time

	// Channels for passing events into the originator and coordinator event loops respectively
	senderEvents      chan common.Event
	coordinatorEvents chan common.Event
	closeEventHandler context.CancelFunc
}

// Return the sequencer for the requested contract address, instantiating it first if this is its first use
func (sMgr *sequencerManager) LoadSequencer(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (Sequencer, error) {
	var err error
	if domainAPI == nil {
		domainAPI, err = sMgr.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
		if err != nil {
			// Treat as a valid case, let the caller decide if it is or not
			log.L(ctx).Infof("no sequencer found for contract %s, assuming contract deploy: %s", contractAddr, err)
			return nil, nil
		}
	}

	readlock := true
	sMgr.sequencersLock.RLock()
	defer func() {
		if readlock {
			sMgr.sequencersLock.RUnlock()
		}
	}()
	if sMgr.sequencers[contractAddr.String()] == nil {
		//swap the read lock for a write lock
		sMgr.sequencersLock.RUnlock()
		readlock = false
		sMgr.sequencersLock.Lock()
		defer sMgr.sequencersLock.Unlock()

		//double check in case another goroutine has created the sequencer while we were waiting for the write lock
		if sMgr.sequencers[contractAddr.String()] == nil {
			// log.L(ctx).Debugf("Creating sequencer for contract address %s", contractAddr.String())

			log.L(log.WithComponent(ctx, common.SUBCOMP_MISC)).Debugf("creating sequencer for contract address %s", contractAddr.String())

			// Are we handing this off to the sequencer now?
			// Locally we store mappings of contract address to sender/coordinator pair

			// Do we have space for another sequencer?
			if sMgr.targetActiveSequencersLimit > 0 && len(sMgr.sequencers) > sMgr.targetActiveSequencersLimit {
				log.L(ctx).Debugf("Max concurrent sequencers reached, stopping lowest priority sequencer")
				sMgr.stopLowestPrioritySequencer(ctx)
			}
			sMgr.metrics.SetActiveSequencers(len(sMgr.sequencers))

			if tx == nil {
				//err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "No TX provided to create distributed sequencer")
				//return nil, err
				log.L(ctx).Debugf("No TX provided to create sequencer")
			}

			domainAPI, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, sMgr.components.Persistence().NOTX(), contractAddr)
			if err != nil {
				log.L(ctx).Errorf("failed to get domain API for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			if domainAPI == nil {
				err := i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "No domain provided to create sequencer")
				log.L(ctx).Error(err)
				return nil, err
			}

			senderNodePool, err := sMgr.getInitialSenderNodePool(ctx, tx, domainAPI)
			if err != nil {
				log.L(ctx).Errorf("failed to get transaction sender node pool for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			// Create a new domain context for the sequencer. This will be re-used for the lifetime of the sequencer
			dCtx := sMgr.components.StateManager().NewDomainContext(sMgr.ctx, domainAPI.Domain(), contractAddr)

			// Create a transport writer for the sequencer to communicate with sequencers on other peers
			transportWriter := transport.NewTransportWriter(&contractAddr, sMgr.nodeName, sMgr.components.TransportManager(), sMgr.HandlePaladinMsg)

			sMgr.engineIntegration = common.NewEngineIntegration(sMgr.ctx, sMgr.components, sMgr.nodeName, domainAPI, dCtx, sMgr, sMgr)

			sequencer := &sequencer{
				ctx:             sMgr.ctx,
				contractAddress: contractAddr.String(),
				transportWriter: transportWriter,
			}

			sender, err := sender.NewSender(sMgr.ctx, sMgr.nodeName, transportWriter, common.RealClock(), sMgr.engineIntegration, 10, &contractAddr, 15000, 10, sMgr.metrics)
			if err != nil {
				log.L(ctx).Errorf("failed to create sequencer sender for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			coordinator, err := coordinator.NewCoordinator(sMgr.ctx,
				domainAPI,
				transportWriter,
				senderNodePool,
				common.RealClock(),
				sMgr.engineIntegration,
				sMgr.syncPoints,
				confutil.DurationMin(sMgr.config.RequestTimeout, pldconf.SequencerMinimum.RequestTimeout, *pldconf.SequencerDefaults.RequestTimeout),
				confutil.DurationMin(sMgr.config.AssembleTimeout, pldconf.SequencerMinimum.AssembleTimeout, *pldconf.SequencerDefaults.AssembleTimeout),
				10,
				&contractAddr,
				confutil.Uint64Min(sMgr.config.BlockHeightTolerance, pldconf.SequencerMinimum.BlockHeightTolerance, *pldconf.SequencerDefaults.BlockHeightTolerance),
				confutil.IntMin(sMgr.config.ClosingGracePeriod, pldconf.SequencerMinimum.ClosingGracePeriod, *pldconf.SequencerDefaults.ClosingGracePeriod),
				sMgr.nodeName,
				func(ctx context.Context, t *coordTransaction.Transaction) {
					// MRW TODO - move to sequencer module?
					log.L(ctx).Debugf("Transaction %s ready for dispatch", t.ID.String())
					log.L(ctx).Debugf("Call syncpoints to handle submission of transaction %s", t.ID.String())

					domainAPI, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, sMgr.components.Persistence().NOTX(), contractAddr)
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
					// Need to prepare the transaction
					readTX := sMgr.components.Persistence().NOTX() // no DB transaction required here
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
						log.L(ctx).Infof("Result of transaction %s is a public transaction (gas=%d)", preparedTransaction.ID, *preparedTransaction.PreparedPublicTransaction.Gas)
						publicTransactionsToSend = append(publicTransactionsToSend, preparedTransaction)
						sequence.PrivateTransactionDispatches = append(sequence.PrivateTransactionDispatches, &syncpoints.DispatchPersisted{
							PrivateTransactionID: t.ID.String(),
						})
					case preparedTransaction.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPrivateTransaction && !hasPublicTransaction:
						log.L(ctx).Infof("Result of transaction %s is a chained private transaction", preparedTransaction.ID)
						validatedPrivateTx, err := sMgr.components.TxManager().PrepareChainedPrivateTransaction(ctx, sMgr.components.Persistence().NOTX(), t.PreAssembly.TransactionSpecification.From, t.ID, t.Domain, &contractAddr, preparedTransaction.PreparedPrivateTransaction, pldapi.SubmitModeAuto)
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

					stateDistributionBuilder := common.NewStateDistributionBuilder(sMgr.components, t.PrivateTransaction)
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
					publicTransactionEngine := sMgr.components.PublicTxManager()

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
						keyMgr := sMgr.components.KeyManager()
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
							err = publicTransactionEngine.ValidateTransaction(ctx, sMgr.components.Persistence().NOTX(), publicTXs[i])
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
					localNullifiers, err := sMgr.BuildNullifiers(ctx, localStateDistributions)
					if err == nil && len(localNullifiers) > 0 {
						err = dCtx.UpsertNullifiers(localNullifiers...)
					}
					if err != nil {
						log.L(ctx).Errorf("Error building nullifiers: %s", err)
						return
					}

					log.L(ctx).Infof("Persisting & deploying batch. %d public transactions, %d private transactions, %d prepared transactions", len(dispatchBatch.PublicDispatches), len(dispatchBatch.PrivateDispatches), len(dispatchBatch.PreparedTransactions))
					err = sMgr.syncPoints.PersistDispatchBatch(dCtx, contractAddr, dispatchBatch, stateDistributions, preparedTxnDistributions)
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
					for _, dispatch := range dispatchBatch.PrivateDispatches {
						// Create a new DB transaction and handle the new transaction
						sMgr.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTx persistence.DBTX) error {
							return sMgr.HandleNewTx(ctx, dbTx, dispatch.NewTransaction)
						})
					}
					log.L(ctx).Tracef("[Sequencer] Chained %d private transactions", len(dispatchBatch.PrivateDispatches))
				},
				func(contractAddress *pldtypes.EthAddress, coordinatorNode string) {
					// A new coordinator started, it might be us or it might be another node.
					// Update metrics and check if we need to stop one to stay within the configured max active coordinators
					sMgr.updateActiveCoordinators(sMgr.ctx)

					// The sender needs to know where to delegate transactions to
					err := sender.SetActiveCoordinator(sMgr.ctx, coordinatorNode)
					if err != nil {
						log.L(ctx).Errorf("[Sequencer] failed to set active coordinator for contract %s: %s", contractAddr.String(), err)
						return
					}
				},
				func(contractAddress *pldtypes.EthAddress) {
					// A new coordinator became idle, update metrics
					sMgr.updateActiveCoordinators(sMgr.ctx)
				},
				sMgr.metrics,
			)
			if err != nil {
				log.L(ctx).Errorf("[Sequencer] failed to create sequencer coordinator for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			// Start by populating the pool of originators with the endorsers of this transaction. At this point
			// we don't have anything else to use to determine who our candidate coordinators are.
			if tx != nil && tx.PreAssembly != nil && tx.PreAssembly.RequiredVerifiers != nil {
				for _, verifiers := range tx.PreAssembly.RequiredVerifiers {
					if strings.Contains(verifiers.Lookup, "@") {
						parts := strings.Split(verifiers.Lookup, "@")
						coordinator.UpdateSenderNodePool(ctx, parts[1])
					}
				}

				// Get the best candidate for an initial coordinator, and use as the delegate for any originated transactions
				err := sender.SetActiveCoordinator(sMgr.ctx, coordinator.GetActiveCoordinatorNode(sMgr.ctx))
				if err != nil {
					log.L(ctx).Errorf("failed to set active coordinator for contract %s: %s", contractAddr.String(), err)
				}
			}

			sequencer.sender = sender
			sequencer.coordinator = coordinator
			sMgr.sequencers[contractAddr.String()] = sequencer

			if tx != nil {
				sMgr.sequencers[contractAddr.String()].lastTXTime = time.Now()
			}

			log.L(log.WithComponent(ctx, common.SUBCOMP_LIFECYCLE)).Debugf("created  | %s", contractAddr.String())
		}
	} else {
		// MRW TODO - move to a common function
		// We already have a sequencer initialized but we might not have an initial coordinator selected
		// Start by populating the pool of originators with the endorsers of this transaction. At this point
		// we don't have anything else to use to determine who our candidate coordinators are.
		if tx != nil && tx.PreAssembly != nil && tx.PreAssembly.RequiredVerifiers != nil {
			for _, verifiers := range tx.PreAssembly.RequiredVerifiers {
				if strings.Contains(verifiers.Lookup, "@") {
					parts := strings.Split(verifiers.Lookup, "@")
					sMgr.sequencers[contractAddr.String()].GetCoordinator().UpdateSenderNodePool(ctx, parts[1])
				}
			}

			// Get the best candidate for an initial coordinator, and use as the delegate for any originated transactions
			sMgr.sequencers[contractAddr.String()].GetSender().SetActiveCoordinator(sMgr.ctx, sMgr.sequencers[contractAddr.String()].GetCoordinator().GetActiveCoordinatorNode(sMgr.ctx))
		}
	}

	if tx != nil {
		sMgr.sequencers[contractAddr.String()].lastTXTime = time.Now()
	}

	return sMgr.sequencers[contractAddr.String()], nil
}

// Must be called within the sequencer's write lock
func (sMgr *sequencerManager) stopLowestPrioritySequencer(ctx context.Context) {
	log.L(ctx).Debugf("max concurrent sequencers reached, stopping lowest priority sequencer")
	if len(sMgr.sequencers) != 0 {
		// If any sequencers are already closing we can wait for them to close instead of stopping a different one
		for _, sequencer := range sMgr.sequencers {
			if sequencer.coordinator.GetCurrentState() == coordinator.State_Flush ||
				sequencer.coordinator.GetCurrentState() == coordinator.State_Closing {

				// To avoid blocking the start of new sequencer that has caused us to purge the lowest priority one,
				// we don't wait for the closing ones to complete. The aim is to allow the node to remain stable while
				// still being responsive to new contract activity so a closing sequencer is allowed to page out in its
				// own time.
				log.L(ctx).Debugf("coordinator %s is closing, waiting for it to close", sequencer.contractAddress)
				return
			} else if sequencer.coordinator.GetCurrentState() == coordinator.State_Idle ||
				sequencer.coordinator.GetCurrentState() == coordinator.State_Observing {
				// This sequencer is already idle or observing so we can page it out immediately

				log.L(log.WithComponent(ctx, common.SUBCOMP_LIFECYCLE)).Debugf("unloading | %s", sequencer.contractAddress)
				sequencer.sender.Stop()
				delete(sMgr.sequencers, sequencer.contractAddress)
				return
			}
		}

		// Order existing sequencers by LRU time
		sequencers := make([]*sequencer, 0)
		for _, sequencer := range sMgr.sequencers {
			sequencers = append(sequencers, sequencer)
		}
		sort.Slice(sequencers, func(i, j int) bool {
			return sequencers[i].lastTXTime.Before(sequencers[j].lastTXTime)
		})

		// Stop the lowest priority sequencer by emitting an event and waiting for it to move to closed
		sequencers[0].coordinator.Stop()
		sequencers[0].sender.Stop()
		log.L(log.WithComponent(ctx, common.SUBCOMP_LIFECYCLE)).Debugf("unloading | %s", sequencers[0].contractAddress)
		delete(sMgr.sequencers, sequencers[0].contractAddress)
	}
}

func (sMgr *sequencerManager) updateActiveCoordinators(ctx context.Context) {
	// log.L(ctx).Debugf("[Sequencer] checking number of concurrent coordinators")
	log.L(log.WithComponent(ctx, common.SUBCOMP_MISC)).Debugf("checking number of concurrent coordinators")

	readlock := true
	sMgr.sequencersLock.RLock()
	defer func() {
		if readlock {
			sMgr.sequencersLock.RUnlock()
		}
	}()

	activeCoordinators := 0
	// If any sequencers are already closing we can wait for them to close instead of stopping a different one
	for _, sequencer := range sMgr.sequencers {
		log.L(ctx).Debugf("coordinator %s state %s", sequencer.contractAddress, sequencer.coordinator.GetCurrentState())
		if sequencer.coordinator.GetCurrentState() == coordinator.State_Active {
			log.L(ctx).Debugf("coordinator %s is active", sequencer.contractAddress)
			activeCoordinators++
		}
	}

	sMgr.metrics.SetActiveCoordinators(activeCoordinators)

	log.L(ctx).Debugf("%d coordinators currently active", activeCoordinators)

	if activeCoordinators >= sMgr.targetActiveCoordinatorsLimit {
		log.L(ctx).Debugf("max concurrent coordinators reached, asking the lowest priority coordinator to hand over to another node")
		// Order existing sequencers by LRU time
		sequencers := make([]*sequencer, 0)
		for _, sequencer := range sMgr.sequencers {
			sequencers = append(sequencers, sequencer)
		}
		sort.Slice(sequencers, func(i, j int) bool {
			return sequencers[i].lastTXTime.Before(sequencers[j].lastTXTime)
		})

		// Stop the lowest priority coordinator by emitting an event asking it to handover to another coordinator
		sequencers[0].coordinator.Stop()
		log.L(log.WithComponent(ctx, common.SUBCOMP_LIFECYCLE)).Debugf("unloading | %s", sequencers[0].contractAddress)
		delete(sMgr.sequencers, sequencers[0].contractAddress)
	}
}
