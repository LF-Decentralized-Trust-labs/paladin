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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (p *privateTxManager) BuildNullifier(ctx context.Context, kr components.KeyResolver, s *components.StateDistributionWithData) (*components.NullifierUpsert, error) {
	// We need to call the signing engine with the local identity to build the nullifier
	log.L(ctx).Infof("Generating nullifier for state %s on node %s (algorithm=%s,verifierType=%s,payloadType=%s)",
		s.StateID, p.nodeName, *s.NullifierAlgorithm, *s.NullifierVerifierType, *s.NullifierPayloadType)

	// We require a fully qualified identifier for the local node in this function
	identifier, node, err := tktypes.PrivateIdentityLocator(s.IdentityLocator).Validate(ctx, "", false)
	if err != nil || node != p.nodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierNotLocal)
	}

	// Call the signing engine to build the nullifier
	var nulliferBytes []byte
	mapping, err := kr.ResolveKey(identifier, *s.NullifierAlgorithm, *s.NullifierVerifierType)
	if err == nil {
		nulliferBytes, err = p.components.KeyManager().Sign(ctx, mapping, *s.NullifierPayloadType, s.StateData.Bytes())
	}
	if err != nil || len(nulliferBytes) == 0 {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierFail, s.StateID)
	}
	return &components.NullifierUpsert{
		ID:    nulliferBytes,
		State: tktypes.MustParseHexBytes(s.StateID),
	}, nil
}

func (p *privateTxManager) withKeyResolutionContext(ctx context.Context, fn func(krc components.KeyResolutionContextLazyDB) error) (err error) {

	// Unlikely we'll be resolving any new identities on this path - if we do, we'll start a new DB transaction
	// Note: This requires we're not on an existing DB TX coming into this function
	krc := p.components.KeyManager().NewKeyResolutionContextLazyDB(ctx)
	defer func() {
		if err == nil {
			err = krc.Commit()
		} else {
			krc.Rollback()
		}
	}()

	err = fn(krc)
	return err // note we require err to be set before return
}

func (p *privateTxManager) BuildNullifiers(ctx context.Context, stateDistributions []*components.StateDistributionWithData) (nullifiers []*components.NullifierUpsert, err error) {

	nullifiers = []*components.NullifierUpsert{}
	err = p.withKeyResolutionContext(ctx, func(krc components.KeyResolutionContextLazyDB) error {
		for _, s := range stateDistributions {
			if s.NullifierAlgorithm == nil || s.NullifierVerifierType == nil || s.NullifierPayloadType == nil {
				log.L(ctx).Debugf("No nullifier required for state %s on node %s", s.StateID, p.nodeName)
				continue
			}

			nullifier, err := p.BuildNullifier(ctx, krc.KeyResolverLazyDB(), s)
			if err != nil {
				return err
			}

			nullifiers = append(nullifiers, nullifier)
		}
		return nil
	})
	return nullifiers, err
}
