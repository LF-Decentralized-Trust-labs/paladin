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
)

func (c *contractSequencerAgent) DispatchTransactions(ctx context.Context, transactionsToDispatch DispatchableTransactions) error {
	//TODO this is a placeholder for the actual implementation which codifies a significant amount of integration with other components.
	// This will effectively be a copy/refactor of sequencer_dispatch.go in package privatetxnmgr but needs some thought about which parts to keep and
	// which parts to simplify / optimize for the new algorithm.
	//for now we just add them to the DispatchedTransactionIndex so that the sequencer state machine is consistent within itself
	for _, transactions := range transactionsToDispatch {
		for _, transaction := range transactions {

			signerLocator := transaction.Signer
			if signerLocator == "" {
				signerLocator = c.defaultSignerLocator
			}

			keyMgr := c.components.KeyManager()
			signerEthAddress, err := keyMgr.ResolveEthAddressNewDatabaseTX(ctx, signerLocator)
			if err != nil {
				//TODO log and wrap error
				return err
			}

			c.coordinator.dispatchedTransactions.add(&DispatchedTransaction{
				Delegation: transaction,
				Signer:     *signerEthAddress,
			})
		}
	}

	return nil
}
