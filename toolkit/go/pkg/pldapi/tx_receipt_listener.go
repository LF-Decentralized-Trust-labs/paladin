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

import "github.com/kaleido-io/paladin/toolkit/pkg/tktypes"

type TransactionReceiptListener struct {
	Name    string                            `docstruct:"TransactionReceiptStream" json:"name"`
	Created tktypes.Timestamp                 `docstruct:"TransactionReceiptStream" json:"created"`
	Started *bool                             `docstruct:"TransactionReceiptStream" json:"started"`
	Filters TransactionReceiptFilters         `docstruct:"TransactionReceiptStream" json:"filters"`
	Options TransactionReceiptListenerOptions `docstruct:"TransactionReceiptStream" json:"options"`
}

type TransactionReceiptFilters struct {
	SequenceAbove *uint64                        `docstruct:"TransactionReceiptFilters" json:"sequenceAbove,omitempty"`
	Type          *tktypes.Enum[TransactionType] `docstruct:"TransactionReceiptFilters" json:"type,omitempty"`
	Domain        string                         `docstruct:"TransactionReceiptFilters" json:"domain,omitempty"`
}

type IncompleteStateReceiptBehavior string

const (
	IncompleteStateReceiptBehaviorBlockContract IncompleteStateReceiptBehavior = "block_contract"
	IncompleteStateReceiptBehaviorProcess       IncompleteStateReceiptBehavior = "process"
)

func (tt IncompleteStateReceiptBehavior) Enum() tktypes.Enum[IncompleteStateReceiptBehavior] {
	return tktypes.Enum[IncompleteStateReceiptBehavior](tt)
}

func (tt IncompleteStateReceiptBehavior) Options() []string {
	return []string{
		string(IncompleteStateReceiptBehaviorBlockContract),
		string(IncompleteStateReceiptBehaviorProcess),
	}
}

func (tt IncompleteStateReceiptBehavior) Default() string {
	return string(IncompleteStateReceiptBehaviorBlockContract)
}

type TransactionReceiptListenerOptions struct {
	DomainReceipts                 bool                                         `docstruct:"TransactionReceiptFilters" json:"domainReceipts"`
	IncompleteStateReceiptBehavior tktypes.Enum[IncompleteStateReceiptBehavior] `docstruct:"TransactionReceiptFilters" json:"incompleteStateReceiptBehavior,omitempty"`
}
