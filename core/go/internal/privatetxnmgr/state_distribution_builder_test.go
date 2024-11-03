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
	"testing"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStateDistributionBuilder(t *testing.T, tx *components.PrivateTransaction) (context.Context, *stateDistributionBuilder) {
	mt := componentmocks.NewTransportManager(t)
	mt.On("LocalNodeName").Return("node1")

	mc := componentmocks.NewAllComponents(t)
	mc.On("TransportManager").Return(mt)

	return context.Background(), newStateDistributionBuilder(mc, tx)
}

func TestStateDistributionBuilderAllSenderNoNullifiers(t *testing.T) {
	schema1ID := tktypes.Bytes32(tktypes.RandBytes(32))
	state1ID := tktypes.HexBytes(tktypes.RandBytes(32))
	schema2ID := tktypes.Bytes32(tktypes.RandBytes(32))
	state2ID := tktypes.HexBytes(tktypes.RandBytes(32))
	contractAddr := *tktypes.RandAddress()
	ctx, sd := newTestStateDistributionBuilder(t, &components.PrivateTransaction{
		Inputs: &components.TransactionInputs{
			From:   "sender@node1",
			Domain: "domain1",
			To:     contractAddr,
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStates: []*components.FullState{
				{
					ID:     state1ID,
					Schema: schema1ID,
					Data:   tktypes.RawJSON(`{"state":"1"}`),
				},
			},
			OutputStatesPotential: []*prototk.NewState{
				{ /* nothing read from here in this test - we gen sender distros */ },
			},
			InfoStates: []*components.FullState{
				{
					ID:     state2ID,
					Schema: schema2ID,
					Data:   tktypes.RawJSON(`{"state":"2"}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{
				{ /* nothing read from here in this test - we gen sender distros  */ },
			},
		},
	})

	sds, err := sd.Build(ctx)
	require.NoError(t, err)
	assert.Empty(t, sds.Remote)
	require.Len(t, sds.Local, 2)

	assert.NotEmpty(t, sds.Local[0].ID)
	assert.Equal(t, "sender@node1", sds.Local[0].IdentityLocator)
	assert.Equal(t, "domain1", sds.Local[0].Domain)
	assert.Equal(t, contractAddr.String(), sds.Local[0].ContractAddress)
	assert.Equal(t, state1ID.String(), sds.Local[0].StateID)
	assert.Equal(t, schema1ID.String(), sds.Local[0].SchemaID)
	assert.JSONEq(t, `{"state":"1"}`, sds.Local[0].StateDataJson)

	assert.NotEmpty(t, sds.Local[1].ID)
	assert.Equal(t, "sender@node1", sds.Local[1].IdentityLocator)
	assert.Equal(t, "domain1", sds.Local[1].Domain)
	assert.Equal(t, contractAddr.String(), sds.Local[1].ContractAddress)
	assert.Equal(t, state2ID.String(), sds.Local[1].StateID)
	assert.Equal(t, schema2ID.String(), sds.Local[1].SchemaID)
	assert.JSONEq(t, `{"state":"2"}`, sds.Local[1].StateDataJson)
}

func TestStateDistributionWithNullifiersAllRemote(t *testing.T) {
	schema1ID := tktypes.Bytes32(tktypes.RandBytes(32))
	state1ID := tktypes.HexBytes(tktypes.RandBytes(32))
	state2ID := tktypes.HexBytes(tktypes.RandBytes(32))
	contractAddr := *tktypes.RandAddress()
	ctx, sd := newTestStateDistributionBuilder(t, &components.PrivateTransaction{
		Inputs: &components.TransactionInputs{
			From:   "bob@node2",
			Domain: "domain1",
			To:     contractAddr,
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStates: []*components.FullState{
				{
					ID:     state1ID,
					Schema: schema1ID,
					Data:   tktypes.RawJSON(`{"coin":"with change back to bob"}`),
				},
				{
					ID:     state2ID,
					Schema: schema1ID,
					Data:   tktypes.RawJSON(`{"coin":"with value for sally"}`),
				},
			},
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"bob@node2"},
					NullifierSpecs: []*prototk.NullifierSpec{
						{
							Party:        "bob@node2",
							Algorithm:    "nullifier_algo",
							VerifierType: "nullifier_verifier_type",
							PayloadType:  "nullifier_payload_type",
						},
					},
				},
				{
					DistributionList: []string{"sally@node3"},
					NullifierSpecs: []*prototk.NullifierSpec{
						{
							Party:        "sally@node3",
							Algorithm:    "nullifier_algo",
							VerifierType: "nullifier_verifier_type",
							PayloadType:  "nullifier_payload_type",
						},
					},
				},
			},
		},
	})

	sds, err := sd.Build(ctx)
	require.NoError(t, err)
	assert.Len(t, sds.Remote, 3) // bob gets all the coins as the sender, whether asked in distro or not

	// in this example the local coordinator node isn't involved
	require.Empty(t, sds.Local)

	checkCommon := func(s *components.StateDistribution, withNullifier bool) {
		if withNullifier {
			require.Equal(t, "nullifier_algo", *s.NullifierAlgorithm)
			require.Equal(t, "nullifier_verifier_type", *s.NullifierVerifierType)
			require.Equal(t, "nullifier_payload_type", *s.NullifierPayloadType)
		} else {
			assert.Nil(t, s.NullifierAlgorithm)
		}
		assert.Equal(t, "domain1", s.Domain)
		assert.Equal(t, contractAddr.String(), s.ContractAddress)
		assert.Equal(t, schema1ID.String(), s.SchemaID)
	}

	// Bob gets his change on node2
	assert.NotEmpty(t, sds.Remote[0].ID)
	assert.Equal(t, "bob@node2", sds.Remote[0].IdentityLocator)
	assert.Equal(t, state1ID.String(), sds.Remote[0].StateID)
	assert.JSONEq(t, `{"coin":"with change back to bob"}`, sds.Remote[0].StateDataJson)
	checkCommon(sds.Remote[0], true)

	// Sally gets her coin
	assert.NotEmpty(t, sds.Remote[1].ID)
	assert.Equal(t, "sally@node3", sds.Remote[1].IdentityLocator)
	assert.Equal(t, state2ID.String(), sds.Remote[1].StateID)
	assert.JSONEq(t, `{"coin":"with value for sally"}`, sds.Remote[2].StateDataJson)
	checkCommon(sds.Remote[1], true)

	// Bob also gets sally's coin - but without a nullifier spec
	assert.NotEmpty(t, sds.Remote[2].ID)
	assert.Equal(t, "bob@node2", sds.Remote[2].IdentityLocator)
	assert.Equal(t, state2ID.String(), sds.Remote[2].StateID)
	assert.JSONEq(t, `{"coin":"with value for sally"}`, sds.Remote[2].StateDataJson)
	checkCommon(sds.Remote[2], false)

}

func TestStateDistributionNonFullyQualifiedSender(t *testing.T) {

	ctx, sd := newTestStateDistributionBuilder(t, &components.PrivateTransaction{
		Inputs: &components.TransactionInputs{
			From:   "bob",
			Domain: "domain1",
		},
	})

	_, err := sd.Build(ctx)
	assert.Regexp(t, "PD011830", err)

}

func TestStateDistributionInvalidAssembly(t *testing.T) {

	ctx, sd := newTestStateDistributionBuilder(t, &components.PrivateTransaction{
		Inputs: &components.TransactionInputs{
			From:   "bob@node1",
			Domain: "domain1",
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{SchemaId: "schema1"},
			},
			// but no full states
		},
	})

	_, err := sd.Build(ctx)
	assert.Regexp(t, "PD011831", err)

}

func TestStateDistributionInvalidNullifiers(t *testing.T) {

	schema1ID := tktypes.Bytes32(tktypes.RandBytes(32))
	state1ID := tktypes.HexBytes(tktypes.RandBytes(32))
	contractAddr := *tktypes.RandAddress()
	ctx, sd := newTestStateDistributionBuilder(t, &components.PrivateTransaction{
		Inputs: &components.TransactionInputs{
			From:   "bob@node2",
			Domain: "domain1",
			To:     contractAddr,
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStates: []*components.FullState{
				{
					ID:     state1ID,
					Schema: schema1ID,
					Data:   tktypes.RawJSON(`{"some":"coin"}`),
				},
			},
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"bob@node2"},
					NullifierSpecs: []*prototk.NullifierSpec{
						{
							Party:        "not.bob@node2",
							Algorithm:    "nullifier_algo",
							VerifierType: "nullifier_verifier_type",
							PayloadType:  "nullifier_payload_type",
						},
					},
				},
			},
		},
	})

	_, err := sd.Build(ctx)
	assert.Regexp(t, "PD011833", err)

}

func TestStateDistributionInfoStateNoNodeName(t *testing.T) {

	schema1ID := tktypes.Bytes32(tktypes.RandBytes(32))
	state1ID := tktypes.HexBytes(tktypes.RandBytes(32))
	contractAddr := *tktypes.RandAddress()
	ctx, sd := newTestStateDistributionBuilder(t, &components.PrivateTransaction{
		Inputs: &components.TransactionInputs{
			From:   "bob@node2",
			Domain: "domain1",
			To:     contractAddr,
		},
		PostAssembly: &components.TransactionPostAssembly{
			InfoStates: []*components.FullState{
				{
					ID:     state1ID,
					Schema: schema1ID,
					Data:   tktypes.RawJSON(`{"some":"coin"}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"no.node.name"},
				},
			},
		},
	})

	_, err := sd.Build(ctx)
	assert.Regexp(t, "PD011832", err)

}