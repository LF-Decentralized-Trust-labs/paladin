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
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRCPMissingID(t *testing.T) {

	url, _, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeInvalidRequest), errResponse.Error.Code)
	assert.Regexp(t, "PD020701", errResponse.Error.Message)

}

func TestRCPUnknownMethod(t *testing.T) {

	url, _, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	var errResponse rpcclient.RPCResponse
	res, err := resty.New().R().
		SetBody(`{
		  "id": 12345,
		  "method": "wrong"
		}`).
		SetError(&errResponse).
		Post(url)
	require.NoError(t, err)
	assert.False(t, res.IsSuccess())
	assert.Equal(t, int64(rpcclient.RPCCodeInvalidRequest), errResponse.Error.Code)
	assert.Regexp(t, "PD020702", errResponse.Error.Message)

}
