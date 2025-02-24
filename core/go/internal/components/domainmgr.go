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

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
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
	GetSmartContractByAddress(ctx context.Context, dbTX persistence.DBTX, addr tktypes.EthAddress) (DomainSmartContract, error)
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

	// Specific to domains that support privacy groups (domain should return error if it does not).
	// Validates the input properties, and turns it into the full genesis configuration for a group
	InitPrivacyGroup(ctx context.Context, pgInput *pldapi.PrivacyGroupInput) (tx *PreparedGroupInitTransaction, err error)

	InitDeploy(ctx context.Context, tx *PrivateContractDeploy) error
	PrepareDeploy(ctx context.Context, tx *PrivateContractDeploy) error

	// The state manager calls this when states are received for a domain that has a custom hash function.
	// Any nil IDs should be filled in, and any mis-matched IDs should result in an error
	ValidateStateHashes(ctx context.Context, states []*FullState) ([]tktypes.HexBytes, error)

	GetDomainReceipt(ctx context.Context, dbTX persistence.DBTX, txID uuid.UUID) (tktypes.RawJSON, error)
	BuildDomainReceipt(ctx context.Context, dbTX persistence.DBTX, txID uuid.UUID, txStates *pldapi.TransactionStates) (tktypes.RawJSON, error)
}

// External interface for other components to call against a private smart contract
type DomainSmartContract interface {
	Domain() Domain
	Address() tktypes.EthAddress
	ContractConfig() *prototk.ContractConfig

	InitTransaction(ctx context.Context, ptx *PrivateTransaction, localTx *ResolvedTransaction) error
	AssembleTransaction(dCtx DomainContext, readTX persistence.DBTX, ptx *PrivateTransaction, localTx *ResolvedTransaction) error
	WritePotentialStates(dCtx DomainContext, readTX persistence.DBTX, tx *PrivateTransaction) error
	LockStates(dCtx DomainContext, readTX persistence.DBTX, tx *PrivateTransaction) error
	EndorseTransaction(dCtx DomainContext, readTX persistence.DBTX, req *PrivateTransactionEndorseRequest) (*EndorsementResult, error)
	PrepareTransaction(dCtx DomainContext, readTX persistence.DBTX, tx *PrivateTransaction) error

	InitCall(ctx context.Context, tx *ResolvedTransaction) ([]*prototk.ResolveVerifierRequest, error)
	ExecCall(dCtx DomainContext, readTX persistence.DBTX, tx *ResolvedTransaction, verifiers []*prototk.ResolvedVerifier) (*abi.ComponentValue, error)

	WrapPrivacyGroupTransaction(ctx context.Context, pg *pldapi.PrivacyGroupWithABI, fnDef *abi.Entry, pgInput *pldapi.TransactionInput) (err error)
}

type DomainPrivacyGroupConfig struct {
	DefaultSchemaABI *abi.Parameter
}

type EndorsementResult struct {
	Endorser     *prototk.ResolvedVerifier
	Result       prototk.EndorseTransactionResponse_Result
	Payload      []byte
	RevertReason *string
}
