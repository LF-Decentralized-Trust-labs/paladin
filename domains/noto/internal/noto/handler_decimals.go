/*
 * Copyright © 2025 Kaleido, Inc.
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

package noto

import (
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type decimalsHandler struct {
	noto *Noto
}

func (h *decimalsHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	return nil, nil
}

func (h *decimalsHandler) InitCall(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
	return &prototk.InitCallResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{},
	}, nil
}

func (h *decimalsHandler) ExecCall(ctx context.Context, tx *types.ParsedTransaction, req *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
	decimalsJson, err := json.Marshal(tx.DomainConfig.Decimals)
	if err != nil {
		return nil, err
	}

	return &prototk.ExecCallResponse{
		ResultJson: string(decimalsJson),
	}, nil
}
