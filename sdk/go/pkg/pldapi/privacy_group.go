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
	"sort"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type PrivacyGroup struct {
	ID                 pldtypes.HexBytes    `docstruct:"PrivacyGroup" json:"id"`
	Domain             string               `docstruct:"PrivacyGroup" json:"domain"`
	Created            pldtypes.Timestamp   `docstruct:"PrivacyGroup" json:"created"`
	Name               string               `docstruct:"PrivacyGroup" json:"name"`
	Members            []string             `docstruct:"PrivacyGroup" json:"members"`
	Properties         map[string]string    `docstruct:"PrivacyGroup" json:"properties"`
	Configuration      map[string]string    `docstruct:"PrivacyGroup" json:"configuration"`
	GenesisSalt        pldtypes.Bytes32     `docstruct:"PrivacyGroup" json:"genesisSalt"`
	GenesisSchema      pldtypes.Bytes32     `docstruct:"PrivacyGroup" json:"genesisSchema"`
	GenesisTransaction uuid.UUID            `docstruct:"PrivacyGroup" json:"genesisTransaction"`
	ContractAddress    *pldtypes.EthAddress `docstruct:"PrivacyGroup" json:"contractAddress"`
}

type PrivacyGroupTXOptions struct {
	IdempotencyKey string `docstruct:"PrivacyGroup" json:"idempotencyKey,omitempty"`
	PublicTxOptions
}

type PrivacyGroupMessage struct {
	ID            uuid.UUID          `docstruct:"PrivacyGroupMessage" json:"id"`
	LocalSequence uint64             `docstruct:"PrivacyGroupMessage" json:"localSequence"`
	Sent          pldtypes.Timestamp `docstruct:"PrivacyGroupMessage" json:"sent"`
	Received      pldtypes.Timestamp `docstruct:"PrivacyGroupMessage" json:"received"`
	Node          string             `docstruct:"PrivacyGroupMessage" json:"node"`
	PrivacyGroupMessageInput
}

type PrivacyGroupMessageInput struct {
	CorrelationID *uuid.UUID        `docstruct:"PrivacyGroupMessage" json:"correlationId,omitempty"`
	Domain        string            `docstruct:"PrivacyGroupMessage" json:"domain"`
	Group         pldtypes.HexBytes `docstruct:"PrivacyGroupMessage" json:"group"`
	Topic         string            `docstruct:"PrivacyGroupMessage" json:"topic,omitempty"`
	Data          pldtypes.RawJSON  `docstruct:"PrivacyGroupMessage" json:"data,omitempty"`
}

type PrivacyGroupInput struct {
	Domain             string                 `docstruct:"PrivacyGroup" json:"domain"`
	Members            []string               `docstruct:"PrivacyGroup" json:"members"`
	Name               string                 `docstruct:"PrivacyGroup" json:"name"`
	Properties         map[string]string      `docstruct:"PrivacyGroup" json:"properties,omitempty"`
	Configuration      map[string]string      `docstruct:"PrivacyGroup" json:"configuration,omitempty"`
	TransactionOptions *PrivacyGroupTXOptions `docstruct:"PrivacyGroupInput" json:"transactionOptions,omitempty"`
}

type PrivacyGroupEVMTX struct {
	From     string               `docstruct:"PrivacyGroupEVMTX" json:"from,omitempty"` // signing key reference
	To       *pldtypes.EthAddress `docstruct:"PrivacyGroupEVMTX" json:"to,omitempty"`
	Gas      *pldtypes.HexUint64  `docstruct:"PrivacyGroupEVMTX" json:"gas,omitempty"`
	Value    *pldtypes.HexUint256 `docstruct:"PrivacyGroupEVMTX" json:"value,omitempty"`
	Input    pldtypes.RawJSON     `docstruct:"PrivacyGroupEVMTX" json:"input,omitempty"`    // hex encoded bytes or object
	Function *abi.Entry           `docstruct:"PrivacyGroupEVMTX" json:"function,omitempty"` // required when JSON object/array input is supplied
	Bytecode pldtypes.HexBytes    `docstruct:"PrivacyGroupEVMTX" json:"bytecode,omitempty"` // bytes or object
}

// Transaction for a privacy group - is an ethereum style transaction input
type PrivacyGroupEVMTXInput struct {
	IdempotencyKey string            `docstruct:"PrivacyGroupEVMTX" json:"idempotencyKey,omitempty"`
	Domain         string            `docstruct:"PrivacyGroupEVMTX" json:"domain,omitempty"`
	Group          pldtypes.HexBytes `docstruct:"PrivacyGroupEVMTX" json:"group,omitempty"`
	PrivacyGroupEVMTX
	PublicTxOptions PublicTxOptions `docstruct:"PrivacyGroupEVMTX" json:"publicTxOptions,omitempty"`
}

