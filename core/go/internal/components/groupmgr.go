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
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type PrivacyGroupGenesisWithABI struct {
	GenesisState StateDistributionWithData `json:"genesisState"`
	GenesisABI   abi.Parameter             `json:"genesisABI"`
}

type PreparedGroupInitTransaction struct {
	TX            *pldapi.TransactionInput
	GenesisState  tktypes.RawJSON
	GenesisSchema *abi.Parameter
}

type PrivacyGroupMessageDistribution struct {
	Domain string           `json:"domain"`
	Group  tktypes.HexBytes `json:"group"`
	ID     uuid.UUID        `json:"id"`
}

type PrivacyGroupMessageReceiver interface {
	DeliverMessageBatch(ctx context.Context, batchID uint64, msgs []*pldapi.PrivacyGroupMessage) error
}

type PrivacyGroupMessageReceiverCloser interface {
	Close()
}

type GroupManager interface {
	ManagerLifecycle

	CreateGroup(ctx context.Context, dbTX persistence.DBTX, spec *pldapi.PrivacyGroupInput) (id tktypes.HexBytes, err error)
	GetGroupByID(ctx context.Context, dbTX persistence.DBTX, domainName string, groupID tktypes.HexBytes) (*pldapi.PrivacyGroupWithABI, error)
	QueryGroups(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PrivacyGroup, error)
	QueryGroupsByProperties(ctx context.Context, dbTX persistence.DBTX, domainName string, schemaID tktypes.Bytes32, jq *query.QueryJSON) ([]*pldapi.PrivacyGroup, error)

	SendMessage(ctx context.Context, dbTX persistence.DBTX, msg *pldapi.PrivacyGroupMessageInput) (*uuid.UUID, error)
	ReceiveMessages(ctx context.Context, dbTX persistence.DBTX, msgs []*pldapi.PrivacyGroupMessage) (results map[uuid.UUID]error, err error)
	QueryMessages(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PrivacyGroupMessage, error)
	GetMessageByID(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID, failNotFound bool) (*pldapi.PrivacyGroupMessage, error)

	CreateMessageListener(ctx context.Context, spec *pldapi.PrivacyGroupMessageListener) error
	AddMessageReceiver(ctx context.Context, name string, r PrivacyGroupMessageReceiver) (PrivacyGroupMessageReceiverCloser, error)
	GetMessageListener(ctx context.Context, name string) *pldapi.PrivacyGroupMessageListener
}
