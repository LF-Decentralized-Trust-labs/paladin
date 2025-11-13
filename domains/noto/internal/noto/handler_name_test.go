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

func TestNameValidateParams(t *testing.T) {
	h := nameHandler{}
	ctx := context.Background()

	result, err := h.ValidateParams(ctx, nil, "")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestNameInitCall(t *testing.T) {
	n := &Noto{
		Callbacks: mockCallbacks,
	}
	h := nameHandler{noto: n}
	ctx := context.Background()

	parsedTx := &types.ParsedTransaction{
		Params: nil, // name() has no parameters
		DomainConfig: &types.NotoParsedConfig{
			Name: "TestToken",
		},
	}

	req := &prototk.InitCallRequest{}
	res, err := h.InitCall(ctx, parsedTx, req)

	assert.NoError(t, err)
	assert.Empty(t, res.RequiredVerifiers) // name() doesn't require any verifiers
}

func TestNameExecCall(t *testing.T) {
	n := &Noto{
		Callbacks: mockCallbacks,
	}
	h := nameHandler{noto: n}
	ctx := context.Background()

	parsedTx := &types.ParsedTransaction{
		Params: nil,
		DomainConfig: &types.NotoParsedConfig{
			Name: "MyTestToken",
		},
	}

	req := &prototk.ExecCallRequest{}
	res, err := h.ExecCall(ctx, parsedTx, req)

	assert.NoError(t, err)
	assert.Equal(t, `"MyTestToken"`, res.ResultJson)
}
