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

package transportmgr

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func mockGetStateRetryThenOk(mc *mockComponents) {
	mc.stateManager.On("GetState", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false).
		Return(nil, fmt.Errorf("pop")).Once()
	mockGetStateOk(mc)
}

func mockGetStateOk(mc *mockComponents) {
	mGS := mc.stateManager.On("GetState", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false)
	mGS.Run(func(args mock.Arguments) {
		mGS.Return(&pldapi.State{
			StateBase: pldapi.StateBase{
				DomainName:      args[2].(string),
				ContractAddress: args[3].(tktypes.EthAddress),
				ID:              args[4].(tktypes.HexBytes),
				Data:            []byte(fmt.Sprintf(`{"dataFor": "%s"}`, args[4].(tktypes.HexBytes).HexString())),
			},
		}, nil)
	})
}

func TestReliableMessageResendRealDB(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		mockGetStateRetryThenOk,
	)
	defer done()

	tm.sendShortRetry = retry.NewRetryLimited(&pldconf.RetryConfigWithMax{
		MaxAttempts: confutil.P(1),
	})
	tm.reliableScanRetry = retry.NewRetryIndefinite(&pldconf.RetryConfig{
		MaxDelay: confutil.P("1ms"),
	})
	tm.quiesceTimeout = 10 * time.Millisecond
	tm.reliableMessageResend = 10 * time.Millisecond
	tm.peerInactivityTimeout = 1 * time.Second
	tm.reliableMessagePageSize = 1 // forking pagination

	mockActivateDeactivateOk(tp)

	sentMessages := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	sds := make([]*components.StateDistribution, 2)
	postCommits := make([]func(), 0)
	_ = tm.persistence.DB().Transaction(func(dbTX *gorm.DB) error {
		for i := 0; i < len(sds); i++ {
			sds[i] = &components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: tktypes.RandAddress().String(),
				SchemaID:        tktypes.RandHex(32),
				StateID:         tktypes.RandHex(32),
			}

			postCommit, err := tm.SendReliable(ctx, dbTX, &components.ReliableMessage{
				MessageType: components.RMTState.Enum(),
				Node:        "node2",
				Metadata:    tktypes.JSONString(sds[i]),
			})
			require.NoError(t, err)
			postCommits = append(postCommits, postCommit)
		}
		return nil
	})
	for _, pc := range postCommits {
		pc()
	}

	// Check we get the two messages twice, with the send retry kicking in
	for i := 0; i < 2; i++ {
		for iSD := 0; iSD < len(sds); iSD++ {
			msg := <-sentMessages
			var receivedSD components.StateDistributionWithData
			err := json.Unmarshal(msg.Payload, &receivedSD)
			require.NoError(t, err)
			require.Equal(t, sds[iSD], &receivedSD.StateDistribution)
			var receivedState pldapi.State
			err = json.Unmarshal(receivedSD.StateData, &receivedState)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"dataFor": "%s"}`, receivedSD.StateID), string(receivedSD.StateData))
		}
	}

	// From this point on we just drain
	go func() {
		for range sentMessages {
		}
	}()

	// Close the peer
	tm.peers["node2"].close()

	// Clean up the routine
	close(sentMessages)

}

func TestReliableMessageSendSendQuiesceRealDB(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		mockGetStateOk,
	)
	defer done()

	log.SetLevel("debug")

	tm.sendShortRetry = retry.NewRetryLimited(&pldconf.RetryConfigWithMax{
		MaxAttempts: confutil.P(1),
	})
	tm.quiesceTimeout = 10 * time.Millisecond
	tm.reliableMessageResend = 1 * time.Second
	tm.peerInactivityTimeout = 10 * time.Millisecond

	mockActivateDeactivateOk(tp)

	sentMessages := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	// Here we send two messages one at a time and check they arrive
	msgIDs := make([]uuid.UUID, 2)
	for i := 0; i < 2; i++ {
		sd := &components.StateDistribution{
			Domain:          "domain1",
			ContractAddress: tktypes.RandAddress().String(),
			SchemaID:        tktypes.RandHex(32),
			StateID:         tktypes.RandHex(32),
		}

		postCommit, err := tm.SendReliable(ctx, tm.persistence.DB(), &components.ReliableMessage{
			MessageType: components.RMTState.Enum(),
			Node:        "node2",
			Metadata:    tktypes.JSONString(sd),
		})
		require.NoError(t, err)
		postCommit()

		msg := <-sentMessages
		var receivedSD components.StateDistributionWithData
		err = json.Unmarshal(msg.Payload, &receivedSD)
		require.NoError(t, err)
		require.Equal(t, sd, &receivedSD.StateDistribution)
		var receivedState pldapi.State
		err = json.Unmarshal(receivedSD.StateData, &receivedState)
		require.NoError(t, err)
		require.JSONEq(t, fmt.Sprintf(`{"dataFor": "%s"}`, receivedSD.StateID), string(receivedSD.StateData))

		msgIDs[i] = uuid.MustParse(msg.MessageId)
	}

	// Deliver the two acks
	p := tm.peers["node2"]
	for _, msgID := range msgIDs {
		err := tm.writeAcks(ctx, tm.persistence.DB(), &components.ReliableMessageAck{
			MessageID: msgID,
		})
		require.NoError(t, err)
	}

	// Wait for the peer to end via quiesce
	<-p.senderDone

}

func TestSendBadReliableMessageMarkedFailRealDB(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		func(mc *mockComponents) {
			// missing state
			mc.stateManager.On("GetState", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false).
				Return(nil, nil).Once()
		},
	)
	defer done()

	tm.sendShortRetry = retry.NewRetryLimited(&pldconf.RetryConfigWithMax{
		MaxAttempts: confutil.P(1),
	})
	tm.quiesceTimeout = 10 * time.Millisecond
	tm.reliableMessageResend = 10 * time.Millisecond
	tm.peerInactivityTimeout = 1 * time.Second

	mockActivateDeactivateOk(tp)

	// First with missing metadata
	rm := &components.ReliableMessage{
		MessageType: components.RMTState.Enum(),
		Node:        "node2",
	}
	postCommit, err := tm.SendReliable(ctx, tm.persistence.DB(), rm)
	require.NoError(t, err)
	postCommit()

	// Second with missing state
	rm2 := &components.ReliableMessage{
		MessageType: components.RMTState.Enum(),
		Node:        "node2",
		Metadata: tktypes.JSONString(&components.StateDistribution{
			Domain:          "domain1",
			ContractAddress: tktypes.RandAddress().String(),
			SchemaID:        tktypes.RandHex(32),
			StateID:         tktypes.RandHex(32),
		}),
	}
	postCommit, err = tm.SendReliable(ctx, tm.persistence.DB(), rm2)
	require.NoError(t, err)
	postCommit()

	// Wait for nack
	var rmWithAck *components.ReliableMessage
	for (rmWithAck == nil || rmWithAck.Ack == nil) && !t.Failed() {
		time.Sleep(10 * time.Millisecond)
		rmWithAck, err = tm.getReliableMessageByID(ctx, tm.persistence.DB(), rm.ID)
		require.NoError(t, err)
	}
	require.NotNil(t, rmWithAck.Ack)
	require.Regexp(t, "PD012016", rmWithAck.Ack.Error)

	// Second nack
	rmWithAck, err = tm.getReliableMessageByID(ctx, tm.persistence.DB(), rm2.ID)
	require.NoError(t, err)
	require.NotNil(t, rmWithAck.Ack)
	require.Regexp(t, "PD012014", rmWithAck.Ack.Error)

}

func TestNameSortedPeers(t *testing.T) {

	peerList := nameSortedPeers{
		{PeerInfo: pldapi.PeerInfo{Name: "ccc"}},
		{PeerInfo: pldapi.PeerInfo{Name: "aaa"}},
		{PeerInfo: pldapi.PeerInfo{Name: "ddd"}},
		{PeerInfo: pldapi.PeerInfo{Name: "bbb"}},
	}

	sort.Sort(peerList)

	require.Equal(t, nameSortedPeers{
		{PeerInfo: pldapi.PeerInfo{Name: "aaa"}},
		{PeerInfo: pldapi.PeerInfo{Name: "bbb"}},
		{PeerInfo: pldapi.PeerInfo{Name: "ccc"}},
		{PeerInfo: pldapi.PeerInfo{Name: "ddd"}},
	}, peerList)

}

func TestConnectionRace(t *testing.T) {

	connWaiting := make(chan struct{})
	connRelease := make(chan struct{})

	ctx, tm, tp, done := newTestTransport(t, false,
		func(mc *mockComponents) {
			mGNT := mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return([]*components.RegistryNodeTransportEntry{
				{
					Node:      "node2",
					Transport: "test1",
					Details:   `{"likely":"json stuff"}`,
				},
			}, nil)
			mGNT.Run(func(args mock.Arguments) {
				close(connWaiting)
				<-connRelease
			})
		},
	)
	defer done()

	mockActivateDeactivateOk(tp)
	connDone := make(chan bool)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := tm.connectPeer(ctx, "node2", true)
			require.NoError(t, err)
			connDone <- true
		}()
	}
	<-connWaiting
	time.Sleep(10 * time.Millisecond)
	close(connRelease)
	<-connDone
	<-connDone

}

func TestActivateFail(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false, mockGoodTransport)
	defer done()

	tp.Functions.ActivateNode = func(ctx context.Context, anr *prototk.ActivateNodeRequest) (*prototk.ActivateNodeResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := tm.getPeer(ctx, "node2", true)
	assert.Regexp(t, "pop", err)

}

func TestActivateBadPeerInfo(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false, mockGoodTransport)
	defer done()

	tp.Functions.ActivateNode = func(ctx context.Context, anr *prototk.ActivateNodeRequest) (*prototk.ActivateNodeResponse, error) {
		return &prototk.ActivateNodeResponse{PeerInfoJson: "!{ not valid JSON"}, nil
	}

	p, err := tm.getPeer(ctx, "node2", true)
	assert.NoError(t, err)
	assert.Regexp(t, "!{ not valid JSON", p.Outbound["info"])

}

func TestQuiesceDetectPersistentMessage(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false, mockGoodTransport)
	defer done()

	// Load up a notification for a persistent message
	tm.reliableMessageResend = 10 * time.Millisecond
	tm.peerInactivityTimeout = 1 * time.Second
	tm.quiesceTimeout = 1 * time.Millisecond

	mockActivateDeactivateOk(tp)

	quiescingPeer, err := tm.getPeer(ctx, "node2", true)
	require.NoError(t, err)

	// Force cancel that peer
	quiescingPeer.cancelCtx()
	<-quiescingPeer.senderDone

	// Simulate quiescing with persistent messages delivered
	quiescingPeer.quiescing = true
	quiescingPeer.senderDone = make(chan struct{})
	quiescingPeer.persistedMsgsAvailable = make(chan struct{}, 1)
	quiescingPeer.persistedMsgsAvailable <- struct{}{}

	// Now in quiesce it will start up a new one
	quiescingPeer.senderCleanup()

	require.NotNil(t, tm.peers["node2"])

}

func TestQuiesceDetectFireAndForgetMessage(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
		mockEmptyReliableMsgs,
	)
	defer done()

	// Load up a notification for a persistent message
	tm.reliableMessageResend = 1 * time.Second
	tm.peerInactivityTimeout = 1 * time.Second
	tm.quiesceTimeout = 1 * time.Millisecond

	mockActivateDeactivateOk(tp)

	quiescingPeer, err := tm.getPeer(ctx, "node2", true)
	require.NoError(t, err)

	// Force cancel that peer
	quiescingPeer.cancelCtx()
	<-quiescingPeer.senderDone

	// Simulate quiescing with persistent messages delivered
	quiescingPeer.quiescing = true
	quiescingPeer.ctx = ctx
	quiescingPeer.senderDone = make(chan struct{})
	quiescingPeer.sendQueue = make(chan *prototk.PaladinMsg, 1)
	quiescingPeer.sendQueue <- &prototk.PaladinMsg{
		MessageId:   uuid.NewString(),
		Component:   prototk.PaladinMsg_TRANSACTION_ENGINE,
		MessageType: "example",
		Payload:     []byte(`{}`),
	}

	sentMessages := make(chan *prototk.PaladinMsg, 1)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		assert.NotEmpty(t, sent.MessageId)
		sentMessages <- sent
		return nil, nil
	}
	// Now in quiesce it will start up a new one
	quiescingPeer.senderCleanup()

	require.NotNil(t, tm.peers["node2"])

	<-sentMessages

}

func TestDeactivateFail(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false,
		mockGoodTransport,
		mockEmptyReliableMsgs,
	)
	defer done()

	tm.reliableMessageResend = 1 * time.Second
	tm.peerInactivityTimeout = 1 * time.Second
	tm.quiesceTimeout = 1 * time.Millisecond

	tp.Functions.ActivateNode = func(ctx context.Context, anr *prototk.ActivateNodeRequest) (*prototk.ActivateNodeResponse, error) {
		return &prototk.ActivateNodeResponse{PeerInfoJson: `{"endpoint":"some.url"}`}, nil
	}
	tp.Functions.DeactivateNode = func(ctx context.Context, dnr *prototk.DeactivateNodeRequest) (*prototk.DeactivateNodeResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := tm.getPeer(ctx, "node2", true)
	require.NoError(t, err)

}

func TestGetReliableMessageByIDFail(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents) {
		mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := tm.getReliableMessageByID(ctx, tm.persistence.DB(), uuid.New())
	require.Regexp(t, "pop", err)

}

func TestGetReliableMessageScanNoAction(t *testing.T) {

	_, tm, _, done := newTestTransport(t, false)
	defer done()

	tm.reliableMessageResend = 100 * time.Second

	p := &peer{
		tm:           tm,
		lastDrainHWM: confutil.P(uint64(100)),
		lastFullScan: time.Now(),
	}

	require.Nil(t, p.reliableMessageScan(false))

}

func TestProcessReliableMsgPageIgnoreBeforeHWM(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false)
	defer done()

	p := &peer{
		ctx:          ctx,
		tm:           tm,
		lastDrainHWM: confutil.P(uint64(100)),
	}

	err := p.processReliableMsgPage([]*components.ReliableMessage{
		{
			ID:       uuid.New(),
			Sequence: 50,
			Created:  tktypes.TimestampNow(),
		},
	})
	require.NoError(t, err)

}

func TestProcessReliableMsgPageIgnoreUnsupported(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents) {
		mc.db.Mock.ExpectExec("INSERT.*reliable_msg_acks").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	p := &peer{
		ctx: ctx,
		tm:  tm,
	}

	err := p.processReliableMsgPage([]*components.ReliableMessage{
		{
			ID:          uuid.New(),
			Sequence:    50,
			Created:     tktypes.TimestampNow(),
			MessageType: components.RMTReceipt.Enum(),
		},
	})
	require.Regexp(t, "pop", err)

}

func TestProcessReliableMsgPageInsertFail(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false,
		mockGetStateOk,
		func(mc *mockComponents) {
			mc.db.Mock.ExpectExec("INSERT.*reliable_msgs").WillReturnResult(driver.ResultNoRows)
		})
	defer done()

	p := &peer{
		ctx:       ctx,
		tm:        tm,
		transport: tp.t,
	}

	sd := &components.StateDistribution{
		Domain:          "domain1",
		ContractAddress: tktypes.RandAddress().String(),
		SchemaID:        tktypes.RandHex(32),
		StateID:         tktypes.RandHex(32),
	}

	rm := &components.ReliableMessage{
		ID:          uuid.New(),
		Sequence:    50,
		MessageType: components.RMTState.Enum(),
		Node:        "node2",
		Metadata:    tktypes.JSONString(sd),
		Created:     tktypes.TimestampNow(),
	}

	err := p.processReliableMsgPage([]*components.ReliableMessage{rm})
	require.Regexp(t, "PD020302", err)

}
