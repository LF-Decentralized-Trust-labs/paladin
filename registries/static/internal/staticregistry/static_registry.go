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
	return &prototk.ConfigureRegistryResponse{}, nil
}
