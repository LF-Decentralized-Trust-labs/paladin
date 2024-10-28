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

package zetosigner

import (
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/signer"
)

func EncodeBabyJubJubPublicKey(pubKey *babyjub.PublicKey) string {
	return signer.EncodeBabyJubJubPublicKey(pubKey)
}

func DecodeBabyJubJubPublicKey(pubKeyHex string) (*babyjub.PublicKey, error) {
	return signer.DecodeBabyJubJubPublicKey(pubKeyHex)
}

func NewBabyJubJubPrivateKey(privateKey []byte) (*babyjub.PrivateKey, error) {
	return signer.NewBabyJubJubPrivateKey(privateKey)
}
