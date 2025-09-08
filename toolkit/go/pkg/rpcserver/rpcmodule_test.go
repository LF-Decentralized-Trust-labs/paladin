// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpcserver

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRCPModule(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	s.Register(NewRPCModule("example").
		Add("example_test1", RPCMethod0(func(ctx context.Context) (string, error) {
			return "result0", nil
		})),
	)

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "example_test1",
		  "params": []
		}`).
		SetResult(&jsonResponse).
		SetError(&jsonResponse).
		Post(url)
	require.NoError(t, err)
	assert.True(t, res.IsSuccess())
	assert.JSONEq(t, `{
		"jsonrpc": "2.0",
		"id": "1",
		"result": "result0"
	}`, (string)(jsonResponse))

}

func TestRCPModulePanicOutsideModule(t *testing.T) {
	assert.Panics(t, func() {
		_ = NewRPCModule("example").
			Add("wrong_test1", RPCMethod0(func(ctx context.Context) (string, error) {
				return "result0", nil
			}))
	})

}

func TestRCPModulePanicDup(t *testing.T) {
	assert.Panics(t, func() {
		_ = NewRPCModule("example").
			Add("example_test1", RPCMethod0(func(ctx context.Context) (string, error) {
				return "result0", nil
			})).
			Add("example_test1", RPCMethod0(func(ctx context.Context) (string, error) {
				return "result0", nil
			}))
	})

}
