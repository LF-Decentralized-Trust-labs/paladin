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

package noto

import (
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

func (n *Noto) HandleEventBatch(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
	var res prototk.HandleEventBatchResponse

	var variant pldtypes.HexUint64
	var domainConfig types.NotoParsedConfig
	if err := json.Unmarshal([]byte(req.ContractInfo.GetContractConfigJson()), &domainConfig); err == nil {
		if domainConfig.Variant != 0 {
			variant = domainConfig.Variant
		}
	}

	for _, ev := range req.Events {
		if variant == types.NotoVariantDefault {
			if err := n.handleV1Event(ctx, ev, &res, req); err != nil {
				log.L(ctx).Warnf("Error handling V1 event: %s", err)
				return nil, err
			}
		} else {
			if err := n.handleV0Event(ctx, ev, &res, req); err != nil {
				log.L(ctx).Warnf("Error handling V0 event: %s", err)
				return nil, err
			}
		}
	}
	return &res, nil
}

func (n *Noto) handleV0Event(ctx context.Context, ev *prototk.OnChainEvent, res *prototk.HandleEventBatchResponse, req *prototk.HandleEventBatchRequest) error {
	switch ev.SoliditySignature {
	case eventSignaturesV0[EventNotoTransfer]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var transfer NotoTransfer_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
			txData, err := n.decodeTransactionData(ctx, transfer.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, transfer.TxId, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(transfer.TxId, transfer.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(transfer.TxId, transfer.Outputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoTransfer event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[EventNotoLock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lock NotoLock_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
			txData, err := n.decodeTransactionData(ctx, lock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lock.TxId, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lock.TxId, lock.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.Outputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.LockedOutputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[EventNotoUnlock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlock NotoUnlock_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlock); err == nil {
			txData, err := n.decodeTransactionData(ctx, unlock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, unlock.TxId, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(unlock.TxId, unlock.LockedInputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlock.TxId, unlock.LockedOutputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlock.TxId, unlock.Outputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[EventNotoUnlockPrepared]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlockPrepared NotoUnlockPrepared_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlockPrepared); err == nil {
			txData, err := n.decodeTransactionData(ctx, unlockPrepared.Data)
			if err != nil {
				return err
			}
			// Transaction ID is not available in the event data, so we must
			// decode it from the data field
			n.recordTransactionInfo(ev, txData.TransactionID, txData, res)
			res.ReadStates = append(res.ReadStates, n.parseStatesFromEvent(txData.TransactionID, unlockPrepared.LockedInputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlockPrepared event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[EventNotoLockDelegated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockDelegated NotoLockDelegated_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockDelegated); err == nil {
			txData, err := n.decodeTransactionData(ctx, lockDelegated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lockDelegated.TxId, txData, res)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLockDelegated event in batch %s: %s", req.BatchId, err)
		}
	}
	return nil
}

func (n *Noto) handleV1Event(ctx context.Context, ev *prototk.OnChainEvent, res *prototk.HandleEventBatchResponse, req *prototk.HandleEventBatchRequest) error {
	switch ev.SoliditySignature {
	case eventSignatures[EventTransfer]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var transfer NotoTransfer_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
			txData, err := n.decodeTransactionData(ctx, transfer.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, transfer.TxId, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(transfer.TxId, transfer.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(transfer.TxId, transfer.Outputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoTransfer event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[EventLockCreated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockCreated NotoLockCreated_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockCreated); err == nil {
			txData, err := n.decodeTransactionData(ctx, lockCreated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lockCreated.TxId, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lockCreated.TxId, lockCreated.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lockCreated.TxId, lockCreated.Outputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lockCreated.TxId, lockCreated.LockedOutputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed LockCreated event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[EventLockUpdated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockUpdated NotoLockUpdated_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockUpdated); err == nil {
			txData, err := n.decodeTransactionData(ctx, lockUpdated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lockUpdated.TxId, txData, res)
			res.ReadStates = append(res.ReadStates, n.parseStatesFromEvent(lockUpdated.TxId, lockUpdated.LockedInputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed LockUpdated event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[EventLockSpent]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockSpent NotoLockSpent_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockSpent); err == nil {
			unlockDataValue, err := UnlockDataABI.DecodeABIDataCtx(ctx, lockSpent.Data, 0)
			if err != nil {
				log.L(ctx).Warnf("Ignoring LockSpent event with malformed UnlockData in batch %s: %s", req.BatchId, err)
				break
			}
			unlockDataJSON, err := unlockDataValue.JSON()
			if err != nil {
				log.L(ctx).Warnf("Ignoring LockSpent event with malformed UnlockData in batch %s: %s", req.BatchId, err)
				break
			}
			var unlockData UnlockData
			if err := json.Unmarshal(unlockDataJSON, &unlockData); err != nil {
				log.L(ctx).Warnf("Ignoring LockSpent event with malformed UnlockData in batch %s: %s", req.BatchId, err)
				break
			}
			txData, err := n.decodeTransactionData(ctx, unlockData.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, unlockData.TxId, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(unlockData.TxId, unlockData.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlockData.TxId, unlockData.Outputs)...)

			if req.ContractInfo != nil {
				var domainConfig *types.NotoParsedConfig
				err = json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
				if err != nil {
					return err
				}
				if domainConfig.IsNotary &&
					domainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
					!domainConfig.Options.Hooks.PublicAddress.Equals(lockSpent.Spender) {
					err = n.handleNotaryPrivateUnlock(ctx, req.StateQueryContext, domainConfig, lockSpent.LockID, lockSpent.Spender, &unlockData)
					if err != nil {
						// Should all errors cause retry?
						log.L(ctx).Errorf("Failed to handle NotoLockSpent event in batch %s: %s", req.BatchId, err)
						return err
					}
				}
			}
		} else {
			log.L(ctx).Warnf("Ignoring malformed LockSpent event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[EventLockCancelled]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockCancelled NotoLockCancelled_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockCancelled); err == nil {
			unlockDataValue, err := UnlockDataABI.DecodeABIDataCtx(ctx, lockCancelled.Data, 0)
			if err != nil {
				log.L(ctx).Warnf("Ignoring LockCancelled event with malformed UnlockData in batch %s: %s", req.BatchId, err)
				break
			}
			unlockDataJSON, err := unlockDataValue.JSON()
			if err != nil {
				log.L(ctx).Warnf("Ignoring LockCancelled event with malformed UnlockData in batch %s: %s", req.BatchId, err)
				break
			}
			var unlockData UnlockData
			if err := json.Unmarshal(unlockDataJSON, &unlockData); err != nil {
				log.L(ctx).Warnf("Ignoring LockCancelled event with malformed UnlockData in batch %s: %s", req.BatchId, err)
				break
			}
			txData, err := n.decodeTransactionData(ctx, unlockData.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, unlockData.TxId, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(unlockData.TxId, unlockData.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlockData.TxId, unlockData.Outputs)...)

			if req.ContractInfo != nil {
				var domainConfig *types.NotoParsedConfig
				err = json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
				if err != nil {
					return err
				}
				if domainConfig.IsNotary &&
					domainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
					!domainConfig.Options.Hooks.PublicAddress.Equals(lockCancelled.Spender) {
					err = n.handleNotaryPrivateUnlock(ctx, req.StateQueryContext, domainConfig, lockCancelled.LockID, lockCancelled.Spender, &unlockData)
					if err != nil {
						// Should all errors cause retry?
						log.L(ctx).Errorf("Failed to handle NotoLockCancelled event in batch %s: %s", req.BatchId, err)
						return err
					}
				}
			}
		} else {
			log.L(ctx).Warnf("Ignoring malformed LockCancelled event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[EventLockDelegated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockDelegated NotoLockDelegated_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockDelegated); err == nil {
			delegateLockDataValue, err := DelegateLockDataABI.DecodeABIDataCtx(ctx, lockDelegated.Data, 0)
			if err != nil {
				log.L(ctx).Warnf("Ignoring LockDelegated event with malformed DelegateLockData in batch %s: %s", req.BatchId, err)
				break
			}
			delegateLockDataJSON, err := delegateLockDataValue.JSON()
			if err != nil {
				log.L(ctx).Warnf("Ignoring LockDelegated event with malformed DelegateLockData in batch %s: %s", req.BatchId, err)
				break
			}
			var delegateLockData DelegateLockData
			if err := json.Unmarshal(delegateLockDataJSON, &delegateLockData); err != nil {
				log.L(ctx).Warnf("Ignoring LockDelegated event with malformed DelegateLockData in batch %s: %s", req.BatchId, err)
				break
			}
			txData, err := n.decodeTransactionData(ctx, delegateLockData.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, delegateLockData.TxId, txData, res)
		} else {
			log.L(ctx).Warnf("Ignoring malformed LockDelegated event in batch %s: %s", req.BatchId, err)
		}
	}
	return nil
}

// When notary logic is implemented via Pente, unlock events from the base ledger must be propagated
// back to the Pente hooks
// TODO: this method should not be invoked directly on the event loop, but rather via a queue
func (n *Noto) handleNotaryPrivateUnlock(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, lockID pldtypes.Bytes32, spender *pldtypes.EthAddress, unlock *UnlockData) error {
	lockedInputs := make([]string, len(unlock.Inputs))
	for i, input := range unlock.Inputs {
		lockedInputs[i] = input.String()
	}
	unlockedOutputs := make([]string, len(unlock.Outputs))
	for i, output := range unlock.Outputs {
		unlockedOutputs[i] = output.String()
	}

	inputStates, err := n.getStates(ctx, stateQueryContext, n.lockedCoinSchema.Id, lockedInputs)
	if err != nil {
		return err
	}
	if len(inputStates) != len(lockedInputs) {
		return i18n.NewError(ctx, msgs.MsgMissingStateData, unlock.Inputs)
	}

	outputStates, err := n.getStates(ctx, stateQueryContext, n.coinSchema.Id, unlockedOutputs)
	if err != nil {
		return err
	}
	if len(outputStates) != len(unlock.Outputs) {
		return i18n.NewError(ctx, msgs.MsgMissingStateData, unlock.Outputs)
	}

	recipients := make([]*ResolvedUnlockRecipient, len(outputStates))
	for i, state := range outputStates {
		coin, err := n.unmarshalCoin(state.DataJson)
		if err != nil {
			return err
		}
		recipients[i] = &ResolvedUnlockRecipient{
			To:     coin.Owner,
			Amount: coin.Amount,
		}
	}

	transactionType, functionABI, paramsJSON, err := n.wrapHookTransaction(
		domainConfig,
		solutils.MustLoadBuild(notoHooksJSON).ABI.Functions()["handleDelegateUnlock"],
		&DelegateUnlockHookParams{
			Sender:     spender,
			LockID:     lockID,
			Recipients: recipients,
			Data:       unlock.Data,
		},
	)
	if err != nil {
		return err
	}
	functionABIJSON, err := json.Marshal(functionABI)
	if err != nil {
		return err
	}

	_, err = n.Callbacks.SendTransaction(ctx, &prototk.SendTransactionRequest{
		StateQueryContext: stateQueryContext,
		Transaction: &prototk.TransactionInput{
			Type:            mapSendTransactionType(transactionType),
			From:            domainConfig.NotaryLookup,
			ContractAddress: domainConfig.Options.Hooks.PublicAddress.String(),
			FunctionAbiJson: string(functionABIJSON),
			ParamsJson:      string(paramsJSON),
		},
	})
	return err
}

func (n *Noto) parseStatesFromEvent(txID pldtypes.Bytes32, states []pldtypes.Bytes32) []*prototk.StateUpdate {
	refs := make([]*prototk.StateUpdate, len(states))
	for i, state := range states {
		refs[i] = &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txID.String(),
		}
	}
	return refs
}

// We accept a V0 struct here, but do not assume that the transaction ID is present (it is not for V1)
func (n *Noto) recordTransactionInfo(ev *prototk.OnChainEvent, txId pldtypes.Bytes32, txData *types.NotoTransactionData_V0, res *prototk.HandleEventBatchResponse) {
	res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
		TransactionId: txId.String(),
		Location:      ev.Location,
	})
	for _, state := range txData.InfoStates {
		res.InfoStates = append(res.InfoStates, &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txId.String(),
		})
	}
}