// Call for a privacy group
type PrivacyGroupEVMCall struct {
	Domain string            `docstruct:"PrivacyGroupEVMTX" json:"domain,omitempty"`
	Group  pldtypes.HexBytes `docstruct:"PrivacyGroupEVMTX" json:"group,omitempty"`
	PrivacyGroupEVMTX
	PublicCallOptions
	DataFormat pldtypes.JSONFormatOptions `docstruct:"TransactionCall" json:"dataFormat"` // formatting options for the result data
}

type PrivacyGroupMessageListener struct {
	Name    string                             `docstruct:"PrivacyGroupMessageListener" json:"name"`
	Created pldtypes.Timestamp                 `docstruct:"PrivacyGroupMessageListener" json:"created"`
	Started *bool                              `docstruct:"PrivacyGroupMessageListener" json:"started"`
	Filters PrivacyGroupMessageListenerFilters `docstruct:"PrivacyGroupMessageListener" json:"filters"`
	Options PrivacyGroupMessageListenerOptions `docstruct:"PrivacyGroupMessageListener" json:"options"`
}

type PrivacyGroupMessageBatch struct {
	BatchID  uint64                 `docstruct:"PrivacyGroupMessageBatch" json:"batchId,omitempty"`
	Messages []*PrivacyGroupMessage `docstruct:"PrivacyGroupMessageBatch" json:"messages,omitempty"`
}

type PrivacyGroupMessageListenerFilters struct {
	SequenceAbove *uint64           `docstruct:"MessageListenerFilters" json:"sequenceAbove,omitempty"`
	Domain        string            `docstruct:"MessageListenerFilters" json:"domain,omitempty"`
	Group         pldtypes.HexBytes `docstruct:"MessageListenerFilters" json:"group,omitempty"`
	Topic         string            `docstruct:"MessageListenerFilters" json:"topic,omitempty"`
}

type PrivacyGroupMessageListenerOptions struct {
	ExcludeLocal bool `docstruct:"MessageListenerOptions" json:"excludeLocal,omitempty"`
}

type PGroupEventType string

const (
	PGroupEventTypeMessages PGroupEventType = "messages"
)

func (tt PGroupEventType) Enum() pldtypes.Enum[PGroupEventType] {
	return pldtypes.Enum[PGroupEventType](tt)
}

func (tt PGroupEventType) Options() []string {
	return []string{
		string(PGroupEventTypeMessages),
	}
}

func PrivacyGroupABISchema() *abi.Parameter {
	return &abi.Parameter{
		Name:         "PrivacyGroup",
		Type:         "tuple",
		InternalType: "struct PrivacyGroup",
		Components: abi.ParameterArray{
			{Name: "genesisSalt", Type: "bytes32"},
			{Name: "name", Type: "string", Indexed: true},
			{Name: "members", Type: "string[]"},
			{Name: "properties", Type: "tuple[]", InternalType: "struct Properties", Components: abi.ParameterArray{
				{Name: "key", Type: "string"},
				{Name: "value", Type: "string"},
			}},
			{Name: "configuration", Type: "tuple[]", InternalType: "struct Configuration", Components: abi.ParameterArray{
				{Name: "key", Type: "string"},
				{Name: "value", Type: "string"},
			}},
		},
	}
}

type KeyValueStringProperties []KeyValueStringProperty

func (p KeyValueStringProperties) Len() int           { return len(p) }
func (p KeyValueStringProperties) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p KeyValueStringProperties) Less(i, j int) bool { return p[i].Key < p[j].Key }

func (p KeyValueStringProperties) Map() map[string]string {
	m := make(map[string]string, len(p))
	for _, p := range p {
		m[p.Key] = p.Value
	}
	return m
}

func NewKeyValueStringProperties(m map[string]string) KeyValueStringProperties {
	p := make(KeyValueStringProperties, 0, len(m))
	for k, v := range m {
		p = append(p, KeyValueStringProperty{Key: k, Value: v})
	}
	sort.Sort(p)
	return p
}

type KeyValueStringProperty struct {
	Key   string `docstruct:"KeyValueProperty" json:"key"`
	Value string `docstruct:"KeyValueProperty" json:"value"`
}

type PrivacyGroupGenesisState struct {
	GenesisSalt   pldtypes.Bytes32         `docstruct:"PrivacyGroupGenesisState" json:"genesisSalt"`
	Name          string                   `docstruct:"PrivacyGroupGenesisState" json:"name"`
	Members       []string                 `docstruct:"PrivacyGroupGenesisState" json:"members"`
	Properties    KeyValueStringProperties `docstruct:"PrivacyGroupGenesisState" json:"properties"`
	Configuration KeyValueStringProperties `docstruct:"PrivacyGroupGenesisState" json:"configuration"`
}

func (pg *PrivacyGroup) GenesisStateData() *PrivacyGroupGenesisState {
	return &PrivacyGroupGenesisState{
		GenesisSalt:   pg.GenesisSalt,
		Name:          pg.Name,
		Members:       pg.Members,
		Properties:    NewKeyValueStringProperties(pg.Properties),
		Configuration: NewKeyValueStringProperties(pg.Configuration),
	}
}
