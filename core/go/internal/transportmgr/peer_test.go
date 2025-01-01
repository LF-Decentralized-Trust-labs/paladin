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
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func mockGetStateOk(mc *mockComponents) components.TransportClient {
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
	return nil
}

func TestReliableMessageResendRealDB(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		mockGetStateOk,
	)
	defer done()

	tm.sendShortRetry = retry.NewRetryLimited(&pldconf.RetryConfigWithMax{
		MaxAttempts: confutil.P(1),
	})
	tm.quiesceTimeout = 10 * time.Millisecond
	tm.reliableMessageResend = 10 * time.Millisecond
	tm.peerInactivityTimeout = 1 * time.Second

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
	<-p.done

}

func TestSendBadReliableMessageMarkedFailRealDB(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, true,
		mockGoodTransport,
	)
	defer done()

	tm.sendShortRetry = retry.NewRetryLimited(&pldconf.RetryConfigWithMax{
		MaxAttempts: confutil.P(1),
	})
	tm.quiesceTimeout = 10 * time.Millisecond
	tm.reliableMessageResend = 10 * time.Millisecond
	tm.peerInactivityTimeout = 1 * time.Second

	mockActivateDeactivateOk(tp)

	rm := &components.ReliableMessage{
		MessageType: components.RMTState.Enum(),
		Node:        "node2",
		// Missing metadata
	}
	postCommit, err := tm.SendReliable(ctx, tm.persistence.DB(), rm)
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
	require.Regexp(t, "PD012017", rmWithAck.Ack.Error)

}

func TestNameSortedPeers(t *testing.T) {

	peerList := nameSortedPeers{
		{name: "ccc"},
		{name: "aaa"},
		{name: "ddd"},
		{name: "bbb"},
	}

	sort.Sort(peerList)

	require.Equal(t, nameSortedPeers{
		{name: "aaa"},
		{name: "bbb"},
		{name: "ccc"},
		{name: "ddd"},
	}, peerList)

}

func TestConnectionRace(t *testing.T) {

	connWaiting := make(chan struct{})
	connRelease := make(chan struct{})

	ctx, tm, tp, done := newTestTransport(t, false,
		func(mc *mockComponents) components.TransportClient {
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
			return nil
		},
	)
	defer done()

	mockActivateDeactivateOk(tp)
	connDone := make(chan bool)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := tm.connectPeer(ctx, "node2")
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

	_, err := tm.getPeer(ctx, "node2")
	assert.Regexp(t, "pop", err)

}

func TestActivateBadPeerInfo(t *testing.T) {

	ctx, tm, tp, done := newTestTransport(t, false, mockGoodTransport)
	defer done()

	tp.Functions.ActivateNode = func(ctx context.Context, anr *prototk.ActivateNodeRequest) (*prototk.ActivateNodeResponse, error) {
		return &prototk.ActivateNodeResponse{PeerInfoJson: ""}, nil
	}

	_, err := tm.getPeer(ctx, "node2")
	assert.Regexp(t, "PD012015", err)

}
