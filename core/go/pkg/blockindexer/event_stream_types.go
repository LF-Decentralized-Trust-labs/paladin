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
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"golang.org/x/crypto/sha3"

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

type EventStreamConfig struct {
	BatchSize    *int    `json:"batchSize,omitempty"`
	BatchTimeout *string `json:"batchTimeout,omitempty"`
}

var EventStreamDefaults = &EventStreamConfig{
	BatchSize:    confutil.P(50),
	BatchTimeout: confutil.P("75ms"),
}

type EventStreamType string

const (
	EventStreamTypeInternal EventStreamType = "internal" // a core Paladin component, such as the state confirmation engine
)

func (est EventStreamType) Options() []string {
	return []string{
		string(EventStreamTypeInternal),
	}
}
func (est EventStreamType) Enum() tktypes.Enum[EventStreamType] {
	return tktypes.Enum[EventStreamType](est)
}

type EventStream struct {
	ID      uuid.UUID                     `json:"id"             gorm:"primaryKey"`
	Name    string                        `json:"name"`
	Created tktypes.Timestamp             `json:"created"        gorm:"autoCreateTime:nano"`
	Updated tktypes.Timestamp             `json:"updated"        gorm:"autoUpdateTime:nano"`
	Type    tktypes.Enum[EventStreamType] `json:"type"`
	Config  EventStreamConfig             `json:"config"         gorm:"type:bytes;serializer:json"`
	Sources EventSources                  `json:"sources"        gorm:"serializer:json"` // immutable (event delivery behavior would be too undefined with mutability)
}

type EventSources []EventStreamSource

// Build a hash that covers the unique set of combinations events + address.
// - Order independent
// - Ignores non-event parts of the ABI
func (ess EventSources) Hash(ctx context.Context) (*tktypes.Bytes32, error) {
	// string hashes so we can sort them in a deterministic order
	sourceHashes := make([]string, len(ess))
	for i, s := range ess {
		hash, err := tktypes.ABISolDefinitionHash(ctx, s.ABI, abi.Event /* only events matter */)
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
	var h32 tktypes.Bytes32
	_ = hash.Sum(h32[0:0])
	return &h32, nil
}

type EventStreamSource struct {
	ABI     abi.ABI             `json:"abi,omitempty"`
	Address *tktypes.EthAddress `json:"address,omitempty"` // optional
}

type EventStreamCheckpoint struct {
	Stream      uuid.UUID `json:"id"                           gorm:"primaryKey"`
	BlockNumber int64     `json:"blockNumber"`
}

type EventStreamSignature struct {
	Stream        uuid.UUID       `json:"stream"                 gorm:"primaryKey"`
	SignatureHash tktypes.Bytes32 `json:"signatureHash"          gorm:"primaryKey"`
}

type EventWithData struct {
	*IndexedEvent

	// SoliditySignature allows a deterministic comparison to which ABI to use in the runtime,
	// when both the blockindexer and consuming code are using the same version of firefly-signer.
	// Includes variable names, including deep within nested structure.
	// Things like whitespace etc. subject to change (so should not stored for later comparison)
	SoliditySignature string `json:"soliditySignature"`

	Address tktypes.EthAddress `json:"address"`
	Data    tktypes.RawJSON    `json:"data"`
}

type EventDeliveryBatch struct {
	StreamID   uuid.UUID        `json:"streamId"`
	StreamName string           `json:"streamName"`
	BatchID    uuid.UUID        `json:"batchId"`
	Events     []*EventWithData `json:"events"`
}

// Post commit callback is invoked after the DB transaction completes (only on success)
type PostCommit func()

type PreCommitHandler func(ctx context.Context, dbTX *gorm.DB, blocks []*IndexedBlock, transactions []*IndexedTransactionNotify) (PostCommit, error)

type InternalStreamCallback func(ctx context.Context, dbTX *gorm.DB, batch *EventDeliveryBatch) (PostCommit, error)

type IESType int

const (
	// An event stream with its own checkpoint, and goroutine with its own DB transactions for checkpoint update, that can fall behind the head if necessary
	IESTypeEventStream IESType = iota
	// An in-line callback that is fired with the raw block information, WITHIN the database transaction the block indexer uses to commit that information.
	// Slowdowns here slow down the whole block indexer, so this is for critical DB coordinated commit processing by other components only (receipt writing).
	// Errors from this function rollback the DB transaction, and hence stall the block indexer.
	// Can return a post-commit handler to be run after the DB transaction commits
	IESTypePreCommitHandler
)

type InternalEventStream struct {
	Type             IESType
	Definition       *EventStream
	Handler          InternalStreamCallback
	PreCommitHandler PreCommitHandler
}
