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

	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCallbacks struct {
	upsertTransportDetails func(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error)
}

func (tc *testCallbacks) UpsertTransportDetails(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {
	return tc.upsertTransportDetails(ctx, req)
}

func TestPluginLifecycle(t *testing.T) {
	pb := NewPlugin(context.Background())
	assert.NotNil(t, pb)
}

func TestBadConfigJSON(t *testing.T) {

	callbacks := &testCallbacks{}
	transport := staticRegistryFactory(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name:       "grpc",
		ConfigJson: `{!!!!`,
	})
	assert.Regexp(t, "PD040001", err)

}

func TestRegistryStringEntry(t *testing.T) {

	callbacks := &testCallbacks{
		upsertTransportDetails: func(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {
			assert.Equal(t, "node1", req.Node)
			assert.Equal(t, "transport1", req.Transport)
			assert.Equal(t, "these are directly the details of the transport", req.TransportDetails)
			return &prototk.UpsertTransportDetailsResponse{}, nil
		},
	}
	transport := staticRegistryFactory(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name: "grpc",
		ConfigJson: `{
		  "nodes": {
		     "node1": {
			   "transports": {
			      "transport1": "these are directly the details of the transport"
			   }
			 }
		  }
		}`,
	})
	require.NoError(t, err)

}

func TestRegistryObjectEntry(t *testing.T) {

	callbacks := &testCallbacks{
		upsertTransportDetails: func(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {
			assert.Equal(t, "node1", req.Node)
			assert.Equal(t, "transport1", req.Transport)
			assert.JSONEq(t, `{"endpoint": "dns:///127.0.0.1:12345", "issuers": "certificate\ndata\nhere"}`, req.TransportDetails)
			return &prototk.UpsertTransportDetailsResponse{}, nil
		},
	}
	transport := staticRegistryFactory(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name: "grpc",
		ConfigJson: `{
		  "nodes": {
		     "node1": {
			   "transports": {
			      "transport1": {
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
		upsertTransportDetails: func(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	}
	transport := staticRegistryFactory(callbacks).(*staticRegistry)
	_, err := transport.ConfigureRegistry(transport.bgCtx, &prototk.ConfigureRegistryRequest{
		Name: "grpc",
		ConfigJson: `{
		  "nodes": {
		     "node1": {
			   "transports": {
			      "transport1": "anything"
			   }
			 }
		  }
		}`,
	})
	assert.Regexp(t, "pop", err)

}

func TestRegistryUpsertBadDAta(t *testing.T) {
	callbacks := &testCallbacks{}
	transport := staticRegistryFactory(callbacks).(*staticRegistry)
	err := transport.registerNodeTransport(context.Background(), "node1", "transport1", tktypes.RawJSON(`{!!! bad json`))
	assert.Error(t, err)
}
