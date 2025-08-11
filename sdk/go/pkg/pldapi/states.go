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

package pldapi

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type SchemaType string

const (
	// ABI schema uses the same semantics as events for defining indexed fields (must be top-level)
	SchemaTypeABI SchemaType = "abi"
)

func (st SchemaType) Enum() pldtypes.Enum[SchemaType] {
	return pldtypes.Enum[SchemaType](st)
}

func (st SchemaType) Options() []string {
	return []string{
		string(SchemaTypeABI),
	}
}

// Queries against the state store can be made in the context of a
// transaction UUID, or one of the standard qualifiers
// (confirmed/unconfirmed/spent/all)
//
// Note this is not modelled as a normal Paladin Enum, as you can fall back to a UUID.
type StateStatusQualifier string

const StateStatusAvailable StateStatusQualifier = "available"
const StateStatusConfirmed StateStatusQualifier = "confirmed"
const StateStatusUnconfirmed StateStatusQualifier = "unconfirmed"
const StateStatusSpent StateStatusQualifier = "spent"
const StateStatusAll StateStatusQualifier = "all"

func (q *StateStatusQualifier) UnmarshalJSON(b []byte) error {
	var text string
	err := json.Unmarshal(b, &text)
	if err == nil {
		qText := StateStatusQualifier(strings.ToLower(text))
		switch qText {
		case StateStatusAvailable,
			StateStatusConfirmed,
			StateStatusUnconfirmed,
			StateStatusSpent,
			StateStatusAll:
			*q = qText
		default:
			u, err := uuid.Parse(string(text))
			if err != nil {
				return i18n.NewError(context.Background(), pldmsgs.MsgTypesInvalidStateQualifier)
			}
			*q = (StateStatusQualifier)(u.String())
		}
	}
	return err
}

type Schema struct {
	ID         pldtypes.Bytes32          `docstruct:"Schema" json:"id"          gorm:"primaryKey"`
	Created    pldtypes.Timestamp        `docstruct:"Schema" json:"created"     gorm:"autoCreateTime:false"` // we calculate the created time ourselves due to complex in-memory caching
	DomainName string                    `docstruct:"Schema" json:"domain"`
	Type       pldtypes.Enum[SchemaType] `docstruct:"Schema" json:"type"`
	Signature  string                    `docstruct:"Schema" json:"signature"`
	Definition pldtypes.RawJSON          `docstruct:"Schema" json:"definition"`
	Labels     []string                  `docstruct:"Schema" json:"labels"      gorm:"type:text[]; serializer:json"`
}

type StateBase struct {
	ID              pldtypes.HexBytes    `docstruct:"State" json:"id"                  gorm:"primaryKey"`
	Created         pldtypes.Timestamp   `docstruct:"State" json:"created"             gorm:"autoCreateTime:nano"`
	DomainName      string               `docstruct:"State" json:"domain"              gorm:"primaryKey"`
	Schema          pldtypes.Bytes32     `docstruct:"State" json:"schema"`
	ContractAddress *pldtypes.EthAddress `docstruct:"State" json:"contractAddress"` // nil used for states like privacy group genesis that exists before state creation
	Data            pldtypes.RawJSON     `docstruct:"State" json:"data"`
}

// Like StateBase, but encodes Data as HexBytes
type StateEncoded struct {
	ID              pldtypes.HexBytes    `json:"id"`
	DomainName      string               `json:"domain"`
	Schema          pldtypes.Bytes32     `json:"schema"`
	ContractAddress *pldtypes.EthAddress `json:"contractAddress"` // nil used for states like privacy group genesis that exists before state creation
	Data            pldtypes.HexBytes    `json:"data"`
}

type State struct {
	StateBase
	Labels      []*StateLabel       `docstruct:"State" json:"-"                   gorm:"foreignKey:state;references:id;"`
	Int64Labels []*StateInt64Label  `docstruct:"State" json:"-"                   gorm:"foreignKey:state;references:id;"`
	Confirmed   *StateConfirmRecord `docstruct:"State" json:"confirmed,omitempty" gorm:"foreignKey:state;references:id;"`
	Read        *StateReadRecord    `docstruct:"State" json:"read,omitempty"      gorm:"foreignKey:state;references:id;"`
	Spent       *StateSpendRecord   `docstruct:"State" json:"spent,omitempty"     gorm:"foreignKey:state;references:id;"`
	Locks       []*StateLock        `docstruct:"State" json:"locks,omitempty"     gorm:"-"` // in memory only processing here
	Nullifier   *StateNullifier     `docstruct:"State" json:"nullifier,omitempty" gorm:"foreignKey:state;references:id;"`
}

// TODO: Separate the GORM DTO from the external pldapi external type definition for States
func (StateBase) TableName() string {
	return "states"
}

type StateLabel struct {
	DomainName string            `gorm:"primaryKey"`
	State      pldtypes.HexBytes `gorm:"primaryKey"`
	Label      string
	Value      string
}

type StateInt64Label struct {
	DomainName string            `gorm:"primaryKey"`
	State      pldtypes.HexBytes `gorm:"primaryKey"`
	Label      string
	Value      int64
}

type TransactionStates struct {
	None        bool               `docstruct:"TransactionStates" json:"none,omitempty"` // true if we know nothing about this transaction at all
	Spent       []*StateBase       `docstruct:"TransactionStates" json:"spent,omitempty"`
	Read        []*StateBase       `docstruct:"TransactionStates" json:"read,omitempty"`
	Confirmed   []*StateBase       `docstruct:"TransactionStates" json:"confirmed,omitempty"`
	Info        []*StateBase       `docstruct:"TransactionStates" json:"info,omitempty"`
	Unavailable *UnavailableStates `docstruct:"TransactionStates" json:"unavailable,omitempty"` // nil if we have the data for all states
}

