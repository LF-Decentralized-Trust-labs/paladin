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
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestDecimalsValidateParams(t *testing.T) {
	h := decimalsHandler{}
	ctx := context.Background()

	result, err := h.ValidateParams(ctx, nil, "")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestDecimalsInitCall(t *testing.T) {
	n := &Noto{
		Callbacks: mockCallbacks,
	}
	h := decimalsHandler{noto: n}
	ctx := context.Background()

	parsedTx := &types.ParsedTransaction{
		Params: nil, // decimals() has no parameters
		DomainConfig: &types.NotoParsedConfig{
			Decimals: 4,
		},
	}

	req := &prototk.InitCallRequest{}
	res, err := h.InitCall(ctx, parsedTx, req)

	assert.NoError(t, err)
	assert.Empty(t, res.RequiredVerifiers) // decimals() doesn't require any verifiers
}

func TestDecimalsExecCall(t *testing.T) {
	n := &Noto{
		Callbacks: mockCallbacks,
	}
	h := decimalsHandler{noto: n}
	ctx := context.Background()

	parsedTx := &types.ParsedTransaction{
		Params: nil,
		DomainConfig: &types.NotoParsedConfig{
			Decimals: 4,
		},
	}

	req := &prototk.ExecCallRequest{}
	res, err := h.ExecCall(ctx, parsedTx, req)

	assert.NoError(t, err)
	assert.Equal(t, `"0x4"`, res.ResultJson)
}
