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

package extensions

import (
	"context"

	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

type zetoKeystoreSigner struct{}

func (ks *zetoKeystoreSigner) FindOrCreateKey_snark(ctx context.Context, req *proto.ResolveKeyRequest) (addr *babyjub.PublicKeyComp, keyHandle string, err error) {
	return nil, "", nil
}

func (ks *zetoKeystoreSigner) Prove_snark(ctx context.Context, keyHandle string, payload []byte) (*types.ZKProof, string, error) {
	return nil, "", nil
}