func (ts *TransactionStates) FirstUnavailable() pldtypes.HexBytes {
	switch {
	case ts.Unavailable == nil:
		return nil
	case len(ts.Unavailable.Confirmed) > 0:
		return ts.Unavailable.Confirmed[0]
	case len(ts.Unavailable.Spent) > 0:
		return ts.Unavailable.Spent[0]
	case len(ts.Unavailable.Read) > 0:
		return ts.Unavailable.Read[0]
	case len(ts.Unavailable.Info) > 0:
		return ts.Unavailable.Info[0]
	default:
		return nil
	}
}

type UnavailableStates struct {
	Confirmed []pldtypes.HexBytes `docstruct:"UnavailableStates" json:"confirmed"`
	Read      []pldtypes.HexBytes `docstruct:"UnavailableStates" json:"read"`
	Spent     []pldtypes.HexBytes `docstruct:"UnavailableStates" json:"spent"`
	Info      []pldtypes.HexBytes `docstruct:"UnavailableStates" json:"info"`
}

// A confirm record is written when indexing the blockchain, and can be written regardless
// of whether we currently have access to the private data of the state.
// It is simply a join record between the Paladin transaction ID and the state.
//
// A state is "available" if we:
// - have the confirm record
// - have the private data for the state
// - do not have a spend record
//
// Note that a Domain Context will track the creation of a state before it makes it to
// the blockchain, allowing us to submit chains of transactions that create and spend
// states all in a single block. In that case the state will only be "available" within
// the in-memory domain context being managed by the sequencer for that smart contract.
type StateConfirmRecord struct {
	DomainName  string            `json:"-"                 gorm:"primaryKey"`
	State       pldtypes.HexBytes `json:"-"                 gorm:"primaryKey"`
	Transaction uuid.UUID         `docstruct:"StateConfirm" json:"transaction"`
}

// A spend record is written when indexing the blockchain, and can be written regardless
// of whether we currently have access to the private data of the state.
// It is simply a join record between the Paladin transaction ID and the state.
//
// Once a spend record has been index, a state is no longer available for any transaction
// to consume (because we know the blockchain would reject if if we tried).
//
// Just like with the creation of new states, we keep an in-memory copy of the spend
// in the Domain Context of the sequencer while we are assembling+endorsing+submitting
// the transaction, to avoid us attempting to double-spend states (which of course will
// be rejected by the blockchain).
type StateSpendRecord struct {
	DomainName  string            `json:"-"                 gorm:"primaryKey"`
	State       pldtypes.HexBytes `json:"-"                 gorm:"primaryKey"`
	Transaction uuid.UUID         `docstruct:"StateSpend" json:"transaction"`
}

// We also record when we simply read a state during a transaction, without creating or
// spending it. This is important for being able to re-execute the transaction in the future
// against the exact state of the blockchain. We use this in receipt generation.
type StateReadRecord struct {
	DomainName  string            `json:"-"                 gorm:"primaryKey"`
	State       pldtypes.HexBytes `json:"-"                 gorm:"primaryKey"`
	Transaction uuid.UUID         `docstruct:"StateRead" json:"transaction"`
}

// Transactions can also refer to state that never exists before or after the transaction.
// It is part of the transaction that is required to fully process the transaction,
// but it originated exclusively within that transaction
type StateInfoRecord struct {
	DomainName  string            `json:"-"                 gorm:"primaryKey"`
	State       pldtypes.HexBytes `json:"-"                 gorm:"primaryKey"`
	Transaction uuid.UUID         `docstruct:"StateConfirm" json:"transaction"`
}

type StateLockType string

const (
	StateLockTypeCreate StateLockType = "create"
	StateLockTypeRead   StateLockType = "read"
	StateLockTypeSpend  StateLockType = "spend"
)

func (tt StateLockType) Enum() pldtypes.Enum[StateLockType] {
	return pldtypes.Enum[StateLockType](tt)
}

func (tt StateLockType) Options() []string {
	return []string{
		string(StateLockTypeCreate),
		string(StateLockTypeRead),
		string(StateLockTypeSpend),
	}
}

// State locks record which transaction a state is being locked to, either
// spending a previously confirmed state, or an optimistic record of creating
// (and maybe later spending) a state that is yet to be confirmed.
type StateLock struct {
	DomainName  string                       `json:"-"`
	StateID     pldtypes.HexBytes            `json:"-"`
	Transaction uuid.UUID                    `docstruct:"StateLock" json:"transaction"`
	Type        pldtypes.Enum[StateLockType] `docstruct:"StateLock" json:"type"`
}

// State nullifiers are used when a domain chooses to use a separate identifier
// specifically for spending states (i.e. not the state ID).
// Domains that choose to leverage this architecture will create nullifier
// entries for all unspent states, and create a StateSpend entry for the
// nullifier (not for the state) when it is spent.
// Immutable once written
type StateNullifier struct {
	DomainName string            `json:"-"               gorm:"primaryKey"`
	State      pldtypes.HexBytes `json:"-"`
	ID         pldtypes.HexBytes `json:"id"              gorm:"primaryKey"`
	Spent      *StateSpendRecord `json:"spent,omitempty" gorm:"foreignKey:state;references:id;"`
}
