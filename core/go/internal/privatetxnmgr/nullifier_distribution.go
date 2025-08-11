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
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

func (p *privateTxManager) BuildNullifier(ctx context.Context, kr components.KeyResolver, s *components.StateDistributionWithData) (*components.NullifierUpsert, error) {
	// We need to call the signing engine with the local identity to build the nullifier
	log.L(ctx).Infof("Generating nullifier for state %s on node %s (algorithm=%s,verifierType=%s,payloadType=%s)",
		s.StateID, p.nodeName, *s.NullifierAlgorithm, *s.NullifierVerifierType, *s.NullifierPayloadType)

	// We require a fully qualified identifier for the local node in this function
	identifier, node, err := pldtypes.PrivateIdentityLocator(s.IdentityLocator).Validate(ctx, "", false)
	if err != nil || node != p.nodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierNotLocal)
	}

	// Call the signing engine to build the nullifier
	var nulliferBytes []byte
	mapping, err := kr.ResolveKey(ctx, identifier, *s.NullifierAlgorithm, *s.NullifierVerifierType)
	if err == nil {
		nulliferBytes, err = p.components.KeyManager().Sign(ctx, mapping, *s.NullifierPayloadType, s.StateData.Bytes())
	}
	if err != nil || len(nulliferBytes) == 0 {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierFail, s.StateID)
	}
	return &components.NullifierUpsert{
		ID:    nulliferBytes,
		State: pldtypes.MustParseHexBytes(s.StateID),
	}, nil
}

func (p *privateTxManager) BuildNullifiers(ctx context.Context, stateDistributions []*components.StateDistributionWithData) (nullifiers []*components.NullifierUpsert, err error) {

	nullifiers = []*components.NullifierUpsert{}
	err = p.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for _, s := range stateDistributions {
			if s.NullifierAlgorithm == nil || s.NullifierVerifierType == nil || s.NullifierPayloadType == nil {
				log.L(ctx).Debugf("No nullifier required for state %s on node %s", s.StateID, p.nodeName)
				continue
			}

			nullifier, err := p.BuildNullifier(ctx, p.components.KeyManager().KeyResolverForDBTX(dbTX), s)
			if err != nil {
				return err
			}

			nullifiers = append(nullifiers, nullifier)
		}
		return nil
	})
	return nullifiers, err
}
