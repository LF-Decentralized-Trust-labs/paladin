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

package evmregistry

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCallbacks struct {
	upsertRegistryRecords func(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest) (*prototk.UpsertRegistryRecordsResponse, error)
}

func (tc *testCallbacks) UpsertRegistryRecords(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest) (*prototk.UpsertRegistryRecordsResponse, error) {
	return tc.upsertRegistryRecords(ctx, req)
}

func TestPluginLifecycle(t *testing.T) {
	pb := NewPlugin(context.Background())
	assert.NotNil(t, pb)
}

func TestBadConfigJSON(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name:       "grpc",
		ConfigJson: `{!!!!`,
	})
	assert.Regexp(t, "PD060001", err)

}

func TestGoodConfigJSON(t *testing.T) {

	addr := tktypes.RandAddress()

	callbacks := &testCallbacks{}
	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	res, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name: "grpc",
		ConfigJson: fmt.Sprintf(`{
			"contractAddress": "%s"
		}`, addr),
	})
	require.NoError(t, err)

	assert.Equal(t, addr.String(), res.RegistryConfig.EventSources[0].ContractAddress)

}

func TestHandleEventBatchOk(t *testing.T) {

	txHash1 := tktypes.Bytes32(tktypes.RandBytes(32)).String()
	txHash2 := tktypes.Bytes32(tktypes.RandBytes(32)).String()

	identityRegistered := IdentityRegisteredEvent{
		ParentIdentityHash: tktypes.Bytes32(tktypes.RandBytes(32)),
		IdentityHash:       tktypes.Bytes32(tktypes.RandBytes(32)),
		Name:               "node1",
		Owner:              *tktypes.RandAddress(),
	}

	propSet := PropertySetEvent{
		IdentityHash: identityRegistered.IdentityHash,
		Name:         "transport.grpc",
		Value:        `{"endpoint":"details"}`,
	}

	callbacks := &testCallbacks{
		upsertRegistryRecords: func(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest) (*prototk.UpsertRegistryRecordsResponse, error) {
			require.Len(t, req.Entries, 1)

			require.Equal(t, &prototk.RegistryEntry{
				Id:       identityRegistered.IdentityHash.String(),
				ParentId: identityRegistered.ParentIdentityHash.String(),
				Name:     "node1",
				Active:   true,
				Location: &prototk.OnChainEventLocation{
					TransactionHash:  txHash1,
					BlockNumber:      100,
					TransactionIndex: 10,
					LogIndex:         5,
				},
			}, req.Entries[0])

			require.Len(t, req.Properties, 2)

			require.Equal(t, &prototk.RegistryProperty{
				EntryId:        identityRegistered.IdentityHash.String(),
				Name:           "$owner",
				Value:          identityRegistered.Owner.String(),
				PluginReserved: true,
				Active:         true,
				Location: &prototk.OnChainEventLocation{
					TransactionHash:  txHash1,
					BlockNumber:      100,
					TransactionIndex: 10,
					LogIndex:         5,
				},
			}, req.Properties[0])

			require.Equal(t, &prototk.RegistryProperty{
				EntryId: identityRegistered.IdentityHash.String(),
				Name:    "transport.grpc",
				Value:   `{"endpoint":"details"}`,
				Active:  true,
				Location: &prototk.OnChainEventLocation{
					TransactionHash:  txHash2,
					BlockNumber:      200,
					TransactionIndex: 20,
					LogIndex:         10,
				},
			}, req.Properties[1])

			return &prototk.UpsertRegistryRecordsResponse{}, nil
		},
	}

	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	_, err := transport.HandleRegistryEvents(transport.bgCtx, &prototk.HandleRegistryEventsRequest{
		BatchId: uuid.New().String(),
		Events: []*prototk.OnChainEvent{
			{
				Location:          &prototk.OnChainEventLocation{TransactionHash: txHash1, BlockNumber: 100, TransactionIndex: 10, LogIndex: 5},
				Signature:         contractDetail.identityRegisteredSignature.String(),
				SoliditySignature: identityRegisteredEventSolSig,
				DataJson:          tktypes.JSONString(&identityRegistered).Pretty(),
			},
			{
				Location:          &prototk.OnChainEventLocation{TransactionHash: txHash2, BlockNumber: 200, TransactionIndex: 20, LogIndex: 10},
				Signature:         contractDetail.propertySetSignature.String(),
				SoliditySignature: propertySetEventSolSig,
				DataJson:          tktypes.JSONString(&propSet).Pretty(),
			},
		},
	})
	require.NoError(t, err)

}

