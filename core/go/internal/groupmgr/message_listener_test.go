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
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testMessageReceiver struct {
	err       error
	callCount int
	called    chan struct{}
	pgMsgs    chan *pldapi.PrivacyGroupMessage
}

func (tmr *testMessageReceiver) DeliverMessageBatch(ctx context.Context, batchID uint64, pgMsgs []*pldapi.PrivacyGroupMessage) error {
	if tmr.callCount == 0 {
		close(tmr.called)
	}
	tmr.callCount++
	if tmr.err != nil {
		return tmr.err
	}
	for _, r := range pgMsgs {
		tmr.pgMsgs <- r
	}
	return nil
}

func newTestMessageReceiver(err error) *testMessageReceiver {
	return &testMessageReceiver{
		err:    err,
		called: make(chan struct{}),
		pgMsgs: make(chan *pldapi.PrivacyGroupMessage, 1),
	}
}

func createTestGroups(t *testing.T, ctx context.Context, mc *mockComponents, gm *groupManager, groups ...*pldapi.PrivacyGroupInput) []pldtypes.HexBytes {

	// Validate the init gets the correct data
	mc.domain.On("ConfigurePrivacyGroup", mock.Anything, mock.Anything).Return(map[string]string{"conf1": "value1"}, nil)
	ipg := mc.domain.On("InitPrivacyGroup", mock.Anything, mock.Anything, mock.Anything)
	ipg.Run(func(args mock.Arguments) {
		ipg.Return(
			&pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					Type: pldapi.TransactionTypePrivate.Enum(),
				},
			},
			nil,
		)
	})

	mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]uuid.UUID{uuid.New()}, nil)

	mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(rm []*pldapi.ReliableMessage) bool {
		return rm[0].MessageType.V() == pldapi.RMTPrivacyGroup
	})).Return(nil)

	ids := make([]pldtypes.HexBytes, len(groups))
	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		for i, g := range groups {
			g, err := gm.CreateGroup(ctx, dbTX, g)
			require.NoError(t, err)
			ids[i] = g.ID
		}
		return nil
	})
	require.NoError(t, err)
	return ids
}

func mockEmptyMessageListeners(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
	mc.db.Mock.ExpectQuery("SELECT.*message_listeners").WillReturnRows(mc.db.Mock.NewRows([]string{}))
}

