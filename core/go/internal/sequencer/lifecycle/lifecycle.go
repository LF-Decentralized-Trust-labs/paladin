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

package lifecycle

type SequencerLifecycle interface {

	// Loads the distributed sequencer for a specific contract. The requested sequencer may be:
	// - already running and coordinating and/or sending other transactions
	// - not currently running and needs starting
	// - has never been run before and needs creating and starting
	// LoadSequencer returns a running sequencer capable of handling transactions. The sequencer may be stopped
	// at any time depending on how many other sequencers are required and how many can be accommodated in parallel
	// but it can always be reloaded with any context it had before it was stopped
	//LoadSequencer(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (sequencer.Sequencer, error)
}
