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
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRCPMethod0(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod0(func(ctx context.Context) (string, error) {
		return "result0", nil
	}))

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
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

func TestRCPMethod1(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod1(func(ctx context.Context, param0 string) (string, error) {
		assert.Equal(t, "value0", param0)
		return "result0", nil
	}))

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [
		    "value0"
		  ]
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

func TestRCPMethod2(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod2(func(ctx context.Context, param0 string, param1 string) (string, error) {
		assert.Equal(t, "value0", param0)
		assert.Equal(t, "value1", param1)
		return "result0", nil
	}))

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [
		    "value0",
		    "value1"
		  ]
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

func TestRCPMethod3(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod3(func(ctx context.Context, param0 string, param1 string, param2 string) (string, error) {
		assert.Equal(t, "value0", param0)
		assert.Equal(t, "value1", param1)
		assert.Equal(t, "value2", param2)
		return "result0", nil
	}))

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [
		    "value0",
		    "value1",
		    "value2"
		  ]
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

func TestRCPMethod4(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod4(func(ctx context.Context, param0 string, param1 string, param2 string, param3 string) (string, error) {
		assert.Equal(t, "value0", param0)
		assert.Equal(t, "value1", param1)
		assert.Equal(t, "value2", param2)
		assert.Equal(t, "value3", param3)
		return "result0", nil
	}))

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [
		    "value0",
		    "value1",
		    "value2",
		    "value3"
		  ]
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

func TestRCPMethod5(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod5(func(ctx context.Context, param0 string, param1 string, param2 string, param3 string, param4 string) (string, error) {
		assert.Equal(t, "value0", param0)
		assert.Equal(t, "value1", param1)
		assert.Equal(t, "value2", param2)
		assert.Equal(t, "value3", param3)
		assert.Equal(t, "value4", param4)
		return "result0", nil
	}))

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [
		    "value0",
		    "value1",
		    "value2",
		    "value3",
		    "value4"
		  ]
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

func TestRCPMethodNullParamPointerPassed(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod2(func(ctx context.Context, param0 *string, param1 *ethtypes.Address0xHex) (string, error) {
		assert.Nil(t, param0)
		assert.Nil(t, param1)
		return "result0", nil
	}))

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [
		    null,
			null
		  ]
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

func TestRCPMethodNullParamNonPointerEmptyVal(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod2(func(ctx context.Context, param0 string, param1 ethtypes.Address0xHex) (string, error) {
		assert.Empty(t, param0)
		assert.Equal(t, "0x0000000000000000000000000000000000000000", param1.String())
		return "result0", nil
	}))

	var jsonResponse pldtypes.RawJSON
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [
		    null,
			null
		  ]
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

func TestRCPMethodInvalidValue(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod1(func(ctx context.Context, param0 []string) (string, error) {
		assert.Fail(t, "should not be called")
		return "", nil
	}))

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [ "not an array" ]
		}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeInvalidRequest), errResponse.Error.Code)
	assert.Regexp(t, "PD020704", errResponse.Error.Message)

}

func TestRCPMethodWrongParamCount(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod1(func(ctx context.Context, param0 string) (string, error) {
		assert.Fail(t, "should not be called")
		return "", nil
	}))

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [ "more", "than", "one" ]
		}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeInvalidRequest), errResponse.Error.Code)
	assert.Regexp(t, "PD020703", errResponse.Error.Message)

}

func TestRCPMethodBadResult(t *testing.T) {

	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	regTestRPC(s, "stringy_method", RPCMethod0(func(ctx context.Context) (map[bool]bool, error) {
		return map[bool]bool{false: true} /* good luck JSON */, nil
	}))

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{
		  "jsonrpc": "2.0",
		  "id": "1",
		  "method": "stringy_method",
		  "params": [ ]
		}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeInternalError), errResponse.Error.Code)
	assert.Regexp(t, "PD020705", errResponse.Error.Message)

}
