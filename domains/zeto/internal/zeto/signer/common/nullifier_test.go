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

package common

import (
	"math/big"
	"testing"

	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/stretchr/testify/assert"
)

func TestCalculateNullifier(t *testing.T) {
	value := big.NewInt(123)
	salt := big.NewInt(456)
	_, _, privKey := newKeypair()

	nullifier, err := CalculateNullifier(value, salt, privKey)
	assert.NoError(t, err)

	expectedNullifier, err := poseidon.Hash([]*big.Int{value, salt, privKey})
	assert.NoError(t, err)
	assert.Equal(t, 0, nullifier.Cmp(expectedNullifier))

	tooBig, ok := new(big.Int).SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
	assert.True(t, ok)
	_, err = CalculateNullifier(value, salt, tooBig)
	assert.EqualError(t, err, "inputs values not inside Finite Field")
}

func newKeypair() (*babyjub.PrivateKey, *babyjub.PublicKey, *big.Int) {
	// generate babyJubjub private key randomly
	babyJubjubPrivKey := babyjub.NewRandPrivKey()
	// generate public key from private key
	babyJubjubPubKey := babyJubjubPrivKey.Public()
	// convert the private key to big.Int for use inside circuits
	privKeyBigInt := babyjub.SkToBigInt(&babyJubjubPrivKey)

	return &babyJubjubPrivKey, babyJubjubPubKey, privKeyBigInt
}
