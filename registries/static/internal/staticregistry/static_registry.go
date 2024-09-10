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
	"encoding/json"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/registries/static/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type Server interface {
	Start() error
	Stop()
}

type staticRegistry struct {
	bgCtx     context.Context
	callbacks plugintk.RegistryCallbacks

	conf *Config
	name string
}

func NewPlugin(ctx context.Context) plugintk.PluginBase {
	return plugintk.NewRegistry(staticRegistryFactory)
}

func staticRegistryFactory(callbacks plugintk.RegistryCallbacks) plugintk.RegistryAPI {
	return &staticRegistry{
		bgCtx:     context.Background(),
		callbacks: callbacks,
	}
}

func (r *staticRegistry) ConfigureRegistry(ctx context.Context, req *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error) {
	r.name = req.Name

	err := json.Unmarshal([]byte(req.ConfigJson), &r.conf)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidRegistryConfig)
	}

	// We simply publish everything here and now with the static registry.
	for nodeName, nodeRecord := range r.conf.Nodes {
		for transportName, transportRecordUnparsed := range nodeRecord.Transports {
			if err := r.registerNodeTransport(ctx, nodeName, transportName, transportRecordUnparsed); err != nil {
				return nil, err
			}
		}
	}

	return &prototk.ConfigureRegistryResponse{}, nil
}

func (r *staticRegistry) registerNodeTransport(ctx context.Context, nodeName, transportName string, transportRecordUnparsed tktypes.RawJSON) error {
	var untyped any
	err := json.Unmarshal(transportRecordUnparsed, &untyped)
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgInvalidRegistryConfig, nodeName, transportName)
	}

	// We let the config contain structured JSON (well YAML in it's original form before it was passed as JSON to us)
	// Or we let the config contain a string
	var transportDetails string
	switch v := untyped.(type) {
	case string:
		// it's already a string - so it's our details directly
		transportDetails = v
	default:
		// otherwise we preserve the JSON
		transportDetails = transportRecordUnparsed.String()
	}
	_, err = r.callbacks.UpsertTransportDetails(ctx, &prototk.UpsertTransportDetails{
		Node:             nodeName,
		Transport:        transportName,
		TransportDetails: transportDetails,
	})
	return err
}
