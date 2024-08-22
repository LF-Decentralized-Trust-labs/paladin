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

package components

import (
	"context"

	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

// Domain manager is the boundary between the paladin core / testbed and the domains
type DomainManager interface {
	ManagerLifecycle
	plugins.DomainRegistration
	GetDomainByName(ctx context.Context, name string) (Domain, error)
}

// External interface for other components (engine, testbed) to call against a domain
type Domain interface {
	Initialized() bool
	Name() string
	Address() *types.EthAddress
	GetSmartContractByAddress(ctx context.Context, addr types.EthAddress) (DomainSmartContract, error)
	Configuration() *prototk.DomainConfig

	InitDeploy(ctx context.Context, tx *PrivateContractDeploy) error
	PrepareDeploy(ctx context.Context, tx *PrivateContractDeploy) error
}

// External interface for other components to call against a private smart contract
type DomainSmartContract interface {
	Domain() Domain
	Address() types.EthAddress
	ConfigBytes() []byte

	InitTransaction(ctx context.Context, tx *PrivateTransaction) error
	AssembleTransaction(ctx context.Context, tx *PrivateTransaction) error
	WritePotentialStates(ctx context.Context, tx *PrivateTransaction) error
	LockStates(ctx context.Context, tx *PrivateTransaction) error
	EndorseTransaction(ctx context.Context, tx *PrivateTransaction, endorser *prototk.ResolvedVerifier) (*EndorsementResult, error)
	PrepareTransaction(ctx context.Context, tx *PrivateTransaction) error
}

type EndorsementResult struct {
	Endorser     *prototk.ResolvedVerifier
	Result       prototk.EndorseTransactionResponse_Result
	Payload      []byte
	RevertReason *string
}
