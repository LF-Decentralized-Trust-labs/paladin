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
	"fmt"
	"hash/fnv"
	"slices"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
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

type CoordinatorSelectionMode int

const (
	BlockHeightRoundRobin CoordinatorSelectionMode = iota
	HashedSelection       CoordinatorSelectionMode = iota
)

// Override only intended for unit tests currently
var EndorsementCoordinatorSelectionMode CoordinatorSelectionMode = HashedSelection

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
		staticCoordinatorNode, err := pldtypes.PrivateIdentityLocator(staticCoordinator).Node(ctx, false)
		if err != nil {
			log.L(ctx).Errorf("Error resolving node for static coordinator %s: %s", staticCoordinator, err)
			return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, err)
		}

		return &staticCoordinatorSelectorPolicy{
			nodeName: staticCoordinatorNode,
		}, nil
	}
	if contractConfig.GetCoordinatorSelection() == prototk.ContractConfig_COORDINATOR_ENDORSER {
		if EndorsementCoordinatorSelectionMode == BlockHeightRoundRobin {
			return &roundRobinCoordinatorSelectorPolicy{
				localNode: nodeName,
				rangeSize: confutil.Int(sequencerConfig.RoundRobinCoordinatorBlockRangeSize, *pldconf.PrivateTxManagerDefaults.Sequencer.RoundRobinCoordinatorBlockRangeSize),
			}, nil
		}
		// TODO: More work is required to perform leader election of an endorser, so right now a simple hash algorithm is used.
		return &endorsementSetHashSelection{
			localNode: nodeName,
		}, nil
	}
	return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidCoordinatorSelection, contractConfig.GetCoordinatorSelection())
}

type staticCoordinatorSelectorPolicy struct {
	nodeName string
}

type endorsementSetHashSelection struct {
	localNode  string
	chosenNode string
}

func (s *staticCoordinatorSelectorPolicy) SelectCoordinatorNode(ctx context.Context, _ *components.PrivateTransaction, environment ptmgrtypes.SequencerEnvironment) (int64, string, error) {
	log.L(ctx).Debugf("SelectCoordinatorNode: Selecting coordinator node %s", s.nodeName)
	return environment.GetBlockHeight(), s.nodeName, nil
}

func (s *endorsementSetHashSelection) SelectCoordinatorNode(ctx context.Context, transaction *components.PrivateTransaction, environment ptmgrtypes.SequencerEnvironment) (int64, string, error) {
	blockHeight := environment.GetBlockHeight()
	if s.chosenNode == "" {
		if transaction.PostAssembly == nil {
			//if we don't know the candidate nodes, and the transaction hasn't been assembled yet, then we can't select a coordinator so just assume we are the coordinator
			// until we get the transaction assembled and then re-evaluate
			log.L(ctx).Debug("SelectCoordinatorNode: Assembly not yet completed - using local node for assembly")
			return blockHeight, s.localNode, nil
		}
		//use a map to dedupe as we go
		candidateNodesMap := make(map[string]struct{})
		identities := make([]string, 0, len(transaction.PostAssembly.AttestationPlan))
		for _, attestationPlan := range transaction.PostAssembly.AttestationPlan {
			if attestationPlan.AttestationType == prototk.AttestationType_ENDORSE {
				for _, party := range attestationPlan.Parties {
					identity, node, err := pldtypes.PrivateIdentityLocator(party).Validate(ctx, s.localNode, false)
					if err != nil {
						log.L(ctx).Errorf("SelectCoordinatorNode: Error resolving node for party %s: %s", party, err)
						return -1, "", i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, err)
					}
					candidateNodesMap[node] = struct{}{}
					identities = append(identities, fmt.Sprintf("%s@%s", identity, node))
				}
			}
		}
		candidateNodes := make([]string, 0, len(candidateNodesMap))
		for candidateNode := range candidateNodesMap {
			candidateNodes = append(candidateNodes, candidateNode)
		}
		slices.Sort(candidateNodes)
		slices.Sort(identities)
		if len(candidateNodes) == 0 {
			log.L(ctx).Warn("SelectCoordinatorNode: No candidate nodes, assuming local node is the coordinator")
			return blockHeight, s.localNode, nil
		}
		// Take a simple numeric hash of the identities string
		h := fnv.New32a()
		for _, identity := range identities {
			h.Write([]byte(identity))
		}
		// Use that as an index into the chosen node set
		s.chosenNode = candidateNodes[int(h.Sum32())%len(candidateNodes)]
	}

	return blockHeight, s.chosenNode, nil

}

type roundRobinCoordinatorSelectorPolicy struct {
	localNode      string
	candidateNodes []string
	rangeSize      int
}

func (s *roundRobinCoordinatorSelectorPolicy) SelectCoordinatorNode(ctx context.Context, transaction *components.PrivateTransaction, environment ptmgrtypes.SequencerEnvironment) (int64, string, error) {
	blockHeight := environment.GetBlockHeight()

	if len(s.candidateNodes) == 0 {
		if transaction.PostAssembly == nil {
			//if we don't know the candidate nodes, and the transaction hasn't been assembled yet, then we can't select a coordinator so just assume we are the coordinator
			// until we get the transaction assembled and then re-evaluate
			log.L(ctx).Debug("SelectCoordinatorNode: No candidate nodes, assuming local node is the coordinator")
			return blockHeight, s.localNode, nil
		} else {
			//use a map to dedupe as we go
			candidateNodesMap := make(map[string]struct{})
			for _, attestationPlan := range transaction.PostAssembly.AttestationPlan {
				if attestationPlan.AttestationType == prototk.AttestationType_ENDORSE {
					for _, party := range attestationPlan.Parties {
						node, err := pldtypes.PrivateIdentityLocator(party).Node(ctx, true)
						if err != nil {
							log.L(ctx).Errorf("SelectCoordinatorNode: Error resolving node for party %s: %s", party, err)
							return -1, "", i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, err)
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
		return blockHeight, s.localNode, nil
	}

	rangeIndex := blockHeight / int64(s.rangeSize)

	coordinatorIndex := int(rangeIndex) % len(s.candidateNodes)
	coordinatorNode := s.candidateNodes[coordinatorIndex]
	log.L(ctx).Debugf("SelectCoordinatorNode: selected coordinator node %s using round robin algorithm for blockHeight: %d and rangeSize %d ", coordinatorNode, blockHeight, s.rangeSize)

	return blockHeight, coordinatorNode, nil

}
