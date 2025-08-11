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

package groupmgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSendMessageNoTopic(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{})
	defer done()

	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
		Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil)

	groupIDs := createTestGroups(t, ctx, mc, gm,
		&pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		},
	)
	require.Len(t, groupIDs, 1)

	_, err := gm.SendMessage(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupMessageInput{
		Domain: "domain1",
		Data:   pldtypes.JSONString("some data"),
		Group:  groupIDs[0],
	})
	require.Regexp(t, "PD012515", err)
}

func TestSendMessageNoGroup(t *testing.T) {
	ctx, gm, _, done := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{})
	defer done()

	_, err := gm.SendMessage(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupMessageInput{
		Domain: "domain1",
		Data:   pldtypes.JSONString("some data"),
		Group:  pldtypes.RandBytes(32),
		Topic:  "topic1",
	})
	require.Regexp(t, "PD012502", err)
}

func TestSendMessageGroupFail(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnError(fmt.Errorf("pop"))

	_, err := gm.SendMessage(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupMessageInput{
		Domain: "domain1",
		Data:   pldtypes.JSONString("some data"),
		Group:  pldtypes.RandBytes(32),
		Topic:  "topic1",
	})
	require.Regexp(t, "pop", err)
}

func TestReceiveMessageInvalid(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{})
	defer done()

	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
		Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil)

	groupIDs := createTestGroups(t, ctx, mc, gm,
		&pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		},
	)
	require.Len(t, groupIDs, 1)

	badID := uuid.New()
	results, err := gm.ReceiveMessages(ctx, gm.p.NOTX(), []*pldapi.PrivacyGroupMessage{
		{
			ID: badID,
			PrivacyGroupMessageInput: pldapi.PrivacyGroupMessageInput{
				Domain: "domain1",
				Data:   pldtypes.JSONString("some data"),
				Topic:  "topic1",
				Group:  groupIDs[0],
			},
		},
	})
	require.NoError(t, err)
	require.Regexp(t, "PD012516", results[badID])
}

func TestSendMessageWriteFail(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	schemaID := pldtypes.RandBytes32()
	groupID := pldtypes.RandBytes(32)
	mockDBPrivacyGroup(mc, schemaID, groupID, nil)

	mc.db.Mock.ExpectExec("INSERT.*pgroup_msgs").WillReturnError(fmt.Errorf("pop"))

	_, err := gm.SendMessage(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupMessageInput{
		Domain: "domain1",
		Data:   pldtypes.JSONString("some data"),
		Group:  groupID,
		Topic:  "topic1",
	})
	require.Regexp(t, "pop", err)
}

func TestSendMessageBadMembers(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	schemaID := pldtypes.RandBytes32()
	groupID := pldtypes.RandBytes(32)
	mockDBPrivacyGroup(mc, schemaID, groupID, nil, "!!!! badness")

	mc.db.Mock.ExpectQuery("INSERT.*pgroup_msgs").WillReturnRows(sqlmock.NewRows([]string{}))

	_, err := gm.SendMessage(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupMessageInput{
		Domain: "domain1",
		Data:   pldtypes.JSONString("some data"),
		Group:  groupID,
		Topic:  "topic1",
	})
	require.Regexp(t, "PD020006", err)
}

func TestSendMessageSendMessageFail(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
		Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil)
	mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	mc.db.Mock.ExpectBegin()

	schemaID := pldtypes.RandBytes32()
	groupID := pldtypes.RandBytes(32)
	mockDBPrivacyGroup(mc, schemaID, groupID, nil, "me@node1", "me@node2")

	mc.db.Mock.ExpectQuery("INSERT.*pgroup_msgs").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.Mock.ExpectRollback()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.SendMessage(ctx, dbTX, &pldapi.PrivacyGroupMessageInput{
			Domain: "domain1",
			Data:   pldtypes.JSONString("some data"),
			Group:  groupID,
			Topic:  "topic1",
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestReceiveMessagesGroupNotFound(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnRows(sqlmock.NewRows([]string{}))
	mc.db.Mock.ExpectCommit()

	badID := uuid.New()
	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		results, err := gm.ReceiveMessages(ctx, dbTX, []*pldapi.PrivacyGroupMessage{
			{
				Sent:     pldtypes.TimestampNow(),
				Received: pldtypes.TimestampNow(),
				Node:     "node2",
				ID:       badID,
				PrivacyGroupMessageInput: pldapi.PrivacyGroupMessageInput{
					Domain: "domain1",
					Data:   pldtypes.JSONString("some data"),
					Group:  pldtypes.RandBytes(32),
					Topic:  "topic1",
				},
			},
		})
		require.Regexp(t, "PD012502", results[badID])
		return err
	})
	require.NoError(t, err)
}

func TestReceiveMessagesFailFindGroup(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnError(fmt.Errorf("pop"))
	mc.db.Mock.ExpectCommit()

	badID := uuid.New()
	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.ReceiveMessages(ctx, dbTX, []*pldapi.PrivacyGroupMessage{
			{
				Sent:     pldtypes.TimestampNow(),
				Received: pldtypes.TimestampNow(),
				Node:     "node2",
				ID:       badID,
				PrivacyGroupMessageInput: pldapi.PrivacyGroupMessageInput{
					Domain: "domain1",
					Data:   pldtypes.JSONString("some data"),
					Group:  pldtypes.RandBytes(32),
					Topic:  "topic1",
				},
			},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestReceiveMessagesFailInsert(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectBegin()
	schemaID := pldtypes.RandBytes32()
	groupID := pldtypes.RandBytes(32)
	mockDBPrivacyGroup(mc, schemaID, groupID, nil, "me@node1", "me@node2")
	mc.db.Mock.ExpectQuery("INSERT.*pgroup_msgs").WillReturnError(fmt.Errorf("pop"))
	mc.db.Mock.ExpectRollback()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		accepted, err := gm.ReceiveMessages(ctx, dbTX, []*pldapi.PrivacyGroupMessage{
			{
				Sent:     pldtypes.TimestampNow(),
				Received: pldtypes.TimestampNow(),
				Node:     "node2",
				ID:       uuid.New(),
				PrivacyGroupMessageInput: pldapi.PrivacyGroupMessageInput{
					Domain: "domain1",
					Data:   pldtypes.JSONString("some data"),
					Group:  pldtypes.RandBytes(32),
					Topic:  "topic1",
				},
			},
		})
		require.Empty(t, accepted)
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestGetMessageByIDFail(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*pgroup_msgs").WillReturnError(fmt.Errorf("pop"))

	_, err := gm.GetMessageByID(ctx, gm.p.NOTX(), uuid.New(), true)
	require.Regexp(t, "pop", err)

}

func TestGetMessageByIDEmptyFail(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*pgroup_msgs").WillReturnRows(sqlmock.NewRows([]string{}))

	_, err := gm.GetMessageByID(ctx, gm.p.NOTX(), uuid.New(), true)
	require.Regexp(t, "PD012513", err)

}

func TestGetMessageByIDEmptyNull(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*pgroup_msgs").WillReturnRows(sqlmock.NewRows([]string{}))

	msgByID, err := gm.GetMessageByID(ctx, gm.p.NOTX(), uuid.New(), false)
	require.NoError(t, err)
	require.Nil(t, msgByID)

}
