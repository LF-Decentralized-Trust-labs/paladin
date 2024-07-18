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

package engine

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/confutil"

	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
)

type DispatchAddress string

type TxSubmissionOutput struct {
	ErrorMessage  string
	TransactionID string
}

type DispatchStage struct{}

func (ds *DispatchStage) Name() string {
	return "dispatch"
}

func (ds *DispatchStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs StageFoundationService) *TxProcessPreReq {
	txsToCheck := tsg.GetPreReqTransactions(ctx)
	preReqsPending := sfs.DependencyChecker().PreReqsMatchCondition(ctx, txsToCheck, func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool) {
		if preReqTx.GetDispatchAddress(ctx) != tsg.GetDispatchAddress(ctx) { // the pre-req tx is managed by a different address - no matter which node
			if preReqTx.GetConfirmedTxHash(ctx) != "" {
				return true
			}
		} else {
			// managed by the same address so we can chain them using nonce
			if preReqTx.GetDispatchTxID(ctx) != "" { // if the pre-req transaction already has a tracking tx ID, it means it has a nonce, so it's no longer a pre-req
				return true
			}
		}
		return false
	})
	if len(preReqsPending) > 0 {
		return &TxProcessPreReq{
			TxIDs: preReqsPending,
		}
	}
	return nil
}

func (ds *DispatchStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs StageFoundationService, stageEvents []*StageEvent) (unprocessedStageEvents []*StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep StageProcessNextStep) {
	unprocessedStageEvents = []*StageEvent{}
	nextStep = NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == ds.Name() { // the current stage does not care about events from other stages yet (may need to be for interrupts)
			if se.Data == nil {
				//TODO: panic error, retry?
				return nil, nil, NextStepWait
			}
			switch v := se.Data.(type) {
			case DispatchAddress:
				if txUpdates == nil {
					txUpdates = &transactionstore.TransactionUpdate{}
				}
				txUpdates.DispatchAddress = confutil.P(string(v))
				nextStep = NextStepNewAction
			case TxSubmissionOutput:
				if txUpdates == nil {
					txUpdates = &transactionstore.TransactionUpdate{}
				}
				submissionOutput := v
				if submissionOutput.ErrorMessage != "" {
					// handle submission error
				} else {
					txUpdates.DispatchTxID = &submissionOutput.TransactionID
					nextStep = NextStepNewStage
				}
			}

		} else {
			unprocessedStageEvents = append(unprocessedStageEvents, se)
		}
	}
	return
}

func (ds *DispatchStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs StageFoundationService) bool {
	if tsg.GetDispatchTxPayload(ctx) != "" && sfs.NodeAndWallet().IsCurrentNode(tsg.GetDispatchNode(ctx)) && tsg.GetDispatchTxID(ctx) == "" {
		// NOTE: we will use transaction payload hash as idempotency key, so it's retry safe
		return true
	}
	return false
}

func (ds *DispatchStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs StageFoundationService) (actionOutput interface{}, actionErr error) {
	if tsg.GetDispatchAddress(ctx) == "" {
		preReqAddresses := sfs.DependencyChecker().GetPreReqDispatchAddresses(ctx, tsg.GetPreReqTransactions(ctx))
		address := sfs.NodeAndWallet().GetDispatchAddress(preReqAddresses)
		if address != "" {
			return DispatchAddress(address), nil
		}
	} else {
		// TODO: submit tx
		return TxSubmissionOutput{}, nil
	}

	return nil, i18n.NewError(ctx, msgs.MsgTransactionProcessorNoValidActions, tsg.GetTxID(ctx))
}
