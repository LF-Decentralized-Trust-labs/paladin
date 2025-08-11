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

package privatetxnmgr

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

func newStateDistributionBuilder(c components.AllComponents, tx *components.PrivateTransaction) *stateDistributionBuilder {
	return &stateDistributionBuilder{
		tx: tx,
		StateDistributionSet: components.StateDistributionSet{
			LocalNode: c.TransportManager().LocalNodeName(),
			Remote:    []*components.StateDistributionWithData{},
			Local:     []*components.StateDistributionWithData{},
		},
	}
}

type stateDistributionBuilder struct {
	components.StateDistributionSet
	tx *components.PrivateTransaction
}

func (sd *stateDistributionBuilder) processStateForDistribution(ctx context.Context, fullState *components.FullState, instruction *prototk.NewState) error {
	tx := sd.tx

	// We enforce that the sender gets a distribution
	senderLocator := tx.PreAssembly.TransactionSpecification.From
	senderIncludedByDomain := false
	for _, recipient := range instruction.DistributionList {
		if recipient == senderLocator {
			senderIncludedByDomain = true
			break
		}
	}
	if !senderIncludedByDomain {
		instruction.DistributionList = append(instruction.DistributionList, senderLocator)
	}

	remainingNullifiers := instruction.NullifierSpecs
	for _, recipient := range instruction.DistributionList {
		nodeName, err := pldtypes.PrivateIdentityLocator(recipient).Node(ctx, false)
		if err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrDistributionNotFullyQualified, recipient)
		}

		// See if we have a nullifier spec for this party
		var matchedNullifier *prototk.NullifierSpec
		newRemainingNullifiers := make([]*prototk.NullifierSpec, 0, len(remainingNullifiers))
		for _, n := range remainingNullifiers {
			if n.Party == recipient && matchedNullifier == nil {
				matchedNullifier = n // pop it out of the list - we only support one per distribution recipient
			} else {
				newRemainingNullifiers = append(newRemainingNullifiers, n)
			}
		}
		remainingNullifiers = newRemainingNullifiers

		distribution := &components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				IdentityLocator: recipient,
				Domain:          tx.Domain,
				ContractAddress: tx.Address.String(),
				// the state data json is available on both but we take it
				// from the outputState to make sure it is the same json that was used to generate the hash
				StateID:  fullState.ID.String(),
				SchemaID: fullState.Schema.String(),
			},
			StateData: fullState.Data,
		}

		// Add the nullifier requirement if there is one
		if matchedNullifier != nil {
			distribution.NullifierAlgorithm = &matchedNullifier.Algorithm
			distribution.NullifierVerifierType = &matchedNullifier.VerifierType
			distribution.NullifierPayloadType = &matchedNullifier.PayloadType
		}

		// Add it to the right list
		if nodeName == sd.LocalNode {
			log.L(ctx).Debugf("new state %s will be written locally for recipient %s hasNullifier=%t", fullState.ID, recipient, matchedNullifier != nil)
			sd.Local = append(sd.Local, distribution)
		} else {
			log.L(ctx).Debugf("new state %s will be written distributed to recipient %s hasNullifier=%t", fullState.ID, recipient, matchedNullifier != nil)
			sd.Remote = append(sd.Remote, distribution)
		}
	}

	// We require that
	// - All nullifier specs match an recipient in the distribution list
	// - There is a maximum of one nullifier spec per recipient
	if len(remainingNullifiers) != 0 {
		// .. the rules must have been broken
		log.L(ctx).Errorf("Invalid nullifier / distribution list combination: %+v", instruction)
		return i18n.NewError(ctx, msgs.MsgPrivateTxMgrInvalidNullifierSpecInDistro)
	}

	return nil

}

// This function is called by the coordinator to validate the new states produced in the postAssembly.
// It knows who the local node is, and who the sender is.
// It is aware of nullifiers and distribution lists, and produces a set of instructions for who needs what.
func (sd *stateDistributionBuilder) Build(ctx context.Context) (sds *components.StateDistributionSet, err error) {

	log.L(ctx).Debug("privateTxManager:ProcessTransactionStatesForDistribution")

	tx := sd.tx

	// This code depends on the fact we ensure this gets fully qualified as the transaction comes into the system,
	// on the sending node. So as the transaction flows around everyone knows who the originating node is.
	sd.SenderNode, err = pldtypes.PrivateIdentityLocator(tx.PreAssembly.TransactionSpecification.From).Node(ctx, false)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxMgrFromNotResolvedDistroTime)
	}

	if tx.PostAssembly == nil ||
		len(tx.PostAssembly.OutputStatesPotential) != len(tx.PostAssembly.OutputStates) ||
		len(tx.PostAssembly.InfoStatesPotential) != len(tx.PostAssembly.InfoStates) {
		log.L(ctx).Debugf("Invalid post assembly: %+v", tx.PostAssembly)
		return nil, i18n.NewError(ctx, msgs.MsgPrivateTxMgrInvalidTxStateStateDistro)
	}

	for i, fullState := range tx.PostAssembly.OutputStates {
		if err := sd.processStateForDistribution(ctx, fullState, tx.PostAssembly.OutputStatesPotential[i]); err != nil {
			return nil, err
		}
	}
	for i, fullState := range tx.PostAssembly.InfoStates {
		if err := sd.processStateForDistribution(ctx, fullState, tx.PostAssembly.InfoStatesPotential[i]); err != nil {
			return nil, err
		}
	}
	return &sd.StateDistributionSet, nil
}
