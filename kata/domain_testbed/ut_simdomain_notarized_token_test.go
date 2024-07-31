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

package main

import (
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
	pb "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Example of how someone might use this testbed externally
func TestDemoNotarizedCoinSelection(t *testing.T) {

	fakeFactoryABI := `[
	  {
	    "type": "function",
		"name": "newToken",
		"inputs": [
			{
				"name": "notary",
				"type": "address"
			},
			{
				"name": "name",
				"type": "string"
			},
			{
				"name": "symbol",
				"type": "string"
			}
		],
		"outputs": [
		    {
				"type": "address"
		    }
		]
	  }
	]`

	fakeCoinConstructorABI := `{
		"type": "constructor",
		"inputs": [
		  {
		    "name": "notary",
			"type": "address"
		  },
		  {
		    "name": "name",
			"type": "string"
		  },
		  {
		    "name": "symbol",
			"type": "string"
		  }
		],
		"outputs": null
	}`

	fakeCoinStateSchema := `{
		"type": "tuple",
		"internalType": "struct FakeCoin",
		"components": [
			{
				"name": "salt",
				"type": "bytes32"
			},
			{
				"name": "owner",
				"type": "address",
				"indexed": true
			},
			{
				"name": "amount",
				"type": "uint256",
				"indexed": true
			}
		]
	}`

	rpcCall, done := newDomainSimulator(t, map[protoreflect.FullName]domainSimulatorFn{

		CONFIGURE: func(iReq pb.Message) (pb.Message, error) {
			req := simRequestToProto[*proto.ConfigureDomainRequest](t, iReq)
			assert.Equal(t, "domain1", req.Name)
			assert.JSONEq(t, `{"some":"config"}`, req.ConfigYaml)
			assert.Equal(t, int64(1122334455), req.ChainId) // fake
			return &proto.ConfigureDomainResponse{
				DomainConfig: &proto.DomainConfig{
					ConstructorAbiJson:     fakeCoinConstructorABI,
					FactoryContractAddress: "0x9D4Ee5Af51AA4e61602ea2A4Fe22A3Ca5c65c027", // fake
					FactoryContractAbiJson: fakeFactoryABI,
					AbiStateSchemasJson:    []string{fakeCoinStateSchema},
				},
			}, nil
		},

		INIT_DOMAIN: func(iReq pb.Message) (pb.Message, error) {
			req := simRequestToProto[*proto.InitDomainRequest](t, iReq)
			assert.Len(t, req.AbiStateSchemaIds, 1)
			return &proto.InitDomainResponse{}, nil
		},

		PREPARE_DEPLOY: func(iReq pb.Message) (pb.Message, error) {
			req := simRequestToProto[*proto.PrepareDeployTransactionRequest](t, iReq)
			assert.JSONEq(t, fakeCoinConstructorABI, req.ConstructorAbi)
			assert.JSONEq(t, `{
				"notary": "0x6a0969a486aefa82b3f7d7b4ced1c4d578bf2d81",
				"name": "FakeToken1",
				"symbol": "FT1"
			}`, req.ConstructorParamsJson)
			return &proto.PrepareDeployTransactionResponse{}, nil
		},
	})
	defer done()

	err := rpcCall("testbed_configureInit", "domain1", types.RawJSON(`{
		"some": "config"
	}`))
	assert.NoError(t, err)

	err = rpcCall("testbed_deploy", "domain1", types.RawJSON(`{
		"notary": "0x6a0969a486aEFa82b3F7D7B4cEd1c4d578bf2D81",
		"name": "FakeToken1",
		"symbol": "FT1"
	}`))
	assert.NoError(t, err)

}
