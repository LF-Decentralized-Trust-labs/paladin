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

package dependencies

import (
	"context"
	"net"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	prototk "github.com/kaleido-io/paladin/kata/pkg/proto/toolkit"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"gorm.io/gorm"
)

// this file is completey throwaway, it is only here in place of the interfaces that are being defined in branch `plugins` under https://github.com/kaleido-io/paladin/tree/plugins/kata/internal/components

/* From "github.com/kaleido-io/paladin/kata/pkg/ethclient" */
type EthTXVersion string
type KeyManager interface {
	ResolveKey(ctx context.Context, identifier string, algorithm string) (keyHandle, verifier string, err error)
	Sign(ctx context.Context, req *proto.SignRequest) (*proto.SignResponse, error)
	Close()
}
type ABIClient interface {
	ABI() abi.ABI
	Function(ctx context.Context, nameOrFullSig string) (_ ABIFunctionClient, err error)
	MustFunction(nameOrFullSig string) ABIFunctionClient
	Constructor(ctx context.Context, bytecode ethtypes.HexBytes0xPrefix) (_ ABIFunctionClient, err error)
	MustConstructor(bytecode ethtypes.HexBytes0xPrefix) ABIFunctionClient
}
type ABIFunctionClient interface {
	R(ctx context.Context) ABIFunctionRequestBuilder
}
type BlockRef string
type ABIFunctionRequestBuilder interface {
	// Builder functions
	TXVersion(EthTXVersion) ABIFunctionRequestBuilder
	Signer(string) ABIFunctionRequestBuilder
	To(*ethtypes.Address0xHex) ABIFunctionRequestBuilder
	GasLimit(uint64) ABIFunctionRequestBuilder
	BlockRef(blockRef BlockRef) ABIFunctionRequestBuilder
	Block(uint64) ABIFunctionRequestBuilder
	Input(any) ABIFunctionRequestBuilder
	Output(any) ABIFunctionRequestBuilder

	// Query functions
	TX() *ethsigner.Transaction

	// Execution functions
	BuildCallData() (err error)
	Call() (err error)
	CallJSON() (jsonData []byte, err error)
	RawTransaction() (rawTX ethtypes.HexBytes0xPrefix, err error)
	SignAndSend() (txHash ethtypes.HexBytes0xPrefix, err error)
}
type EthClient interface {
	Close()
	ABI(ctx context.Context, a abi.ABI) (ABIClient, error)
	ABIJSON(ctx context.Context, abiJson []byte) (ABIClient, error)
	MustABIJSON(abiJson []byte) ABIClient
	ChainID() int64

	// Below are raw functions that the ABI() above provides wrappers for
	CallContract(ctx context.Context, from *string, tx *ethsigner.Transaction, block string) (data ethtypes.HexBytes0xPrefix, err error)
	BuildRawTransaction(ctx context.Context, txVersion EthTXVersion, from string, tx *ethsigner.Transaction) (ethtypes.HexBytes0xPrefix, error)
	SendRawTransaction(ctx context.Context, rawTX ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error)
}

type EthClientFactory interface {
	Start() error              // connects the shared websocket and queries the chainID
	Stop()                     // closes HTTP client and shared WS client
	ChainID() int64            // available after start
	HTTPClient() EthClient     // HTTP client
	SharedWS() EthClient       // WS client with a single long lived socket shared across multiple components
	NewWS() (EthClient, error) // created a dedicated socket - which the caller responsible for closing
}

/* From "github.com/kaleido-io/paladin/kata/pkg/persistence" */
type Persistence interface {
	DB() *gorm.DB
	Close()
}

/* From "github.com/kaleido-io/paladin/kata/internal/rpcserver" */
type RPCServer interface {
	Register(module *RPCModule)
	Start() error
	Stop()
	EthPublish(eventType string, result interface{}) // Note this is an `eth_` specific extension, with no ack or reliability
	HTTPAddr() net.Addr
	WSAddr() net.Addr
}
type RPCModule struct {
	group   string
	methods map[string]RPCHandler
}

// RPCHandler should not be implemented directly - use RPCMethod0 ... RPCMethod5 to implement your function
// These use generics to avoid you needing to do any messy type mapping in your functions.
type RPCHandler func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse

/* From github.com/kaleido-io/paladin/toolkit/pkg/plugintk*/
type DomainAPI interface {
	ConfigureDomain(context.Context, *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error)
	InitDomain(context.Context, *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error)
	InitDeploy(context.Context, *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error)
	PrepareDeploy(context.Context, *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error)
	InitTransaction(context.Context, *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error)
	AssembleTransaction(context.Context, *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error)
	EndorseTransaction(context.Context, *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error)
	PrepareTransaction(context.Context, *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error)
}

type DomainCallbacks interface {
	FindAvailableStates(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error)
}

/* From "github.com/kaleido-io/paladin/kata/internal/plugins" */
type LibraryType string

