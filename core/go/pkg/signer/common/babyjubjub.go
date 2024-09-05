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
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/utils"
)

func EncodePublicKey(pubKey *babyjub.PublicKey) string {
	pubKeyComp := pubKey.Compress()
	return utils.HexEncode(pubKeyComp[:])
}

func DecodePublicKey(pubKeyHex string) (*babyjub.PublicKey, error) {
	pubKeyCompBytes, err := utils.HexDecode(pubKeyHex)
	if err != nil {
		return nil, err
	}
	var compressedPubKey babyjub.PublicKeyComp
	copy(compressedPubKey[:], pubKeyCompBytes)
	return compressedPubKey.Decompress()
}
