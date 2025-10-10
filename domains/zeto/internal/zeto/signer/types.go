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

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/witness"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
)

type witnessInputs interface {
	Validate(ctx context.Context, inputs *pb.ProvingRequestCommon) error
	Build(ctx context.Context, commonInputs *pb.ProvingRequestCommon) error
	Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error)
}

var _ witnessInputs = &witness.FungibleWitnessInputs{}

var _ witnessInputs = &witness.DepositWitnessInputs{}

var _ witnessInputs = &witness.LockWitnessInputs{}

var _ witnessInputs = &witness.FungibleEncWitnessInputs{}

var _ witnessInputs = &witness.FungibleNullifierWitnessInputs{}

var _ witnessInputs = &witness.NonFungibleWitnessInputs{}
