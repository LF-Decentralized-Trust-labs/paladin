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

package secp256k1

import (
	"context"

	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/signer/common"
)

type sepc256k1Signer struct{}

func Register(registry map[string]api.InMemorySigner) {
	signer := &sepc256k1Signer{}
	registry[api.Algorithm_ECDSA_SECP256K1_PLAINBYTES] = signer
}

func (s *sepc256k1Signer) Sign(ctx context.Context, privateKey []byte, req *proto.SignRequest) (*proto.SignResponse, error) {
	kp := secp256k1.KeyPairFromBytes(privateKey)
	sig, err := kp.SignDirect(req.Payload)
	if err == nil {
		return &proto.SignResponse{Payload: common.CompactRSV(sig)}, nil
	}
	return nil, err
}
