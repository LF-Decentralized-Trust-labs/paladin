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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/registries/static/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"golang.org/x/crypto/sha3"
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

func NewPlugin() plugintk.PluginBase {
	return plugintk.NewRegistry(NewStatic)
}

func NewStatic(callbacks plugintk.RegistryCallbacks) plugintk.RegistryAPI {
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
	upsert := &prototk.UpsertRegistryRecordsRequest{}
	for name, entry := range r.conf.Entries {
		if err == nil {
			err = r.recurseBuildUpsert(ctx, upsert, nil, name, entry)
		}
	}
	if err == nil {
		_, err = r.callbacks.UpsertRegistryRecords(ctx, upsert)
	}
	if err != nil {
		return nil, err
	}
	return &prototk.ConfigureRegistryResponse{
		RegistryConfig: &prototk.RegistryConfig{},
	}, nil
}

func (r *staticRegistry) HandleRegistryEvents(ctx context.Context, req *prototk.HandleRegistryEventsRequest) (*prototk.HandleRegistryEventsResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgFunctionUnsupported)
}

func (r *staticRegistry) recurseBuildUpsert(ctx context.Context, req *prototk.UpsertRegistryRecordsRequest, parentID pldtypes.HexBytes, name string, inEntry *StaticEntry) error {

	idHash := sha3.NewLegacyKeccak256()
	if parentID != nil {
		idHash.Write([]byte(parentID))
	}
	idHash.Write([]byte(name))
	entryID := pldtypes.HexBytes(idHash.Sum(nil))
	entry := prototk.RegistryEntry{
		Id:       entryID.String(),
		Name:     name,
		ParentId: parentID.String(),
		Active:   true,
	}
	properties := make([]*prototk.RegistryProperty, 0, len(inEntry.Properties))
	for propName, jsonValue := range inEntry.Properties {
		var untyped any
		err := json.Unmarshal(jsonValue, &untyped)
		if err != nil {
			return i18n.WrapError(ctx, err, msgs.MsgInvalidRegistryConfig)
		}

		// We let the config contain structured JSON (well YAML in it's original form before it was passed as JSON to us)
		// Or we let the config contain a string
		var strValue string
		switch v := untyped.(type) {
		case string:
			// it's already a string - so it's our details directly
			strValue = v
		default:
			// otherwise we preserve the JSON
			strValue = jsonValue.String()
		}

		log.L(ctx).Infof("Registering %s prop=%s (parentId=%s)", entry.Name, propName, parentID)
		properties = append(properties, &prototk.RegistryProperty{
			EntryId: entry.Id,
			Name:    propName,
			Value:   strValue,
			Active:  true,
		})
	}
	req.Entries = append(req.Entries, &entry)
	req.Properties = append(req.Properties, properties...)

	for childName, child := range inEntry.Children {
		if err := r.recurseBuildUpsert(ctx, req, entryID, childName, child); err != nil {
			return err
		}
	}

	return nil
}
