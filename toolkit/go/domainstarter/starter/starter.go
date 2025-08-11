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
package starter

import (
	"C"
)
import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

type starter struct {
	callbacks plugintk.DomainCallbacks
	plugintk.DomainAPIBase
}

func NewStarter(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
	s := &starter{callbacks: callbacks}
	s.Functions = &plugintk.DomainAPIFunctions{
		ConfigureDomain: s.configureDomain,
		InitDomain:      s.initDomain,
	}
	return s
}

func (s *starter) configureDomain(ctx context.Context, cdr *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
	return &prototk.ConfigureDomainResponse{
		// This is a useless, but valid, example
		DomainConfig: &prototk.DomainConfig{
			AbiStateSchemasJson: []string{},
		},
	}, nil
}

func (s *starter) initDomain(ctx context.Context, idr *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
	return &prototk.InitDomainResponse{}, nil
}
