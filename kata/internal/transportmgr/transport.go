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
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type transport struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf *TransportConfig
	tm   *transportManager
	id   uuid.UUID
	name string
	api  components.TransportManagerToTransport

	initialized atomic.Bool
	initRetry   *retry.Retry

	initError atomic.Pointer[error]
	initDone  chan struct{}
}

func (tm *transportManager) newTransport(id uuid.UUID, name string, conf *TransportConfig, toTransport components.TransportManagerToTransport) *transport {
	t := &transport{
		tm:        tm,
		conf:      conf,
		initRetry: retry.NewRetryIndefinite(&conf.Init.Retry),
		name:      name,
		id:        id,
		api:       toTransport,
		initDone:  make(chan struct{}),
	}
	t.ctx, t.cancelCtx = context.WithCancel(log.WithLogField(tm.bgCtx, "transport", t.name))
	return t
}

func (t *transport) init() {
	defer close(t.initDone)

	// We block retrying each part of init until we succeed, or are cancelled
	// (which the plugin manager will do if the transport disconnects)
	err := t.initRetry.Do(t.ctx, func(attempt int) (bool, error) {
		// Send the configuration to the transport for processing
		confJSON, _ := json.Marshal(&t.conf.Config)
		_, err := t.api.ConfigureTransport(t.ctx, &prototk.ConfigureTransportRequest{
			Name:       t.name,
			ConfigJson: string(confJSON),
		})
		return true, err
	})
	if err != nil {
		log.L(t.ctx).Debugf("transport initialization cancelled before completion: %s", err)
		t.initError.Store(&err)
	} else {
		log.L(t.ctx).Debugf("transport initialization complete")
		t.initialized.Store(true)
		// Inform the plugin manager callback
		t.api.Initialized()
	}
}

func (t *transport) checkInit(ctx context.Context) error {
	if !t.initialized.Load() {
		return i18n.NewError(ctx, msgs.MsgDomainNotInitialized)
	}
	return nil
}

func (t *transport) send(ctx context.Context, msg *prototk.Message) error {
	if err := t.checkInit(ctx); err != nil {
		return err
	}

	_, err := t.api.SendMessage(ctx, &prototk.SendMessageRequest{Message: msg})
	if err != nil {
		return err
	}
	var correlIDStr string
	if msg.CorrelationId != nil {
		correlIDStr = *msg.CorrelationId
	}
	log.L(ctx).Debugf("transport %s message sent id=%s (cid=%s)", t.name, msg.MessageId, correlIDStr)
	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("transport %s message sent: %s", t.name, protoToJSON(msg))
	}
	return nil
}

func protoToJSON(m proto.Message) (s string) {
	b, err := protojson.Marshal(m)
	if err == nil {
		s = string(b)
	}
	return
}

// Transport callback to the transport manager when a message is received
func (t *transport) ReceiveMessage(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	if err := t.checkInit(ctx); err != nil {
		return nil, err
	}

	msg := req.Message
	if msg == nil || len(msg.Payload) == 0 || len(msg.MessageType) == 0 {
		log.L(ctx).Errorf("Invalid message from transport: %s", protoToJSON(msg))
		return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
	}
	destNode, err := types.PrivateIdentityLocator(msg.Destination).Node(ctx, false)
	if err != nil || destNode != t.tm.localNodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidDestinationReceived, t.tm.localNodeName, msg.Destination)
	}
	_, _, err = types.PrivateIdentityLocator(msg.ReplyTo).Validate(ctx, "", false)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidReplyToReceived, msg.ReplyTo)
	}
	msgID, err := uuid.Parse(msg.MessageId)
	if err != nil {
		log.L(ctx).Errorf("Invalid messageId from transport: %s", protoToJSON(msg))
		return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
	}
	var pCorrelID *uuid.UUID
	var correlIDStr string
	if msg.CorrelationId != nil {
		correlIDStr = *msg.CorrelationId
		correlID, err := uuid.Parse(correlIDStr)
		if err != nil {
			log.L(ctx).Errorf("Invalid correlationId from transport: %s", protoToJSON(msg))
			return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
		}
		pCorrelID = &correlID
	}

	log.L(ctx).Debugf("transport %s message received id=%s (cid=%s)", t.name, msgID, correlIDStr)
	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("transport %s message received: %s", t.name, protoToJSON(msg))
	}
	transportMessage := &components.TransportMessage{
		MessageID:     msgID,
		CorrelationID: pCorrelID,
		Destination:   types.PrivateIdentityLocator(msg.Destination),
		ReplyTo:       types.PrivateIdentityLocator(msg.ReplyTo),
		Payload:       msg.Payload,
	}
	t.tm.engine.ReceiveTransportMessage(transportMessage)

	return &prototk.ReceiveMessageResponse{}, nil
}

func (t *transport) GetTransportDetails(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
	node, err := types.PrivateIdentityLocator(req.Destination).Node(ctx, false)
	if err != nil || node == t.tm.localNodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportInvalidDestinationSend, t.tm.localNodeName, req.Destination)
	}

	// Do a cache-optimized in the registry manager to get the details of the transport.
	// We expect this to succeed because we did it before sending (see notes on Send() function)
	var transportDetails string
	availableTransports, err := t.tm.registryManager.GetNodeTransports(ctx, node)
	for _, atd := range availableTransports {
		if atd.Transport == t.name {
			transportDetails = atd.TransportDetails
			break
		}
	}
	if transportDetails == "" {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportDetailsNotAvailable, t.name, node)
	}
	return &prototk.GetTransportDetailsResponse{
		TransportDetails: transportDetails,
	}, nil
}

func (t *transport) close() {
	t.cancelCtx()
	<-t.initDone
}
