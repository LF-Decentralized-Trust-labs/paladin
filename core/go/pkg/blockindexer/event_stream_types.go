// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package blockindexer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"golang.org/x/crypto/sha3"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type EventStreamConfig struct {
	BatchSize    *int            `json:"batchSize,omitempty"`
	BatchTimeout *string         `json:"batchTimeout,omitempty"`
	FromBlock    json.RawMessage `json:"fromBlock,omitempty"`
}

var EventStreamDefaults = &EventStreamConfig{
	BatchSize:    confutil.P(50),
	BatchTimeout: confutil.P("75ms"),
	FromBlock:    json.RawMessage(`0`),
}

type EventStreamType string

const (
	EventStreamTypeInternal                   EventStreamType = "internal"                      // a core Paladin component, such as the state confirmation engine
	EventStreamTypePTXBlockchainEventListener EventStreamType = "ptx-blockchain-event-listener" // an event stream that backs an external PTX blockchain event listener
)

func (est EventStreamType) Options() []string {
	return []string{
		string(EventStreamTypeInternal),
		string(EventStreamTypePTXBlockchainEventListener),
	}
}
func (est EventStreamType) Enum() pldtypes.Enum[EventStreamType] {
	return pldtypes.Enum[EventStreamType](est)
}

type EventStream struct {
	ID      uuid.UUID                      `json:"id"             gorm:"primaryKey"`
	Name    string                         `json:"name"`
	Created pldtypes.Timestamp             `json:"created"        gorm:"autoCreateTime:nano"`
	Updated pldtypes.Timestamp             `json:"updated"        gorm:"autoUpdateTime:nano"`
	Type    pldtypes.Enum[EventStreamType] `json:"type"`
	Config  EventStreamConfig              `json:"config"         gorm:"type:bytes;serializer:json"`
	Sources EventSources                   `json:"sources"        gorm:"serializer:json"` // immutable (event delivery behavior would be too undefined with mutability)
	Format  pldtypes.JSONFormatOptions     `json:"format"`
	Started *bool                          `json:"started"`
}

type EventSources []EventStreamSource

// Build a hash that covers the unique set of combinations events + address.
// - Order independent
// - Ignores non-event parts of the ABI
func (ess EventSources) Hash(ctx context.Context) (*pldtypes.Bytes32, error) {
	// string hashes so we can sort them in a deterministic order
	sourceHashes := make([]string, len(ess))
	for i, s := range ess {
		hash, err := pldtypes.ABISolDefinitionHash(ctx, s.ABI, abi.Event /* only events matter */)
		if err != nil {
			return nil, err
		}
		// Need to factor the address into the hash
		if s.Address != nil {
			sourceHashes[i] = fmt.Sprintf("%s:%s", s.Address, hash)
		} else {
			sourceHashes[i] = fmt.Sprintf("*:%s", hash)
		}
	}
	sort.Strings(sourceHashes)
	hash := sha3.NewLegacyKeccak256()
	for _, h := range sourceHashes {
		hash.Write([]byte(h))
	}
	var h32 pldtypes.Bytes32
	_ = hash.Sum(h32[0:0])
	return &h32, nil
}

type EventStreamSource struct {
	ABI     abi.ABI              `json:"abi,omitempty"`
	Address *pldtypes.EthAddress `json:"address,omitempty"` // optional
}

type EventStreamCheckpoint struct {
	Stream      uuid.UUID `json:"id"                           gorm:"primaryKey"`
	BlockNumber int64     `json:"blockNumber"`
}

type EventStreamSignature struct {
	Stream        uuid.UUID        `json:"stream"                 gorm:"primaryKey"`
	SignatureHash pldtypes.Bytes32 `json:"signatureHash"          gorm:"primaryKey"`
}

type EventStreamStatus struct {
	CheckpointBlock int64
	Catchup         bool
}

type EventDeliveryBatch struct {
	StreamID   uuid.UUID               `json:"streamId"`
	StreamName string                  `json:"streamName"`
	BatchID    uuid.UUID               `json:"batchId"`
	Events     []*pldapi.EventWithData `json:"events"`
}

type PreCommitHandler func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*IndexedTransactionNotify) error

type InternalStreamCallbackDBTX func(ctx context.Context, dbTX persistence.DBTX, batch *EventDeliveryBatch) error

type InternalStreamCallbackNOTX func(ctx context.Context, batch *EventDeliveryBatch) error

type IESType int

const (
	// An event stream with its own checkpoint, and goroutine with its own DB transactions for checkpoint update, that can fall behind the head if necessary.
	// The event stream has a handler that is called with a DB transaction so must complete immediately.
	IESTypeEventStreamDBTX IESType = iota

	// An event stream with its own checkpoint, and goroutine with its own DB transactions for checkpoint update, that can fall behind the head if necessary.
	// The event stream has a handler that is called without a DB transaction so can block waiting for an external response.
	IESTypeEventStreamNOTX

	// An in-line callback that is fired with the raw block information, WITHIN the database transaction the block indexer uses to commit that information.
	// Slowdowns here slow down the whole block indexer, so this is for critical DB coordinated commit processing by other components only (receipt writing).
	// Errors from this function rollback the DB transaction, and hence stall the block indexer.
	// Can return a post-commit handler to be run after the DB transaction commits
	IESTypePreCommitHandler
)

type InternalEventStream struct {
	Type             IESType
	Definition       *EventStream
	HandlerDBTX      InternalStreamCallbackDBTX
	HandlerNOTX      InternalStreamCallbackNOTX
	PreCommitHandler PreCommitHandler
}
