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

package signer

import (
	"context"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/core/pkg/proto"
)

// Some cryptographic storage supports signing directly with secp256k1 curve and an ECDSA algorithm,
// which is a core signing function used in many Paladin domains, and during base EVM signing.
//
// Because an administrator might require certain wallets are ONLY used this way, there is an
// option on all wallets to require it. In which case (even though it's always supported)
// that wallet will reject any signing/proof-generation request that requires uses a loadable key.
type KeyStoreSigner_secp256k1 interface {
	FindOrCreateKey_secp256k1(ctx context.Context, req *proto.ResolveKeyRequest) (addr *ethtypes.Address0xHex, keyHandle string, err error)
	Sign_secp256k1(ctx context.Context, keyHandle string, payload []byte) (*secp256k1.SignatureData, error)
}
