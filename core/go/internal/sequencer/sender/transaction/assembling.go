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
package transaction

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

func action_AssembleAndSign(ctx context.Context, txn *Transaction) error {
	if txn.latestAssembleRequest == nil {
		//This should never happen unless there is a bug in the state machine logic
		log.L(ctx).Errorf("No assemble request found")
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "No assemble request found")
	}

	requestID := txn.latestAssembleRequest.requestID

	// The following could be offloaded to a separate goroutine because the response is applied to the state machine via an event emission
	// However, we do pass the preAssembly by pointer so there may be a need to add locking or pass by value if we off load to a separate thread
	// lets keep it synchronous for now given that the whole contract is single threaded on the assemble stage anyway, this is unlikely to have a huge negative impact
	// but from a flow of data perspective and the state machine logic, it _could_ be converted to async
	postAssembly, err := txn.engineIntegration.AssembleAndSign(ctx, txn.ID, txn.PreAssembly, txn.latestAssembleRequest.stateLocksJSON, txn.latestAssembleRequest.coordinatorsBlockHeight)
	if err != nil {
		log.L(ctx).Errorf("Failed to assemble and sign transaction: %s", err)
		//TODO is this right to cause an abend. The other option is to send an error back to the coordinator and let it decide whether to abend itself or not?
		//Most likely cause of failure is an error in the local domain code so probably right to abend the sender state machine
		return err
	}

	switch postAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
		txn.emit(&AssembleAndSignSuccessEvent{
			event: event{
				TransactionID: txn.ID,
			},
			RequestID:    requestID,
			PostAssembly: postAssembly,
		})
	case prototk.AssembleTransactionResponse_REVERT:
		txn.emit(&AssembleRevertEvent{
			event: event{
				TransactionID: txn.ID,
			},
			RequestID:    requestID,
			PostAssembly: postAssembly,
		})
	case prototk.AssembleTransactionResponse_PARK:
		txn.emit(&AssembleParkEvent{
			event: event{
				TransactionID: txn.ID,
			},
			RequestID:    requestID,
			PostAssembly: postAssembly,
		})
	}
	return nil
}

func action_SendAssembleRevertResponse(ctx context.Context, txn *Transaction) error {

	txn.messageSender.SendAssembleResponse(ctx, txn.latestFulfilledAssembleRequestID, txn.PostAssembly)
	return nil
}
func action_SendAssembleParkResponse(ctx context.Context, txn *Transaction) error {
	txn.messageSender.SendAssembleResponse(ctx, txn.latestFulfilledAssembleRequestID, txn.PostAssembly)
	return nil
}

func action_SendAssembleSuccessResponse(ctx context.Context, txn *Transaction) error {
	txn.messageSender.SendAssembleResponse(ctx, txn.latestFulfilledAssembleRequestID, txn.PostAssembly)
	return nil
}