const (
	LibraryTypeCShared LibraryType = "c-shared"
	LibraryTypeJar     LibraryType = "jar"
)

func (lt LibraryType) Enum() types.Enum[LibraryType] {
	return types.Enum[LibraryType](lt)
}

func (pl LibraryType) Options() []string {
	return []string{
		string(LibraryTypeCShared),
		string(LibraryTypeJar),
	}
}

type PluginConfig struct {
	Type     types.Enum[LibraryType]
	Location string
}

type DomainManagerToDomain interface {
	DomainAPI
	Initialized()
}

type DomainRegistration interface {
	ConfiguredDomains() map[string]*PluginConfig
	DomainRegistered(name string, id uuid.UUID, toDomain DomainManagerToDomain) (fromDomain DomainCallbacks, err error)
}
type PluginController interface {
	Start() error
	Stop()
	GRPCTargetURL() string
	LoaderID() uuid.UUID
	WaitForInit(ctx context.Context) error
	ReloadPluginList() error
}

/* From "github.com/kaleido-io/paladin/kata/internal/components" */
type PreInitComponents interface {
	KeyManager() KeyManager
	EthClientFactory() EthClientFactory
	Persistence() persistence.Persistence
	StateStore() statestore.StateStore
	BlockIndexer() blockindexer.BlockIndexer
	RPCServer() RPCServer
}
type PrivateContractDeploy struct {

	// INPUTS: Items that come in from the submitter of the transaction to send to the constructor
	ID     uuid.UUID // TODO: == idempotency key?
	Domain string
	Inputs types.RawJSON

	// ASSEMBLY PHASE
	TransactionSpecification *prototk.DeployTransactionSpecification
	RequiredVerifiers        []*prototk.ResolveVerifierRequest
	Verifiers                []*prototk.ResolvedVerifier

	// DISPATCH PHASE
	Signer            string
	InvokeTransaction *prototk.BaseLedgerTransaction
	DeployTransaction *prototk.BaseLedgerDeployTransaction
}

type Domain interface {
	Initialized() bool
	Name() string
	Address() *types.EthAddress
	GetSmartContractByAddress(ctx context.Context, addr types.EthAddress) (DomainSmartContract, error)
	Configuration() *prototk.DomainConfig

	InitDeploy(ctx context.Context, tx *PrivateContractDeploy) error
	PrepareDeploy(ctx context.Context, tx *PrivateContractDeploy) error
}
type TransactionInputs struct {
	Domain   string
	From     string
	To       *types.EthAddress
	Function *abi.Entry
	Inputs   types.RawJSON
}
type TransactionPreAssembly struct {
	TransactionSpecification *prototk.TransactionSpecification
	RequiredVerifiers        []*prototk.ResolveVerifierRequest
	Verifiers                []*prototk.ResolvedVerifier
}

type FullState struct {
	ID     types.Bytes32
	Schema types.Bytes32
	Data   types.RawJSON
}

type TransactionPostAssembly struct {
	AssemblyResult        prototk.AssembleTransactionResponse_Result
	OutputStatesPotential []*prototk.NewState // the raw result of assembly, before sequence allocation
	InputStates           []*FullState
	ReadStates            []*FullState
	OutputStates          []*FullState
	AttestationPlan       []*prototk.AttestationRequest
	Signatures            []*prototk.AttestationResult
	Endorsements          []*prototk.AttestationResult
}
type PrivateTransaction struct {
	ID uuid.UUID // TODO: == idempotency key?

	// INPUTS: Items that come in from the submitter of the transaction
	Inputs *TransactionInputs

	// ASSEMBLY PHASE: Items that get added to the transaction as it goes on its journey through
	// assembly, signing and endorsement (possibly going back through the journey many times)
	PreAssembly  *TransactionPreAssembly  // the bit of the assembly phase state that can be retained across re-assembly
	PostAssembly *TransactionPostAssembly // the bit of the assembly phase state that must be completely discarded on re-assembly

	// DISPATCH PHASE: Once the transaction has reached sufficient confidence of success,
	// we move on to submitting it to the blockchain.
	PreparedTransaction *prototk.BaseLedgerTransaction
}

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
type DomainManager interface {
	ManagerLifecycle
	DomainRegistration
	GetDomainByName(ctx context.Context, name string) (Domain, error)
}
type Managers interface {
	DomainManager() DomainManager
}
type PostInitComponents interface {
	PluginController() PluginController
}
type AllComponents interface {
	PreInitComponents
	PostInitComponents
	Managers
}

type ManagerEventStream struct {
	ABI     abi.ABI
	Handler blockindexer.InternalStreamCallback
}
type ManagerInitResult struct {
	EventStreams []*ManagerEventStream
	RPCModules   []*RPCModule
}
type ManagerLifecycle interface {
	Init(PreInitComponents) (*ManagerInitResult, error)
	Start() error
	Stop()
}