func TestE2EMessageListenerDelivery(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{})
	defer done()

	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
		Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil)

	mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(rm []*pldapi.ReliableMessage) bool {
		return rm[0].MessageType.V() == pldapi.RMTPrivacyGroupMessage
	})).Return(nil)

	// Create the groups
	groupIDs := createTestGroups(t, ctx, mc, gm,
		&pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		},
		&pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		},
	)
	require.Len(t, groupIDs, 2)

	// Create listener (started) that includes local
	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name: "listener1",
		Filters: pldapi.PrivacyGroupMessageListenerFilters{
			Domain: "domain1",
			Group:  groupIDs[0],
			Topic:  "my\\/.*",
		},
	})
	require.NoError(t, err)
	// Create another listener (started) that doesn't include local
	err = gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name: "listener2",
		Options: pldapi.PrivacyGroupMessageListenerOptions{
			ExcludeLocal: true, // we won't get notified for all the messages we send locally
		},
	})
	require.NoError(t, err)

	// Write some message (before we attach to the listener to consume events)
	msgs := make([]*pldapi.PrivacyGroupMessage, 10)
	topics := []string{"my/topic/aaa", "my/topic/bbb", "another/topic/ccc", "my/topic/ddd"}
	err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		msgIDs := make([]uuid.UUID, len(msgs))
		for i := range msgIDs {
			msgID, err := gm.SendMessage(ctx, dbTX, &pldapi.PrivacyGroupMessageInput{
				Domain: "domain1",
				Group:  groupIDs[i%2],
				Topic:  topics[i%len(topics)],
				Data:   pldtypes.JSONString("some data"),
			})
			require.NoError(t, err)
			msgIDs[i] = *msgID
			msgs[i], err = gm.GetMessageByID(ctx, dbTX, *msgID, true)
			require.NoError(t, err)
		}
		return nil
	})
	require.NoError(t, err)
	require.Len(t, msgs, 10)

	// Query all the messages for one of the groups
	msgsGroup1, err := gm.QueryMessages(ctx, gm.p.NOTX(), query.NewQueryBuilder().Equal("group", groupIDs[0]).Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, msgsGroup1, 5)

	// Create receivers for the listeners
	receivedMsgsIncLocalGroup0 := newTestMessageReceiver(nil)
	closeReceiver1, err := gm.AddMessageReceiver(ctx, "listener1", receivedMsgsIncLocalGroup0)
	require.NoError(t, err)
	defer closeReceiver1.Close()

	// The messages should all be delivered to the receiver that specifies local
	for _, m := range msgs {
		if !m.Group.Equals(groupIDs[0]) || !strings.HasPrefix(m.Topic, "my") {
			continue
		}
		rm := <-receivedMsgsIncLocalGroup0.pgMsgs
		require.Equal(t, m.ID.String(), rm.ID.String())
	}

	// Second listener will not get the local ones
	receivedMsgsExcLocal := newTestMessageReceiver(nil)
	closeReceiver2, err := gm.AddMessageReceiver(ctx, "listener2", receivedMsgsExcLocal)
	require.NoError(t, err)
	defer closeReceiver2.Close()

	// Receive a remote message
	goodRemoteMsg := &pldapi.PrivacyGroupMessage{
		LocalSequence: 999999, /* will be overridden */
		Sent:          pldtypes.MustParseTimeString("2021-05-15T19:49:04.123Z"),
		Received:      pldtypes.MustParseTimeString("2021-05-15T19:49:04.123Z"), /* will be overridden */
		Node:          "ignored",                                                /* will be overridden */
		ID:            uuid.New(),
		PrivacyGroupMessageInput: pldapi.PrivacyGroupMessageInput{
			CorrelationID: confutil.P(uuid.New()),
			Domain:        "domain1",
			Group:         groupIDs[0],
			Topic:         "my/topic",
			Data:          pldtypes.JSONString("some data"),
		},
	}
	err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		badID := uuid.New()
		accepted, err := gm.ReceiveMessages(ctx, dbTX, []*pldapi.PrivacyGroupMessage{
			goodRemoteMsg,
			{ID: badID /* bad remote message */},
		})
		require.NoError(t, err)
		require.Len(t, accepted, 2)
		require.Regexp(t, "PD012514", accepted[badID])
		require.Nil(t, accepted[goodRemoteMsg.ID])
		return nil
	})
	require.NoError(t, err)

	// We should get the message on the first listener, which delivered a the local ones
	rm := <-receivedMsgsIncLocalGroup0.pgMsgs
	require.Equal(t, goodRemoteMsg.ID.String(), rm.ID.String())

	// We should also get it on the one that excludes local, as the first one to be delivered
	rm = <-receivedMsgsExcLocal.pgMsgs
	require.Equal(t, goodRemoteMsg.ID.String(), rm.ID.String())

}

func TestLoadListenersMultiPage(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{})
	defer done()

	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
		Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil).Maybe()

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{Name: "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	err = gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{Name: "listener2"})
	require.NoError(t, err)

	gm.messagesInit()
	gm.messageListenersLoadPageSize = 1

	err = gm.loadMessageListeners()
	require.NoError(t, err)
	gm.startMessageListeners()

	require.Len(t, gm.messageListeners, 2)

	err = gm.StartMessageListener(ctx, "listener1")
	require.NoError(t, err)

	err = gm.StopMessageListener(ctx, "listener1")
	require.NoError(t, err)

	err = gm.DeleteMessageListener(ctx, "listener1")
	require.NoError(t, err)

	ls, err := gm.QueryMessageListeners(ctx, gm.p.NOTX(), query.NewQueryBuilder().Limit(100).Query())
	require.NoError(t, err)
	require.Len(t, ls, 1)
	require.Equal(t, "listener2", ls[0].Name)

}

func TestLoadListenersFailBadListenerName(t *testing.T) {
	_, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock

	// 2nd load gives bad data
	mdb.ExpectQuery("SELECT.*message_listeners").WillReturnRows(mdb.NewRows([]string{
		"name", "filters", "options",
	}).AddRow(
		"" /* bad name */, "{}", "{}",
	))

	gm.messagesInit()

	err := gm.loadMessageListeners()
	require.Regexp(t, "PD020005", err)
}

func TestLoadListenersFailBadListenerTopicFilterRegexp(t *testing.T) {
	_, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock

	// 2nd load gives bad data
	mdb.ExpectQuery("SELECT.*message_listeners").WillReturnRows(mdb.NewRows([]string{
		"name", "filters", "options",
	}).AddRow(
		"listener1", `{"topic":"((((bad regexp"}`, "{}",
	))

	gm.messagesInit()

	err := gm.loadMessageListeners()
	require.Regexp(t, "PD012509", err)
}

