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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
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

/* From "github.com/kaleido-io/paladin/kata/internal/components" */
type PreInitComponents interface {
	KeyManager() KeyManager
	EthClientFactory() EthClientFactory
	Persistence() persistence.Persistence
	StateStore() statestore.StateStore
	BlockIndexer() blockindexer.BlockIndexer
	RPCServer() RPCServer
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
