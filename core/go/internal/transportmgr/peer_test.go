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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func mockGetStateRetryThenOk(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
	mc.stateManager.On("GetStatesByID", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false).
		Return(nil, fmt.Errorf("pop")).Once()
	mockGetStateOk(mc, conf)
}

func mockGetStateOk(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
	mGS := mc.stateManager.On("GetStatesByID", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false)
	mGS.Run(func(args mock.Arguments) {
		id := (args[4].([]pldtypes.HexBytes))[0]
		mGS.Return([]*pldapi.State{
			{
				StateBase: pldapi.StateBase{
					DomainName:      args[2].(string),
					ContractAddress: args[3].(*pldtypes.EthAddress),
					ID:              id,
					Data:            []byte(fmt.Sprintf(`{"dataFor": "%s"}`, id.HexString())),
				},
			},
		}, nil)
	})
}

func TestReliableMessageResendRealDB(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		mockGetStateRetryThenOk,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node3").Return([]*components.RegistryNodeTransportEntry{
				{
					Node:      "node3",
					Transport: "test1",
				},
			}, nil)
		},
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

	sentMessagesNode2 := make(chan *prototk.PaladinMsg)
	sentMessagesNode3 := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		if req.Node == "node2" {
			sentMessagesNode2 <- sent
		} else {
			sentMessagesNode3 <- sent
		}
		return nil, nil
	}

	sds := make([]*components.StateDistribution, 4)
	for i := range sds {
		sds[i] = &components.StateDistribution{
			Domain:          "domain1",
			ContractAddress: pldtypes.RandAddress().String(),
			SchemaID:        pldtypes.RandHex(32),
			StateID:         pldtypes.RandHex(32),
		}
	}

	_ = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for i := range sds {
			var node string
			if i%2 == 0 {
				node = "node2"
			} else {
				node = "node3"
			}
			err := tm.SendReliable(ctx, dbTX, &pldapi.ReliableMessage{
				MessageType: pldapi.RMTState.Enum(),
				Node:        node,
				Metadata:    pldtypes.JSONString(sds[i]),
			})
			require.NoError(t, err)
		}
		return nil
	})

	// Check each peer dispatches two messages twice (with the send retry kicking in)
	for range 2 {
		for iSD := range sds {
			var msg *prototk.PaladinMsg
			if iSD%2 == 0 {
				msg = <-sentMessagesNode2
			} else {
				msg = <-sentMessagesNode3
			}
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
		for range sentMessagesNode2 {
		}
		for range sentMessagesNode3 {
		}
	}()

	// Close the peer
	tm.peers["node2"].close()
	tm.peers["node3"].close()

	// Clean up the routine
	close(sentMessagesNode2)
	close(sentMessagesNode3)

}

func TestReliableMessageSendSendQuiesceRealDB(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, true,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			conf.PeerReaperInterval = confutil.P("50ms")
		},
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
			ContractAddress: pldtypes.RandAddress().String(),
			SchemaID:        pldtypes.RandHex(32),
			StateID:         pldtypes.RandHex(32),
		}

		err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			return tm.SendReliable(ctx, dbTX, &pldapi.ReliableMessage{
				MessageType: pldapi.RMTState.Enum(),
				Node:        "node2",
				Metadata:    pldtypes.JSONString(sd),
			})
		})
		require.NoError(t, err)

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
		rmr, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
			FromNode: "node2",
			Message:  buildAck(msgID, ""),
		})
		require.NoError(t, err)
		assert.NotNil(t, rmr)
	}

	// Wait for the peer to end via quiesce
	<-p.senderDone

}

func TestSendBadReliableMessageMarkedFailRealDB(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			// missing state
			mc.stateManager.On("GetStatesByID", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, false).
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
	rm := &pldapi.ReliableMessage{
		MessageType: pldapi.RMTState.Enum(),
		Node:        "node2",
	}
	err := tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return tm.SendReliable(ctx, dbTX, rm)
	})
	require.NoError(t, err)

	// Second with missing state
	rm2 := &pldapi.ReliableMessage{
		MessageType: pldapi.RMTState.Enum(),
		Node:        "node2",
		Metadata: pldtypes.JSONString(&components.StateDistribution{
			Domain:          "domain1",
			ContractAddress: pldtypes.RandAddress().String(),
			SchemaID:        pldtypes.RandHex(32),
			StateID:         pldtypes.RandHex(32),
		}),
	}
	err = tm.persistence.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return tm.SendReliable(ctx, dbTX, rm2)
	})
	require.NoError(t, err)

	// Wait for nack
	var rmWithAck *pldapi.ReliableMessage
	for (rmWithAck == nil || rmWithAck.Ack == nil) && !t.Failed() {
		time.Sleep(10 * time.Millisecond)
		rmWithAck, err = tm.getReliableMessageByID(ctx, tm.persistence.NOTX(), rm.ID)
		require.NoError(t, err)
	}
	require.NotNil(t, rmWithAck.Ack)
	require.Regexp(t, "PD012016", rmWithAck.Ack.Error)

	// Second nack
	rmWithAck, err = tm.getReliableMessageByID(ctx, tm.persistence.NOTX(), rm2.ID)
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
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
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

	tp.Functions.ActivatePeer = func(ctx context.Context, anr *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := tm.getPeer(ctx, "node2", true)
	assert.Regexp(t, "pop", err)

}