func TestHandleEventBadIdentityRegistered(t *testing.T) {

	txHash := tktypes.Bytes32(tktypes.RandBytes(32)).String()
	callbacks := &testCallbacks{}

	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	_, err := transport.HandleRegistryEvents(transport.bgCtx, &prototk.HandleRegistryEventsRequest{
		BatchId: uuid.New().String(),
		Events: []*prototk.OnChainEvent{
			{
				Location:          &prototk.OnChainEventLocation{TransactionHash: txHash, BlockNumber: 100, TransactionIndex: 10, LogIndex: 5},
				Signature:         contractDetail.identityRegisteredSignature.String(),
				SoliditySignature: identityRegisteredEventSolSig,
				DataJson:          `{"owner": "WRONG"}`,
			},
		},
	})
	require.Regexp(t, "PD060002", err)

}

func TestHandleEventBadSetProperty(t *testing.T) {

	txHash := tktypes.Bytes32(tktypes.RandBytes(32)).String()
	callbacks := &testCallbacks{}

	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	_, err := transport.HandleRegistryEvents(transport.bgCtx, &prototk.HandleRegistryEventsRequest{
		BatchId: uuid.New().String(),
		Events: []*prototk.OnChainEvent{
			{
				Location:          &prototk.OnChainEventLocation{TransactionHash: txHash, BlockNumber: 100, TransactionIndex: 10, LogIndex: 5},
				Signature:         contractDetail.propertySetSignature.String(),
				SoliditySignature: propertySetEventSolSig,
				DataJson:          `{"name": { "not": "stringy" }}`,
			},
		},
	})
	require.Regexp(t, "PD060002", err)

}

func TestHandleEventBadSig(t *testing.T) {

	callbacks := &testCallbacks{}

	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	_, err := transport.HandleRegistryEvents(transport.bgCtx, &prototk.HandleRegistryEventsRequest{
		BatchId: uuid.New().String(),
		Events: []*prototk.OnChainEvent{
			{
				Signature: "Wrong",
			},
		},
	})
	require.Regexp(t, "PD060002", err)

}
func TestHandleEventUnknownSig(t *testing.T) {

	txHash := tktypes.Bytes32(tktypes.RandBytes(32)).String()
	callbacks := &testCallbacks{}

	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	res, err := transport.HandleRegistryEvents(transport.bgCtx, &prototk.HandleRegistryEventsRequest{
		BatchId: uuid.New().String(),
		Events: []*prototk.OnChainEvent{
			{
				Location:          &prototk.OnChainEventLocation{TransactionHash: txHash, BlockNumber: 100, TransactionIndex: 10, LogIndex: 5},
				Signature:         tktypes.RandHex(32),
				SoliditySignature: "event any()",
				DataJson:          `{}`,
			},
		},
	})
	require.NoError(t, err)
	require.Empty(t, res.Entries)
	require.Empty(t, res.Properties)

}

func TestHandleEventBadEntryName(t *testing.T) {

	txHash := tktypes.Bytes32(tktypes.RandBytes(32)).String()
	callbacks := &testCallbacks{}

	identityRegistered := IdentityRegisteredEvent{
		ParentIdentityHash: tktypes.Bytes32(tktypes.RandBytes(32)),
		IdentityHash:       tktypes.Bytes32(tktypes.RandBytes(32)),
		Name:               "___ wrong",
		Owner:              *tktypes.RandAddress(),
	}

	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	res, err := transport.HandleRegistryEvents(transport.bgCtx, &prototk.HandleRegistryEventsRequest{
		BatchId: uuid.New().String(),
		Events: []*prototk.OnChainEvent{
			{
				Location:          &prototk.OnChainEventLocation{TransactionHash: txHash, BlockNumber: 100, TransactionIndex: 10, LogIndex: 5},
				Signature:         contractDetail.identityRegisteredSignature.String(),
				SoliditySignature: identityRegisteredEventSolSig,
				DataJson:          tktypes.JSONString(&identityRegistered).Pretty(),
			},
		},
	})
	require.NoError(t, err)
	require.Empty(t, res.Entries)
	require.Empty(t, res.Properties)

}

func TestHandleEventBatchPropBadName(t *testing.T) {

	txHash := tktypes.Bytes32(tktypes.RandBytes(32)).String()

	propSet := PropertySetEvent{
		IdentityHash: tktypes.Bytes32(tktypes.RandBytes(32)),
		Name:         "___ wrong",
		Value:        `{"endpoint":"details"}`,
	}

	callbacks := &testCallbacks{}

	transport := evmRegistryFactory(callbacks).(*evmRegistry)
	res, err := transport.HandleRegistryEvents(transport.bgCtx, &prototk.HandleRegistryEventsRequest{
		BatchId: uuid.New().String(),
		Events: []*prototk.OnChainEvent{
			{
				Location:          &prototk.OnChainEventLocation{TransactionHash: txHash, BlockNumber: 200, TransactionIndex: 20, LogIndex: 10},
				Signature:         contractDetail.propertySetSignature.String(),
				SoliditySignature: propertySetEventSolSig,
				DataJson:          tktypes.JSONString(&propSet).Pretty(),
			},
		},
	})
	require.NoError(t, err)
	require.Empty(t, res.Entries)
	require.Empty(t, res.Properties)

}