func TestLoadListenersFail(t *testing.T) {
	_, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock

	mdb.ExpectQuery("SELECT.*message_listeners").WillReturnError(fmt.Errorf("pop"))

	gm.messagesInit()

	err := gm.loadMessageListeners()
	require.Regexp(t, "pop", err)
}

func TestCreateBadListener(t *testing.T) {
	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name: "badly-behaved",
		Filters: pldapi.PrivacyGroupMessageListenerFilters{
			Topic: "((((wrong",
		},
	})
	require.Regexp(t, "PD012509", err)
}

func TestCreateListenerFail(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectExec("INSERT.*message_listeners").WillReturnError(fmt.Errorf("pop"))

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name: "test1",
	})
	require.Regexp(t, "pop", err)
}

func TestAddMessageReceiverNotFound(t *testing.T) {
	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	_, err := gm.AddMessageReceiver(ctx, "test1", newTestMessageReceiver(nil))
	require.Regexp(t, "PD012508.*test1", err)
}

func TestStopMessageListenerNotFound(t *testing.T) {
	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	err := gm.StopMessageListener(ctx, "test1")
	require.Regexp(t, "PD012508.*test1", err)
}

func TestStartMessageListenerNotFound(t *testing.T) {
	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	err := gm.StartMessageListener(ctx, "test1")
	require.Regexp(t, "PD012508.*test1", err)
}

func TestStartMessageListenerFail(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectExec("UPDATE.*message_listeners").WillReturnError(fmt.Errorf("pop"))

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "test1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	err = gm.StartMessageListener(ctx, "test1")
	require.Regexp(t, "pop", err)
}

func TestDeleteMessageListenerNotFound(t *testing.T) {
	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	err := gm.DeleteMessageListener(ctx, "test1")
	require.Regexp(t, "PD012508.*test1", err)
}