func TestActivateBadPeerInfo(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false, mockGoodTransport)
	defer done()

	tp.Functions.ActivatePeer = func(ctx context.Context, anr *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
		return &prototk.ActivatePeerResponse{PeerInfoJson: "!{ not valid JSON"}, nil
	}

	p, err := tm.getPeer(ctx, "node2", true)
	assert.NoError(t, err)
	assert.Regexp(t, "!{ not valid JSON", p.Outbound["info"])

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

	tp.Functions.ActivatePeer = func(ctx context.Context, anr *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
		return &prototk.ActivatePeerResponse{PeerInfoJson: `{"endpoint":"some.url"}`}, nil
	}
	tp.Functions.DeactivatePeer = func(ctx context.Context, dnr *prototk.DeactivatePeerRequest) (*prototk.DeactivatePeerResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := tm.getPeer(ctx, "node2", true)
	require.NoError(t, err)

}

func TestGetReliableMessageByIDFail(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.db.Mock.ExpectQuery("SELECT.*reliable_msgs").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	_, err := tm.getReliableMessageByID(ctx, tm.persistence.NOTX(), uuid.New())
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

	err := p.processReliableMsgPage(tm.persistence.NOTX(), []*pldapi.ReliableMessage{
		{
			ID:       uuid.New(),
			Sequence: 50,
			Created:  pldtypes.TimestampNow(),
		},
	})
	require.NoError(t, err)

}

func TestProcessReliableMsgPageIgnoreUnsupported(t *testing.T) {

	ctx, tm, _, done := newTestTransport(t, false, func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
		mc.db.Mock.ExpectExec("INSERT.*reliable_msg_acks").WillReturnError(fmt.Errorf("pop"))
	})
	defer done()

	p := &peer{
		ctx: ctx,
		tm:  tm,
	}

	err := p.processReliableMsgPage(tm.persistence.NOTX(), []*pldapi.ReliableMessage{
		{
			ID:          uuid.New(),
			Sequence:    50,
			Created:     pldtypes.TimestampNow(),
			MessageType: pldtypes.Enum[pldapi.ReliableMessageType]("wrong"),
		},
	})
	require.Regexp(t, "pop", err)

}

func TestProcessReliableMsgPageInsertFail(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false,
		mockGetStateOk,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
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
		ContractAddress: pldtypes.RandAddress().String(),
		SchemaID:        pldtypes.RandHex(32),
		StateID:         pldtypes.RandHex(32),
	}

	rm := &pldapi.ReliableMessage{
		ID:          uuid.New(),
		Sequence:    50,
		MessageType: pldapi.RMTState.Enum(),
		Node:        "node2",
		Metadata:    pldtypes.JSONString(sd),
		Created:     pldtypes.TimestampNow(),
	}

	err := p.processReliableMsgPage(tm.persistence.NOTX(), []*pldapi.ReliableMessage{rm})
	require.Regexp(t, "PD020302", err)

}

func TestProcessReliableMsgPagePrivacyGroup(t *testing.T) {

	schemaID := pldtypes.RandBytes32()
	ctx, tm, tp, done := newTestTransport(t, false,
		mockGetStateOk,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.db.Mock.ExpectExec("INSERT.*reliable_msgs").WillReturnResult(driver.ResultNoRows)
		})
	defer done()

	p := &peer{
		ctx:       ctx,
		tm:        tm,
		transport: tp.t,
	}

	pgd := &components.PrivacyGroupDistribution{
		GenesisTransaction: uuid.New(),
		GenesisState: components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: pldtypes.RandAddress().String(),
				SchemaID:        schemaID.String(),
				StateID:         pldtypes.RandHex(32),
			},
		},
	}

	rm := &pldapi.ReliableMessage{
		ID:          uuid.New(),
		Sequence:    50,
		MessageType: pldapi.RMTPrivacyGroup.Enum(),
		Node:        "node2",
		Metadata:    pldtypes.JSONString(pgd),
		Created:     pldtypes.TimestampNow(),
	}

	sentMessages := make(chan *prototk.PaladinMsg, 1)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	err := p.processReliableMsgPage(tm.persistence.NOTX(), []*pldapi.ReliableMessage{rm})
	require.NoError(t, err)

	sentMsg := <-sentMessages

	rMsg, err := parseReceivedMessage(ctx, "node2", sentMsg)
	require.NoError(t, err)
	require.Equal(t, RMHMessageTypePrivacyGroup, rMsg.MessageType)

	rpg, err := parsePrivacyGroupDistribution(ctx, rMsg.MessageID, rMsg.Payload, "node2")
	require.NoError(t, err)
	require.Equal(t, "domain1", rpg.domain)
	require.JSONEq(t, fmt.Sprintf(`{"dataFor": "%s"}`, rpg.genesisState.ID.HexString()), rpg.genesisState.Data.Pretty())
	require.Equal(t, pgd.GenesisTransaction, rpg.genesisTx)
	require.Equal(t, "node2", rpg.node)
}

