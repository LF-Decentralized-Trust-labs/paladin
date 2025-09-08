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
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type ReliableMessageType string

const (
	RMTState               ReliableMessageType = "state"
	RMTReceipt             ReliableMessageType = "receipt"
	RMTPreparedTransaction ReliableMessageType = "prepared_txn"
	RMTPrivacyGroup        ReliableMessageType = "privacy_group"
	RMTPrivacyGroupMessage ReliableMessageType = "privacy_group_message"
)

func (t ReliableMessageType) Enum() pldtypes.Enum[ReliableMessageType] {
	return pldtypes.Enum[ReliableMessageType](t)
}

func (t ReliableMessageType) Options() []string {
	return []string{
		string(RMTState),
		string(RMTReceipt),
		string(RMTPreparedTransaction),
		string(RMTPrivacyGroup),
		string(RMTPrivacyGroupMessage),
	}
}

type ReliableMessage struct {
	Sequence    uint64                             `docstruct:"ReliableMessage" json:"sequence"        gorm:"column:sequence;primaryKey"`
	ID          uuid.UUID                          `docstruct:"ReliableMessage" json:"id"              gorm:"column:id"`
	Created     pldtypes.Timestamp                 `docstruct:"ReliableMessage" json:"created"         gorm:"column:created;autoCreateTime:false"` // generated in our code
	Node        string                             `docstruct:"ReliableMessage" json:"node"            gorm:"column:node"`                         // The node id to send the message to
	MessageType pldtypes.Enum[ReliableMessageType] `docstruct:"ReliableMessage" json:"messageType"     gorm:"column:msg_type"`
	Metadata    pldtypes.RawJSON                   `docstruct:"ReliableMessage" json:"metadata"        gorm:"column:metadata"`
	Ack         *ReliableMessageAckNoMsgID         `docstruct:"ReliableMessage" json:"ack,omitempty"   gorm:"foreignKey:MessageID;references:ID;"`
}

type ReliableMessageAckNoMsgID struct {
	MessageID uuid.UUID          `docstruct:"ReliableMessageAck" json:"-"                                gorm:"column:id;primaryKey"`
	Time      pldtypes.Timestamp `docstruct:"ReliableMessageAck" json:"time,omitempty"                   gorm:"column:time;autoCreateTime:false"` // generated in our code
	Error     string             `docstruct:"ReliableMessageAck" json:"error,omitempty"                  gorm:"column:error"`
}

func (rma ReliableMessageAckNoMsgID) TableName() string {
	return "reliable_msg_acks"
}

func (rm ReliableMessage) TableName() string {
	return "reliable_msgs"
}

type ReliableMessageAck struct {
	MessageID uuid.UUID          `docstruct:"ReliableMessageAck" json:"messageId,omitempty"              gorm:"column:id;primaryKey"`
	Time      pldtypes.Timestamp `docstruct:"ReliableMessageAck" json:"time,omitempty"                   gorm:"column:time;autoCreateTime:false"` // generated in our code
	Error     string             `docstruct:"ReliableMessageAck" json:"error,omitempty"                  gorm:"column:error"`
}

func (rma ReliableMessageAck) TableName() string {
	return "reliable_msg_acks"
}
