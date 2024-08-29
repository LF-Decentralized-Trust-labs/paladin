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

package zeto

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

const testConfig = `
contracts:
  factory:
    abiAndBytecode:
      path: ./abis/ZetoFactory.json
    implementations:
      - name: Zeto_Anon
        abiAndBytecode:
          path: ./abis/Zeto_Anon.json
        libraries:
          - CommonLib
          - Groth16Verifier_Anon
          - Groth16Verifier_CheckHashesValue
          - Groth16Verifier_CheckInputsOutputsValue
      - name: Zeto_AnonNullifier
        abiAndBytecode:
          path: ./abis/Zeto_AnonNullifier.json
        libraries:
          - CommonLib
          - Groth16Verifier_Anon
          - Groth16Verifier_CheckHashesValue
          - Groth16Verifier_CheckInputsOutputsValue
					- SmtLib
      - name: CommonLib
        abiAndBytecode:
          path: ./abis/CommonLib.json
      - name: Groth16Verifier_Anon
        abiAndBytecode:
          path: ./abis/Groth16Verifier_Anon.json
      - name: Groth16Verifier_CheckHashesValue
        abiAndBytecode:
          path: ./abis/Groth16Verifier_CheckHashesValue.json
      - name: Groth16Verifier_CheckInputsOutputsValue
        abiAndBytecode:
          path: ./abis/Groth16Verifier_CheckInputsOutputsValue.json
			- name: SmtLib
				abiAndBytecode:
					path: ./abis/SmtLib.json
				libraries:
					- PoseidonLib2
			- name: PoseidonLib2
				abiAndBytecode:
					path: ./abis/PoseidonLib2.json
`

func TestSortContracts(t *testing.T) {
	var config ZetoDomainConfig
	err := yaml.Unmarshal([]byte(testConfig), &config)
	assert.NoError(t, err)

	contracts, err := sortContracts(&config)
	assert.NoError(t, err)
	fmt.Println(contracts)
}
