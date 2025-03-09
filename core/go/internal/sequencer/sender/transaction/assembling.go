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
)

func action_AssembleAndSign(ctx context.Context, txn *Transaction) error {
	if txn.inprogressAssembleRequest == nil {
		//This should never happen unless there is a bug in the state machine logic
		log.L(ctx).Errorf("No inprogress assemble request found")
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "No inprogress assemble request found")
	}
	postAssembly, err := txn.engineIntegration.AssembleAndSign(ctx, txn.ID, txn.PreAssembly, txn.inprogressAssembleRequest.stateLocksJSON, txn.inprogressAssembleRequest.coordinatorsBlockHeight)
	if err != nil {
		log.L(ctx).Errorf("Failed to assemble and sign transaction: %s", err)
		//TODO is this right to cause an abend. The other option is to send an error back to the coordinator and let it decide whether to abend itself or not?
		//Most likely cause of failure is an error in the local domain code so probably right to abend the sender state machine
		return err
	}

	txn.PostAssembly = postAssembly
	return nil
}

func action_SendAssembleRevertResponse(ctx context.Context, txn *Transaction) error {
	// TODO
	return nil
}
func action_SendAssembleParkResponse(ctx context.Context, txn *Transaction) error {
	//TODO
	return nil
}

func action_SendAssembleSuccessResponse(ctx context.Context, txn *Transaction) error {
	// TODO
	return nil
}
