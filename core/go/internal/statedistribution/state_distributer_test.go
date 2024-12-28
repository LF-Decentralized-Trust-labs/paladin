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

package statedistribution

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/persistence/mockpersistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	db               *mockpersistence.SQLMockProvider
	stateManager     *componentmocks.StateManager
	keyManager       *componentmocks.KeyManager
	transportManager *componentmocks.TransportManager
	keyResolver      *componentmocks.KeyResolver
}

func newTestStateDistributor(t *testing.T) (context.Context, *mockComponents, *stateDistributer) {
	ctx := context.Background()

	mc := &mockComponents{
		stateManager:     componentmocks.NewStateManager(t),
		keyManager:       componentmocks.NewKeyManager(t),
		transportManager: componentmocks.NewTransportManager(t),
		keyResolver:      componentmocks.NewKeyResolver(t),
	}
	mc.transportManager.On("LocalNodeName").Return("node1")
	mkrc := componentmocks.NewKeyResolutionContextLazyDB(t)
	mkrc.On("KeyResolverLazyDB").Return(mc.keyResolver).Maybe()
	mkrc.On("Commit").Return(nil).Maybe()
	mkrc.On("Rollback").Return().Maybe()
	mc.keyManager.On("NewKeyResolutionContextLazyDB", mock.Anything).Return(mkrc).Maybe()

	mdb, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	mc.db = mdb

	sd := NewStateDistributer(ctx, mc.transportManager, mc.stateManager, mc.keyManager, mc.db.P, &pldconf.DistributerConfig{})

	return ctx, mc, sd.(*stateDistributer)

}

func TestBuildNullifiersNoOp(t *testing.T) {

	ctx, _, sd := newTestStateDistributor(t)

	nullifiers, err := sd.BuildNullifiers(ctx, []*components.StateDistributionWithData{
		{
			ID:              uuid.New().String(),
			StateID:         "id1",
			IdentityLocator: "target@node1",
		},
	})
	require.NoError(t, err)
	assert.Empty(t, nullifiers)

}

func TestBuildNullifiersOk(t *testing.T) {

	ctx, mc, sd := newTestStateDistributor(t)

	keyMapping := &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: &pldapi.KeyMappingWithPath{
			KeyMapping: &pldapi.KeyMapping{
				Identifier: "target",
				Wallet:     "wallet1",
				KeyHandle:  "key1",
			},
		},
	}

	mc.keyResolver.On("ResolveKey", "target", "nullifier_algo", "nullifier_verifier_type").
		Return(keyMapping, nil)

	nullifierBytes := tktypes.RandBytes(32)
	mc.keyManager.On("Sign", ctx, keyMapping, "nullifier_payload_type", []byte(`{"state":"data"}`)).
		Return(nullifierBytes, nil)

	stateID := tktypes.HexBytes(tktypes.RandBytes(32))
	nullifiers, err := sd.BuildNullifiers(ctx, []*components.StateDistributionWithData{
		{
			ID:                    uuid.New().String(),
			StateID:               stateID.String(),
			IdentityLocator:       "target@node1",
			StateDataJson:         `{"state":"data"}`,
			NullifierAlgorithm:    confutil.P("nullifier_algo"),
			NullifierVerifierType: confutil.P("nullifier_verifier_type"),
			NullifierPayloadType:  confutil.P("nullifier_payload_type"),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []*components.NullifierUpsert{
		{
			ID:    nullifierBytes,
			State: stateID,
		},
	}, nullifiers)

}

func TestBuildNullifiersFail(t *testing.T) {

	ctx, mc, sd := newTestStateDistributor(t)

	keyMapping := &pldapi.KeyMappingAndVerifier{}

	mc.keyResolver.On("ResolveKey", "target", "nullifier_algo", "nullifier_verifier_type").
		Return(&pldapi.KeyMappingAndVerifier{}, nil)

	mc.keyManager.On("Sign", ctx, keyMapping, "nullifier_payload_type", []byte(`{"state":"data"}`)).
		Return(nil, fmt.Errorf("pop"))

	stateID := tktypes.HexBytes(tktypes.RandBytes(32))
	_, err := sd.BuildNullifiers(ctx, []*components.StateDistributionWithData{
		{
			ID:                    uuid.New().String(),
			StateID:               stateID.String(),
			IdentityLocator:       "target@node1",
			StateDataJson:         `{"state":"data"}`,
			NullifierAlgorithm:    confutil.P("nullifier_algo"),
			NullifierVerifierType: confutil.P("nullifier_verifier_type"),
			NullifierPayloadType:  confutil.P("nullifier_payload_type"),
		},
	})
	assert.Regexp(t, "PD012401.*pop", err)

}

func TestBuildNullifiersNotLocal(t *testing.T) {

	ctx, _, sd := newTestStateDistributor(t)

	stateID := tktypes.HexBytes(tktypes.RandHex(32))
	_, err := sd.BuildNullifiers(ctx, []*components.StateDistributionWithData{
		{
			ID:                    uuid.New().String(),
			StateID:               stateID.String(),
			IdentityLocator:       "target", // missing node
			StateDataJson:         `{"state":"data"}`,
			NullifierAlgorithm:    confutil.P("nullifier_algo"),
			NullifierVerifierType: confutil.P("nullifier_verifier_type"),
			NullifierPayloadType:  confutil.P("nullifier_payload_type"),
		},
	})
	assert.Regexp(t, "PD012400", err)

}
