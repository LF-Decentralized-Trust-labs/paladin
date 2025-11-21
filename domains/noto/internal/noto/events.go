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
	case eventSignaturesV0[NotoTransfer]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var transfer NotoTransfer_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
			txData, err := n.decodeTransactionData(ctx, transfer.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(txData.TransactionID, transfer.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txData.TransactionID, transfer.Outputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoTransfer event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[NotoLock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lock NotoLock_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
			txData, err := n.decodeTransactionData(ctx, lock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lock.TxId, lock.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.Outputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.LockedOutputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[NotoUnlock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlock NotoUnlock_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlock); err == nil {
			txData, err := n.decodeTransactionData(ctx, unlock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(unlock.TxId, unlock.LockedInputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlock.TxId, unlock.LockedOutputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlock.TxId, unlock.Outputs)...)

			var domainConfig *types.NotoParsedConfig
			err = json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
			if err != nil {
				return err
			}
			if domainConfig.IsNotary &&
				domainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
				!domainConfig.Options.Hooks.PublicAddress.Equals(unlock.Sender) {
				err = n.handleNotaryPrivateUnlockV0(ctx, req.StateQueryContext, domainConfig, &unlock)
				if err != nil {
					log.L(ctx).Errorf("Failed to handle NotoUnlock event in batch %s: %s", req.BatchId, err)
					return err
				}
			}
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[NotoUnlockPrepared]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlockPrepared NotoUnlockPrepared_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlockPrepared); err == nil {
			txData, err := n.decodeTransactionData(ctx, unlockPrepared.Data)
			if err != nil {
				return err
			}
			// Transaction ID is not available in the event data, so we must decode it from the data field
			n.recordTransactionInfo(ev, txData, res)
			res.ReadStates = append(res.ReadStates, n.parseStatesFromEvent(txData.TransactionID, unlockPrepared.LockedInputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlockPrepared event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[NotoLockDelegated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockDelegated NotoLockDelegated_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockDelegated); err == nil {
			txData, err := n.decodeTransactionData(ctx, lockDelegated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData, res)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLockDelegated event in batch %s: %s", req.BatchId, err)
		}
	}
	return nil
}

func (n *Noto) handleV1Event(ctx context.Context, ev *prototk.OnChainEvent, res *prototk.HandleEventBatchResponse, req *prototk.HandleEventBatchRequest) error {
	switch ev.SoliditySignature {
	case eventSignatures[NotoTransfer]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var transfer NotoTransfer_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
			txData, err := n.decodeTransactionData(ctx, transfer.Data)
			if err != nil {
				return err
			}
			// For V1, TransactionID is not in TransactionData, but Transfer event doesn't have TxId either
			// The TransactionID will be set from decodeTransactionData fallback (random if zero)
			n.recordTransactionInfo(ev, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(txData.TransactionID, transfer.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txData.TransactionID, transfer.Outputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoTransfer event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[NotoLock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lock NotoLock_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
			txData, err := n.decodeTransactionData(ctx, lock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lock.TxId, lock.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.Outputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.LockedOutputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[NotoUnlock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlock NotoUnlock_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlock); err == nil {
			txData, err := n.decodeTransactionData(ctx, unlock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(unlock.TxId, unlock.LockedInputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlock.TxId, unlock.LockedOutputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlock.TxId, unlock.Outputs)...)

			var domainConfig *types.NotoParsedConfig
			err = json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
			if err != nil {
				return err
			}
			if domainConfig.IsNotary &&
				domainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
				!domainConfig.Options.Hooks.PublicAddress.Equals(unlock.Sender) {
				err = n.handleNotaryPrivateUnlockV1(ctx, req.StateQueryContext, domainConfig, &unlock)
				if err != nil {
					log.L(ctx).Errorf("Failed to handle NotoUnlock event in batch %s: %s", req.BatchId, err)
					return err
				}
			}
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[NotoUnlockPrepared]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlockPrepared NotoUnlockPrepared_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlockPrepared); err == nil {
			txData, err := n.decodeTransactionData(ctx, unlockPrepared.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData, res)
			res.ReadStates = append(res.ReadStates, n.parseStatesFromEvent(unlockPrepared.TxId, unlockPrepared.LockedInputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlockPrepared event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[NotoLockDelegated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockDelegated NotoLockDelegated_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockDelegated); err == nil {
			txData, err := n.decodeTransactionData(ctx, lockDelegated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData, res)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLockDelegated event in batch %s: %s", req.BatchId, err)
		}
	}
	return nil
}

// When notary logic is implemented via Pente, unlock events from the base ledger must be propagated back to the Pente hooks
// TODO: this method should not be invoked directly on the event loop, but rather via a queue
func (n *Noto) handleNotaryPrivateUnlock(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, lockedInputs []pldtypes.Bytes32, outputs []pldtypes.Bytes32, sender *pldtypes.EthAddress, data pldtypes.HexBytes, lockID pldtypes.Bytes32) error {
	lockedInputsStr := make([]string, len(lockedInputs))
	for i, input := range lockedInputs {
		lockedInputsStr[i] = input.String()
	}
	unlockedOutputsStr := make([]string, len(outputs))
	for i, output := range outputs {
		unlockedOutputsStr[i] = output.String()
	}

	inputStates, err := n.getStates(ctx, stateQueryContext, n.lockedCoinSchema.Id, lockedInputsStr)
	if err != nil {
		return err
	}
	if len(inputStates) != len(lockedInputsStr) {
		return i18n.NewError(ctx, msgs.MsgMissingStateData, lockedInputs)
	}

	outputStates, err := n.getStates(ctx, stateQueryContext, n.coinSchema.Id, unlockedOutputsStr)
	if err != nil {
		return err
	}
	if len(outputStates) != len(outputs) {
		return i18n.NewError(ctx, msgs.MsgMissingStateData, outputs)
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
			Sender:     sender,
			LockID:     lockID,
			Recipients: recipients,
			Data:       data,
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

func (n *Noto) handleNotaryPrivateUnlockV0(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, unlock *NotoUnlock_V0_Event) error {
	// V0: extract lockId from locked coin states
	var lockID pldtypes.Bytes32
	lockedInputsStr := make([]string, len(unlock.LockedInputs))
	for i, input := range unlock.LockedInputs {
		lockedInputsStr[i] = input.String()
	}
	inputStates, err := n.getStates(ctx, stateQueryContext, n.lockedCoinSchema.Id, lockedInputsStr)
	if err != nil {
		return err
	}
	if len(inputStates) > 0 {
		coin, err := n.unmarshalLockedCoin(inputStates[0].DataJson)
		if err != nil {
			return err
		}
		lockID = coin.LockID
		// TODO: should we check that all inputs have the same lock ID?
	}
	return n.handleNotaryPrivateUnlock(ctx, stateQueryContext, domainConfig, unlock.LockedInputs, unlock.Outputs, unlock.Sender, unlock.Data, lockID)
}

func (n *Noto) handleNotaryPrivateUnlockV1(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, unlock *NotoUnlock_Event) error {
	// V1: lockId is in the event
	return n.handleNotaryPrivateUnlock(ctx, stateQueryContext, domainConfig, unlock.LockedInputs, unlock.Outputs, unlock.Sender, unlock.Data, unlock.LockId)
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

func (n *Noto) recordTransactionInfo(ev *prototk.OnChainEvent, txData *types.NotoTransactionData_V0, res *prototk.HandleEventBatchResponse) {
	res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
		TransactionId: txData.TransactionID.String(),
		Location:      ev.Location,
	})
	for _, state := range txData.InfoStates {
		res.InfoStates = append(res.InfoStates, &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txData.TransactionID.String(),
		})
	}
}
