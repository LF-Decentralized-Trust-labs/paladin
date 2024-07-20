// Copyright © 2022 Kaleido, Inc.
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
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/stretchr/testify/assert"
)

func TestRPCMessageBatch(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &Config{})
	defer done()

	s.Register("ut_methodA", RPCMethod2(func(ctx context.Context, param0, param1 string) (string, error) {
		assert.Equal(t, "valueA0", param0)
		assert.Equal(t, "valueA1", param1)
		return "resultA", nil
	}))
	s.Register("ut_methodB", RPCMethod2(func(ctx context.Context, param0, param1 string) (string, error) {
		assert.Equal(t, "valueB0", param0)
		assert.Equal(t, "valueB1", param1)
		return "resultB", nil
	}))

	var jsonResponse json.RawMessage
	res, err := resty.New().R().
		SetBody(`[
			{
				"jsonrpc": "2.0",
				"id": "1",
				"method": "ut_methodA",
				"params": ["valueA0","valueA1"]
			},
			{
				"jsonrpc": "2.0",
				"id": "2",
				"method": "ut_methodB",
				"params": ["valueB0","valueB1"]
			}
		]`).
		SetResult(&jsonResponse).
		SetError(&jsonResponse).
		Post(url)
	assert.NoError(t, err)
	assert.True(t, res.IsSuccess())
	assert.JSONEq(t, `[
		{
			"jsonrpc": "2.0",
			"id": "1",
			"result": "resultA"
		},
		{
			"jsonrpc": "2.0",
			"id": "2",
			"result": "resultB"
		}
	]`, (string)(jsonResponse))

}

func TestRPCMessageBatchOneFails200WithError(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &Config{})
	defer done()

	s.Register("ut_methodA", RPCMethod2(func(ctx context.Context, param0, param1 string) (string, error) {
		assert.Equal(t, "valueA0", param0)
		assert.Equal(t, "valueA1", param1)
		return "resultA", nil
	}))
	s.Register("ut_methodB", RPCMethod2(func(ctx context.Context, param0, param1 string) (string, error) {
		assert.Equal(t, "valueB0", param0)
		assert.Equal(t, "valueB1", param1)
		return "", fmt.Errorf("pop")
	}))

	var jsonResponse json.RawMessage
	res, err := resty.New().R().
		SetBody(`[
			{
				"jsonrpc": "2.0",
				"id": "1",
				"method": "ut_methodA",
				"params": ["valueA0","valueA1"]
			},
			{
				"jsonrpc": "2.0",
				"id": "2",
				"method": "ut_methodB",
				"params": ["valueB0","valueB1"]
			}
		]`).
		SetResult(&jsonResponse).
		SetError(&jsonResponse).
		Post(url)
	assert.NoError(t, err)
	assert.True(t, res.IsSuccess())
	assert.JSONEq(t, `[
		{
			"jsonrpc": "2.0",
			"id": "1",
			"result": "resultA"
		},
		{
			"jsonrpc": "2.0",
			"id": "2",
			"error": {
			  "code": -32603,
			  "message": "pop"
			}
		}
	]`, (string)(jsonResponse))

}

func TestRPCMessageBatchAllFail(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &Config{})
	defer done()

	s.Register("ut_methodA", RPCMethod2(func(ctx context.Context, param0, param1 string) (string, error) {
		assert.Equal(t, "valueA0", param0)
		assert.Equal(t, "valueA1", param1)
		return "", fmt.Errorf("snap")
	}))
	s.Register("ut_methodB", RPCMethod2(func(ctx context.Context, param0, param1 string) (string, error) {
		assert.Equal(t, "valueB0", param0)
		assert.Equal(t, "valueB1", param1)
		return "", fmt.Errorf("crackle")
	}))
	s.Register("ut_methodC", RPCMethod1(func(ctx context.Context, param0 map[string]string) (string, error) {
		assert.Equal(t, map[string]string{"some": "things"}, param0)
		return "", fmt.Errorf("pop")
	}))

	var jsonResponse json.RawMessage
	res, err := resty.New().R().
		SetBody(`[
			{
				"jsonrpc": "2.0",
				"id": "1",
				"method": "ut_methodA",
				"params": ["valueA0","valueA1"]
			},
			{
				"jsonrpc": "2.0",
				"id": "2",
				"method": "ut_methodB",
				"params": ["valueB0","valueB1"]
			},
			{
				"jsonrpc": "2.0",
				"id": "3",
				"method": "ut_methodC",
				"params": [{"some":"things"}]
			}
		]`).
		SetResult(&jsonResponse).
		SetError(&jsonResponse).
		Post(url)
	assert.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.JSONEq(t, `[
		{
			"jsonrpc": "2.0",
			"id": "1",
			"error": {
			  "code": -32603,
			  "message": "snap"
			}
		},
		{
			"jsonrpc": "2.0",
			"id": "2",
			"error": {
			  "code": -32603,
			  "message": "crackle"
			}
		},
		{
			"jsonrpc": "2.0",
			"id": "3",
			"error": {
			  "code": -32603,
			  "message": "pop"
			}
		}
	]`, (string)(jsonResponse))

}

func TestRPCHandleBadDataEmptySpace(t *testing.T) {

	url, _, done := newTestServerHTTP(t, &Config{})
	defer done()

	var jsonResponse rpcbackend.RPCResponse
	res, err := resty.New().R().
		SetBody(`     `).
		SetResult(&jsonResponse).
		SetError(&jsonResponse).
		Post(url)
	assert.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcbackend.RPCCodeInvalidRequest), jsonResponse.Error.Code)
	assert.Regexp(t, "PD010800", jsonResponse.Error.Message)

}

func TestRPCHandleIOError(t *testing.T) {

	_, s, done := newTestServerHTTP(t, &Config{})
	defer done()

	iRPCResponse, ok := s.rpcHandler(context.Background(), iotest.ErrReader(fmt.Errorf("pop")))
	assert.False(t, ok)
	jsonResponse := iRPCResponse.(*rpcbackend.RPCResponse)
	assert.Equal(t, int64(rpcbackend.RPCCodeInvalidRequest), jsonResponse.Error.Code)
	assert.Regexp(t, "PD010800", jsonResponse.Error.Message)

}

func TestRPCBadArrayError(t *testing.T) {

	_, s, done := newTestServerHTTP(t, &Config{})
	defer done()

	iRPCResponse, ok := s.rpcHandler(context.Background(), strings.NewReader("[... this is not an array"))
	assert.False(t, ok)
	jsonResponse := iRPCResponse.(*rpcbackend.RPCResponse)
	assert.Equal(t, int64(rpcbackend.RPCCodeInvalidRequest), jsonResponse.Error.Code)
	assert.Regexp(t, "PD010800", jsonResponse.Error.Message)

}