func TestProcessReliableMsgPagePrivacyGroupMessage(t *testing.T) {

	origMsg := &pldapi.PrivacyGroupMessage{
		ID:   uuid.New(),
		Sent: pldtypes.TimestampNow(),
		PrivacyGroupMessageInput: pldapi.PrivacyGroupMessageInput{
			Domain: "domain1",
			Group:  pldtypes.RandBytes(32),
			Topic:  "topic1",
			Data:   pldtypes.JSONString("some data"),
		},
	}
	ctx, tm, tp, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.groupManager.On("GetMessageByID", mock.Anything, mock.Anything, origMsg.ID, false).
				Return(origMsg, nil)

			mc.db.Mock.ExpectExec("INSERT.*reliable_msgs").WillReturnResult(driver.ResultNoRows)
		})
	defer done()

	p := &peer{
		ctx:       ctx,
		tm:        tm,
		transport: tp.t,
	}

	pmd := &components.PrivacyGroupMessageDistribution{
		Domain: "domain1",
		Group:  pldtypes.RandBytes(32),
		ID:     origMsg.ID,
	}

	rm := &pldapi.ReliableMessage{
		ID:          origMsg.ID,
		Sequence:    50,
		MessageType: pldapi.RMTPrivacyGroupMessage.Enum(),
		Node:        "node2",
		Metadata:    pldtypes.JSONString(pmd),
		Created:     pldtypes.TimestampNow(),
	}

	sentMessages := make(chan *prototk.PaladinMsg, 1)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	err := p.processReliableMsgPage(tm.persistence.NOTX(), []*pldapi.ReliableMessage{rm})
	require.NoError(t, err)

	sentMsg := <-sentMessages

	rMsg, err := parseReceivedMessage(ctx, "node2", sentMsg)
	require.NoError(t, err)
	require.Equal(t, RMHMessageTypePrivacyGroupMessage, rMsg.MessageType)

	receivedMsg, err := parsePrivacyGroupMessage(ctx, rMsg.FromNode, rMsg.MessageID, rMsg.Payload)
	require.NoError(t, err)
	origMsg.Received = receivedMsg.Received // expect to be changed on incoming message
	origMsg.Node = receivedMsg.Node         // expect to be changed on incoming message
	require.Equal(t, origMsg, receivedMsg)
}

func TestProcessReliableMsgPageReceipt(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.db.Mock.ExpectExec("INSERT.*reliable_msgs").WillReturnResult(driver.ResultNoRows)
		})
	defer done()

	p := &peer{
		ctx:       ctx,
		tm:        tm,
		transport: tp.t,
	}

	receipt := &components.ReceiptInput{
		Domain:        "domain1",
		ReceiptType:   components.RT_Success,
		TransactionID: uuid.New(),
	}

	rm := &pldapi.ReliableMessage{
		ID:          uuid.New(),
		Sequence:    50,
		MessageType: pldapi.RMTReceipt.Enum(),
		Node:        "node2",
		Metadata:    pldtypes.JSONString(receipt),
		Created:     pldtypes.TimestampNow(),
	}

	sentMessages := make(chan *prototk.PaladinMsg, 1)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	err := p.processReliableMsgPage(tm.persistence.NOTX(), []*pldapi.ReliableMessage{rm})
	require.NoError(t, err)

	sentMsg := <-sentMessages

	rMsg, err := parseReceivedMessage(ctx, "node2", sentMsg)
	require.NoError(t, err)
	require.Equal(t, RMHMessageTypeReceipt, rMsg.MessageType)

	receivedReceipt, err := parseMessageReceiptDistribution(ctx, rMsg.MessageID, rMsg.Payload)
	require.NoError(t, err)
	require.Equal(t, "domain1", receivedReceipt.Domain)
	require.Equal(t, components.RT_Success, receivedReceipt.ReceiptType)
	require.Equal(t, receipt.TransactionID, receivedReceipt.TransactionID)
}
