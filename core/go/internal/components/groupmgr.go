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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
)

type PrivacyGroupGenesis struct {
	GenesisTransaction uuid.UUID                 `json:"genesisTransaction"`
	GenesisState       StateDistributionWithData `json:"genesisState"`
}

type PrivacyGroupMessageDistribution struct {
	Domain string            `json:"domain"`
	Group  pldtypes.HexBytes `json:"group"`
	ID     uuid.UUID         `json:"id"`
}

type PrivacyGroupDistribution struct {
	GenesisTransaction uuid.UUID                 `json:"genesisTransaction"`
	GenesisState       StateDistributionWithData `json:"genesisState"`
}

type PrivacyGroupMessageReceiver interface {
	DeliverMessageBatch(ctx context.Context, batchID uint64, msgs []*pldapi.PrivacyGroupMessage) error
}

type PrivacyGroupMessageReceiverCloser interface {
	Close()
}

type GroupManager interface {
	ManagerLifecycle

	CreateGroup(ctx context.Context, dbTX persistence.DBTX, spec *pldapi.PrivacyGroupInput) (group *pldapi.PrivacyGroup, err error)
	StoreReceivedGroup(context.Context, persistence.DBTX, string, uuid.UUID, *pldapi.State) (error, error)
	GetGroupByID(ctx context.Context, dbTX persistence.DBTX, domainName string, groupID pldtypes.HexBytes) (*pldapi.PrivacyGroup, error)
	QueryGroups(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PrivacyGroup, error)

	SendMessage(ctx context.Context, dbTX persistence.DBTX, msg *pldapi.PrivacyGroupMessageInput) (*uuid.UUID, error)
	ReceiveMessages(ctx context.Context, dbTX persistence.DBTX, msgs []*pldapi.PrivacyGroupMessage) (results map[uuid.UUID]error, err error)
	QueryMessages(ctx context.Context, dbTX persistence.DBTX, jq *query.QueryJSON) ([]*pldapi.PrivacyGroupMessage, error)
	GetMessageByID(ctx context.Context, dbTX persistence.DBTX, id uuid.UUID, failNotFound bool) (*pldapi.PrivacyGroupMessage, error)

	CreateMessageListener(ctx context.Context, spec *pldapi.PrivacyGroupMessageListener) error
	AddMessageReceiver(ctx context.Context, name string, r PrivacyGroupMessageReceiver) (PrivacyGroupMessageReceiverCloser, error)
	GetMessageListener(ctx context.Context, name string) *pldapi.PrivacyGroupMessageListener
}