func TestDeleteMessageListenerFail(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectExec("DELETE.*message_listeners").WillReturnError(fmt.Errorf("pop"))

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "test1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	_, err = gm.loadListener(ctx, &persistedMessageListener{Name: "test1", Filters: pldtypes.RawJSON(`{"topic":"(((bad listener"}`), Options: pldtypes.RawJSON(`{}`)})
	assert.Regexp(t, "PD012509", err)

	_, err = gm.loadListener(ctx, &persistedMessageListener{Name: "test1", Filters: pldtypes.RawJSON(`{}`), Options: pldtypes.RawJSON(`{}`)})
	assert.Regexp(t, "PD012512", err)

	err = gm.DeleteMessageListener(ctx, "test1")
	require.Regexp(t, "pop", err)
}

func TestCreateListenerBadOptions(t *testing.T) {
	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	_, err := gm.loadListener(ctx, &persistedMessageListener{
		Filters: pldtypes.RawJSON(`{ !badness`),
	})
	assert.Regexp(t, "PD012510", err)

	_, err = gm.loadListener(ctx, &persistedMessageListener{
		Filters: pldtypes.RawJSON(`{}`),
		Options: pldtypes.RawJSON(`{ !badness`),
	})
	assert.Regexp(t, "PD012511", err)

}

func TestAddReceiverNoBlock(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	r1, err := gm.AddMessageReceiver(ctx, "listener1", newTestMessageReceiver(nil))
	require.NoError(t, err)
	defer r1.Close()

	r2, err := gm.AddMessageReceiver(ctx, "listener1", newTestMessageReceiver(nil))
	require.NoError(t, err)
	defer r2.Close()
}

func TestNotifyNewMessagesNoBlock(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	gm.messageListeners["listener1"].notifyNewMessages()
	gm.messageListeners["listener1"].notifyNewMessages()
}

func TestClosedRetryingLoadingCheckpoint(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectQuery("SELECT.*message_listener_checkpoints").WillReturnError(fmt.Errorf("pop"))

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	gm.messagesRetry.UTSetMaxAttempts(1)
	l := gm.messageListeners["listener1"]
	l.initStart()
	l.runListener()
}

func mockMessages(count int, mc *mockComponents) {
	mc.db.Mock.MatchExpectationsInOrder(false)
	rows := sqlmock.
		NewRows([]string{
			"local_seq",
			"id",
			"data",
		})
	for i := 0; i < count; i++ {
		rows = rows.AddRow(
			int64(1000),
			uuid.New(),
			pldtypes.JSONString(fmt.Sprintf("message %d", i)),
		)
	}
	mc.db.Mock.ExpectQuery("SELECT.*pgroup_msgs").WillReturnRows(rows)
}

func TestClosedRetryingBatchDeliver(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectQuery("SELECT.*message_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
	mockMessages(1, mc)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	tmr := newTestMessageReceiver(fmt.Errorf("pop"))
	r, err := gm.AddMessageReceiver(ctx, "listener1", tmr)
	require.NoError(t, err)
	defer r.Close()

	gm.messagesRetry.UTSetMaxAttempts(1)
	l := gm.messageListeners["listener1"]
	l.initStart()
	l.runListener()
	<-tmr.called
}

func TestClosedRetryingWritingCheckpoint(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectQuery("SELECT.*message_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
	mdb.ExpectExec("INSERT.*message_listener_checkpoints").WillReturnError(fmt.Errorf("pop"))
	mockMessages(1, mc)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	tmr := newTestMessageReceiver(nil)
	r, err := gm.AddMessageReceiver(ctx, "listener1", tmr)
	require.NoError(t, err)
	defer r.Close()

	gm.messagesRetry.UTSetMaxAttempts(1)
	l := gm.messageListeners["listener1"]
	l.initStart()
	l.runListener()
	<-tmr.pgMsgs
}

func TestClosedRetryingQueryMessages(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectQuery("SELECT.*message_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
	mdb.ExpectQuery("SELECT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	gm.messagesRetry.UTSetMaxAttempts(1)
	l := gm.messageListeners["listener1"]
	l.initStart()
	l.runListener()
}

func TestDeliverBatchCancelledCtxNoReceiver(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := gm.messageListeners["listener1"]
	l.ctx, l.cancelCtx = context.WithCancel(ctx)
	l.cancelCtx()
	err = l.deliverBatch(&messageDeliveryBatch{})
	require.Regexp(t, "PD010301", err)
}

func TestDeliverBatchCancelledCtxNotifyReceiver(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	l := gm.messageListeners["listener1"]
	l.initStart()

	go func() {
		time.Sleep(10 * time.Millisecond)
		receipts := newTestMessageReceiver(nil)
		closeReceiver, err := gm.AddMessageReceiver(ctx, "listener1", receipts)
		require.NoError(t, err)
		t.Cleanup(func() { closeReceiver.Close() })
	}()

	r, err := l.nextReceiver(&messageDeliveryBatch{})
	require.NoError(t, err)
	require.NotNil(t, r)
	close(l.done)

}

func TestProcessPersistedMessagePostFilter(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
		Filters: pldapi.PrivacyGroupMessageListenerFilters{
			Domain: "domain1",
		},
	})
	require.NoError(t, err)

	l := gm.messageListeners["listener1"]
	l.initStart()

	l.processPersistedMessage(&messageDeliveryBatch{}, &persistedMessage{
		Domain: "domain2",
	})
	require.NoError(t, err)
	close(l.done)

}

func TestCreateMessageListenerDup(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.NoError(t, err)

	err = gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(false),
	})
	require.Regexp(t, "PD012507", err)

}

func TestLoadCheckpoint(t *testing.T) {
	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mdb := mc.db.Mock
	mdb.ExpectExec("INSERT.*message_listeners").WillReturnResult(driver.ResultNoRows)
	mdb.ExpectExec("UPDATE.*message_listeners").WillReturnResult(driver.ResultNoRows)

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name:    "listener1",
		Started: confutil.P(true),
	})
	require.NoError(t, err)

	l := gm.messageListeners["listener1"]
	require.NotNil(t, l)

	err = gm.StopMessageListener(ctx, "listener1")
	require.NoError(t, err)

	l.ctx = context.Background()

	l.spec.Filters.SequenceAbove = confutil.P(uint64(100))
	mdb.ExpectQuery("SELECT.*message_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{}))
	err = l.loadCheckpoint()
	require.NoError(t, err)
	require.Equal(t, uint64(100), *l.checkpoint)

	mdb.ExpectQuery("SELECT.*message_listener_checkpoints").WillReturnRows(sqlmock.NewRows([]string{
		"listener", "sequence", "time",
	}).AddRow(
		"listener1", int64(400), pldtypes.TimestampNow(),
	))
	err = l.loadCheckpoint()
	require.NoError(t, err)
	require.Equal(t, uint64(400), *l.checkpoint)

}
