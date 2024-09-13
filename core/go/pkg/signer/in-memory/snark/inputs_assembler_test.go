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

package snark

import (
	"math/big"
	"testing"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/stretchr/testify/assert"
)

func TestAssembleInputsAnonEnc(t *testing.T) {
	inputs := commonWitnessInputs{}
	key := core.KeyEntry{}
	_, publicInputs, err := assembleInputs_anon_enc(&inputs, nil, &key)
	assert.NoError(t, err)
	_, ok := new(big.Int).SetString(publicInputs["encryptionNonce"], 10)
	assert.True(t, ok)
}

func TestAssembleInputsAnonEnc_fail(t *testing.T) {
	inputs := commonWitnessInputs{}
	extras := proto.ProvingRequestExtras_Encryption{
		EncryptionNonce: "1234",
	}
	key := core.KeyEntry{}
	_, publicInputs, err := assembleInputs_anon_enc(&inputs, &extras, &key)
	assert.NoError(t, err)
	assert.Equal(t, "1234", publicInputs["encryptionNonce"])

	extras.EncryptionNonce = "bad number"
	_, _, err = assembleInputs_anon_enc(&inputs, &extras, &key)
	assert.EqualError(t, err, "failed to parse encryption nonce")
}
