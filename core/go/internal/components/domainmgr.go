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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"gorm.io/gorm"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type DomainManagerToDomain interface {
	plugintk.DomainAPI
	Initialized()
}

// Domain manager is the boundary between the paladin core / testbed and the domains
type DomainManager interface {
	ManagerLifecycle
	ConfiguredDomains() map[string]*pldconf.PluginConfig
	DomainRegistered(name string, toDomain DomainManagerToDomain) (fromDomain plugintk.DomainCallbacks, err error)
	GetDomainByName(ctx context.Context, name string) (Domain, error)
	GetSmartContractByAddress(ctx context.Context, dbTX *gorm.DB, addr tktypes.EthAddress) (DomainSmartContract, error)
	ExecDeployAndWait(ctx context.Context, txID uuid.UUID, call func() error) (dc DomainSmartContract, err error)
	ExecAndWaitTransaction(ctx context.Context, txID uuid.UUID, call func() error) error
	GetSigner() signerapi.InMemorySigner
}

// External interface for other components (engine, testbed) to call against a domain
type Domain interface {
	Initialized() bool
	Name() string
	RegistryAddress() *tktypes.EthAddress
	Configuration() *prototk.DomainConfig
	CustomHashFunction() bool

	InitDeploy(ctx context.Context, tx *PrivateContractDeploy) error
	PrepareDeploy(ctx context.Context, tx *PrivateContractDeploy) error

	// The state manager calls this when states are received for a domain that has a custom hash function.
	// Any nil IDs should be filled in, and any mis-matched IDs should result in an error
	ValidateStateHashes(ctx context.Context, states []*FullState) ([]tktypes.HexBytes, error)

	GetDomainReceipt(ctx context.Context, dbTX *gorm.DB, txID uuid.UUID) (tktypes.RawJSON, error)
	BuildDomainReceipt(ctx context.Context, dbTX *gorm.DB, txID uuid.UUID, txStates *pldapi.TransactionStates) (tktypes.RawJSON, error)
}

// External interface for other components to call against a private smart contract
type DomainSmartContract interface {
	Domain() Domain
	Address() tktypes.EthAddress
	ContractConfig() *prototk.ContractConfig

	InitTransaction(ctx context.Context, ptx *PrivateTransaction, localTx *ResolvedTransaction) error
	AssembleTransaction(dCtx DomainContext, readTX *gorm.DB, ptx *PrivateTransaction) error
	WritePotentialStates(dCtx DomainContext, readTX *gorm.DB, tx *PrivateTransaction) error
	LockStates(dCtx DomainContext, readTX *gorm.DB, tx *PrivateTransaction) error
	EndorseTransaction(dCtx DomainContext, readTX *gorm.DB, req *PrivateTransactionEndorseRequest) (*EndorsementResult, error)
	PrepareTransaction(dCtx DomainContext, readTX *gorm.DB, tx *PrivateTransaction) error

	InitCall(ctx context.Context, tx *ResolvedTransaction) ([]*prototk.ResolveVerifierRequest, error)
	ExecCall(dCtx DomainContext, readTX *gorm.DB, tx *ResolvedTransaction, verifiers []*prototk.ResolvedVerifier) (*abi.ComponentValue, error)
}

type EndorsementResult struct {
	Endorser     *prototk.ResolvedVerifier
	Result       prototk.EndorseTransactionResponse_Result
	Payload      []byte
	RevertReason *string
}
