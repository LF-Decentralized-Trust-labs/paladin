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
	"slices"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// Coordinator selector policy is either
//  - coordinator node is statically configured in the contract
//  - deterministic and fair rotation between a predefined set of endorsers
//  - the sender of the transaction coordinates the transaction
//
// Submitter selection policy is either
// - Coordinator submits
// - Sender submits
// - 3rd party submission

// Currently only the following combinations are implemented
// 1+1 - core option set for Noto
// 2+1 - core option set for Pente
// 3+2 - core option set for Zeto

func NewCoordinatorSelector(ctx context.Context, nodeName string, contractConfig *prototk.ContractConfig, sequencerConfig pldconf.PrivateTxManagerSequencerConfig) (ptmgrtypes.CoordinatorSelector, error) {
	if contractConfig.GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_SENDER {
		return &staticCoordinatorSelectorPolicy{
			nodeName: nodeName,
		}, nil
	}
	if contractConfig.GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_STATIC {
		staticCoordinator := contractConfig.GetStaticCoordinator()
		//staticCoordinator must be a fully qualified identity because it is also used to locate the signing key
		// but at this point, we only need the node name
		staticCoordinatorNode, err := tktypes.PrivateIdentityLocator(staticCoordinator).Node(ctx, false)
		if err != nil {
			log.L(ctx).Errorf("Error resolving node for static coordinator %s: %s", staticCoordinator, err)
			return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, err)
		}

		return &staticCoordinatorSelectorPolicy{
			nodeName: staticCoordinatorNode,
		}, nil
	}
	if contractConfig.GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_ENDORSER {
		return &roundRobinCoordinatorSelectorPolicy{
			localNode: nodeName,
			rangeSize: confutil.Int(sequencerConfig.RoundRobinCoordinatorBlockRangeSize, *pldconf.PrivateTxManagerDefaults.Sequencer.RoundRobinCoordinatorBlockRangeSize),
		}, nil
	}
	return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidCoordinatorSelection, contractConfig.GetCoordinatorSelection())
}

type staticCoordinatorSelectorPolicy struct {
	nodeName string
}

type roundRobinCoordinatorSelectorPolicy struct {
	localNode      string
	candidateNodes []string
	rangeSize      int
}

func (s *staticCoordinatorSelectorPolicy) SelectCoordinatorNode(ctx context.Context, _ *components.PrivateTransaction, _ ptmgrtypes.SequencerEnvironment) (string, error) {
	log.L(ctx).Debugf("SelectCoordinatorNode: Selecting coordinator node %s", s.nodeName)
	return s.nodeName, nil
}

func (s *roundRobinCoordinatorSelectorPolicy) SelectCoordinatorNode(ctx context.Context, transaction *components.PrivateTransaction, environment ptmgrtypes.SequencerEnvironment) (string, error) {
	if len(s.candidateNodes) == 0 {
		if transaction.PostAssembly == nil {
			//if we don't know the candidate nodes, and the transaction hasn't been assembled yet, then we can't select a coordinator so just assume we are the coordinator
			// until we get the transaction assembled and then re-evaluate
			log.L(ctx).Debug("SelectCoordinatorNode: No candidate nodes, assuming local node is the coordinator")
			return s.localNode, nil
		} else {
			//use a map to dedupe as we go
			candidateNodesMap := make(map[string]struct{})
			for _, attestationPlan := range transaction.PostAssembly.AttestationPlan {
				if attestationPlan.AttestationType == prototk.AttestationType_ENDORSE {
					for _, party := range attestationPlan.Parties {
						node, err := tktypes.PrivateIdentityLocator(party).Node(ctx, true)
						if err != nil {
							log.L(ctx).Errorf("SelectCoordinatorNode: Error resolving node for party %s: %s", party, err)
							return "", i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, err)
						}
						if node == "" {
							node = s.localNode
						}
						candidateNodesMap[node] = struct{}{}
					}
				}
			}
			for candidateNode := range candidateNodesMap {
				s.candidateNodes = append(s.candidateNodes, candidateNode)
			}
			slices.Sort(s.candidateNodes)
		}
	}

	if len(s.candidateNodes) == 0 {
		//if we still don't have any candidate nodes, then we can't select a coordinator so just assume we are the coordinator
		log.L(ctx).Debug("SelectCoordinatorNode: No candidate nodes, assuming local node is the coordinator")
		return s.localNode, nil
	}

	blockHeight := environment.GetBlockHeight()

	rangeIndex := blockHeight / int64(s.rangeSize)

	coordinatorIndex := int(rangeIndex) % len(s.candidateNodes)
	coordinatorNode := s.candidateNodes[coordinatorIndex]
	log.L(ctx).Debugf("SelectCoordinatorNode: selected coordinator node %s using round robin algorithm for blockHeight: %d and rangeSize %d ", coordinatorNode, blockHeight, s.rangeSize)

	return coordinatorNode, nil

}
