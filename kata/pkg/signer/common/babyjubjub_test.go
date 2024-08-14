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
	"testing"

	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/stretchr/testify/assert"
)

func TestEncodeDecode(t *testing.T) {
	privKey := babyjub.NewRandPrivKey()
	pubKey := privKey.Public()
	pubKeyHex := EncodePublicKey(pubKey)
	pubKey2, _ := DecodePublicKey(pubKeyHex)
	assert.Equal(t, pubKey.X, pubKey2.X)
	assert.Equal(t, pubKey.Y, pubKey2.Y)
}

func TestDecodeFail(t *testing.T) {
	_, err := DecodePublicKey("bad")
	assert.Error(t, err)
}
