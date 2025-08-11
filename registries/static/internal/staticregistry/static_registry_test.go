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

package staticregistry

import (
	"context"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
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
	pb := NewPlugin()
	assert.NotNil(t, pb)
}

func TestBadConfigJSON(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewStatic(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name:       "grpc",
		ConfigJson: `{!!!!`,
	})
	assert.Regexp(t, "PD040001", err)

}

func TestRegistryStringEntry(t *testing.T) {

	callbacks := &testCallbacks{
		upsertRegistryRecords: func(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest) (*prototk.UpsertRegistryRecordsResponse, error) {
			require.Len(t, req.Entries, 1)
			assert.Equal(t, "", req.Entries[0].ParentId)
			assert.Equal(t, "node1", req.Entries[0].Name)
			require.Len(t, req.Properties, 1)
			assert.NotEmpty(t, req.Properties[0].EntryId)
			assert.Equal(t, req.Entries[0].Id, req.Properties[0].EntryId)
			assert.Equal(t, "transport.grpc", req.Properties[0].Name)
			assert.Equal(t, "these are directly the details of the transport", req.Properties[0].Value)
			return &prototk.UpsertRegistryRecordsResponse{}, nil
		},
	}
	transport := NewStatic(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name: "registry1",
		ConfigJson: `{
		  "entries": {
		     "node1": {
			   "properties": {
			      "transport.grpc": "these are directly the details of the transport"
			   }
			 }
		  }
		}`,
	})
	require.NoError(t, err)

}

func TestRegistryHierarchicalEntry(t *testing.T) {

	callbacks := &testCallbacks{
		upsertRegistryRecords: func(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest) (*prototk.UpsertRegistryRecordsResponse, error) {
			require.Len(t, req.Entries, 2)
			assert.Equal(t, "", req.Entries[0].ParentId)
			assert.Equal(t, "org1", req.Entries[0].Name)
			assert.NotEmpty(t, req.Entries[1].ParentId)
			assert.Equal(t, req.Entries[0].Id, req.Entries[1].ParentId)
			assert.Equal(t, "node1", req.Entries[1].Name)
			require.Len(t, req.Properties, 1)
			assert.NotEmpty(t, req.Properties[0].EntryId)
			assert.Equal(t, req.Entries[1].Id, req.Properties[0].EntryId)
			assert.Equal(t, "transport.grpc", req.Properties[0].Name)
			assert.Equal(t, "these are directly the details of the transport", req.Properties[0].Value)
			return &prototk.UpsertRegistryRecordsResponse{}, nil
		},
	}
	transport := NewStatic(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name: "registry1",
		ConfigJson: `{
		  "entries": {
		     "org1": {
			   "children": {
			     "node1": {
					"properties": {
						"transport.grpc": "these are directly the details of the transport"
					}
				 }
			   }
			 }
		  }
		}`,
	})
	require.NoError(t, err)

}

func TestRegistryObjectEntry(t *testing.T) {

	callbacks := &testCallbacks{
		upsertRegistryRecords: func(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest) (*prototk.UpsertRegistryRecordsResponse, error) {
			require.Len(t, req.Entries, 1)
			assert.Equal(t, "", req.Entries[0].ParentId)
			assert.Equal(t, "node1", req.Entries[0].Name)
			require.Len(t, req.Properties, 1)
			assert.NotEmpty(t, req.Properties[0].EntryId)
			assert.Equal(t, req.Entries[0].Id, req.Properties[0].EntryId)
			assert.Equal(t, "transport.grpc", req.Properties[0].Name)
			assert.JSONEq(t, `{"endpoint": "dns:///127.0.0.1:12345", "issuers": "certificate\ndata\nhere"}`, req.Properties[0].Value)
			return &prototk.UpsertRegistryRecordsResponse{}, nil
		},
	}
	transport := NewStatic(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name: "grpc",
		ConfigJson: `{
		  "entries": {
		     "node1": {
			   "properties": {
			      "transport.grpc": {
				  	"endpoint": "dns:///127.0.0.1:12345",
					"issuers": "certificate\ndata\nhere"
				  }
			   }
			 }
		  }
		}`,
	})
	require.NoError(t, err)

}

func TestRegistryUpsertFail(t *testing.T) {

	callbacks := &testCallbacks{
		upsertRegistryRecords: func(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest) (*prototk.UpsertRegistryRecordsResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	}
	transport := NewStatic(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name: "grpc",
		ConfigJson: `{
		  "entries": {
		     "node1": {
			   "properties": {
			      "transport.1": "anything"
			   }
			 }
		  }
		}`,
	})
	assert.Regexp(t, "pop", err)

}

func TestRegistryEventBatch(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := NewStatic(callbacks).(*staticRegistry)
	_, err := transport.HandleRegistryEvents(context.Background(), &prototk.HandleRegistryEventsRequest{})
	assert.Regexp(t, "PD040002", err)

}

func TestRegistryUpsertBadData(t *testing.T) {
	callbacks := &testCallbacks{}
	transport := NewStatic(callbacks).(*staticRegistry)
	err := transport.recurseBuildUpsert(context.Background(),
		&prototk.UpsertRegistryRecordsRequest{},
		nil,
		"org1",
		&StaticEntry{
			Children: map[string]*StaticEntry{
				"node1": {
					Properties: map[string]pldtypes.RawJSON{
						"anything": pldtypes.RawJSON(`{!!! bad json`),
					},
				},
			},
		},
	)
	assert.Error(t, err)
}
