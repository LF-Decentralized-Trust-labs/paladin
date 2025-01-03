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
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReceiveMessageStateWithAckRealDB(t *testing.T) {
	ctx, _, tp, done := newTestTransport(t, true,
		mockGoodTransport,
		func(mc *mockComponents, conf *pldconf.TransportManagerConfig) {
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, nil).Once()
		},
	)
	defer done()

	msgID := uuid.New()
	msg := &prototk.PaladinMsg{
		MessageId:     msgID.String(),
		CorrelationId: confutil.P(uuid.NewString()),
		Component:     prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER,
		MessageType:   RMHMessageTypeStateDistribution,
		Payload: tktypes.JSONString(&components.StateDistributionWithData{
			StateDistribution: components.StateDistribution{
				Domain:          "domain1",
				ContractAddress: tktypes.RandAddress().String(),
				SchemaID:        tktypes.RandHex(32),
				StateID:         tktypes.RandHex(32),
			},
			StateData: []byte(`{"some":"data"}`),
		}),
	}

	mockActivateDeactivateOk(tp)
	sentMessages := make(chan *prototk.PaladinMsg)
	tp.Functions.SendMessage = func(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		sent := req.Message
		sentMessages <- sent
		return nil, nil
	}

	rmr, err := tp.t.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{
		FromNode: "node2",
		Message:  msg,
	})
	require.NoError(t, err)
	assert.NotNil(t, rmr)

	ack := <-sentMessages
	require.JSONEq(t, string(ack.Payload), `{}`)
	require.Equal(t, msgID.String(), *ack.CorrelationId)
}
