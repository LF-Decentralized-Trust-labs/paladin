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

package evmregistry

import (
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
)

func TestDetectsContractChangeAtBuildTime(t *testing.T) {

	assert.PanicsWithValue(t, "contract signature has changed: event IdentityRegistered(address different)", func() {
		mustLoadIdentityRegistryContractDetail(pldtypes.JSONString(SolidityBuild{
			ABI: abi.ABI{
				{
					Type: abi.Event,
					Name: "IdentityRegistered",
					Inputs: abi.ParameterArray{
						{Name: "different", Type: "address"},
					},
				},
			},
		}))
	})

	assert.PanicsWithValue(t, "contract signature has changed: event PropertySet(address different)", func() {
		mustLoadIdentityRegistryContractDetail(pldtypes.JSONString(SolidityBuild{
			ABI: abi.ABI{
				contractDetail.abi.Events()["IdentityRegistered"],
				{
					Type: abi.Event,
					Name: "PropertySet",
					Inputs: abi.ParameterArray{
						{Name: "different", Type: "address"},
					},
				},
			},
		}))
	})

}

func TestBreaksIfBuildIsBroken(t *testing.T) {
	assert.Panics(t, func() {
		mustLoadIdentityRegistryContractDetail([]byte(`{!!! broken JSON`))
	})
}
