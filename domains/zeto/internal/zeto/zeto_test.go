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
	"context"
	"encoding/json"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestDecodeDomainConfig(t *testing.T) {
	config := &types.DomainInstanceConfig{
		CircuitId: "circuit-id",
		TokenName: "token-name",
	}
	configJSON, err := json.Marshal(config)
	assert.NoError(t, err)

	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)

	z := &Zeto{name: "z1"}
	decoded, err := z.decodeDomainConfig(context.Background(), encoded)
	assert.NoError(t, err)
	assert.Equal(t, config, decoded)

	assert.Equal(t, z.getAlgoZetoSnarkBJJ(), "domain:z1:snark:babyjubjub")
}