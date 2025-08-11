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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type transport struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	conf *pldconf.TransportConfig
	tm   *transportManager
	id   uuid.UUID
	name string
	api  components.TransportManagerToTransport

	initialized atomic.Bool
	initRetry   *retry.Retry

	initError atomic.Pointer[error]
	initDone  chan struct{}
}

func (tm *transportManager) newTransport(id uuid.UUID, name string, conf *pldconf.TransportConfig, toTransport components.TransportManagerToTransport) *transport {
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

func (t *transport) send(ctx context.Context, nodeName string, msg *prototk.PaladinMsg) error {

	_, err := t.api.SendMessage(ctx, &prototk.SendMessageRequest{
		Node:    nodeName,
		Message: msg,
	})
	if err != nil {
		return err
	}
	var correlIDStr string
	if msg.CorrelationId != nil {
		correlIDStr = *msg.CorrelationId
	}
	log.L(ctx).Debugf("transport %s message sent id=%s (cid=%s) node=%s component=%s type=%s", t.name, msg.MessageId, correlIDStr, nodeName, msg.Component, msg.MessageType)
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

func parseReceivedMessage(ctx context.Context, fromNode string, msg *prototk.PaladinMsg) (*components.ReceivedMessage, error) {
	if msg == nil || len(msg.Payload) == 0 || len(msg.MessageType) == 0 {
		log.L(ctx).Errorf("Invalid message from transport: %s", protoToJSON(msg))
		return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
	}

	msgID, err := uuid.Parse(msg.MessageId)
	if err != nil {
		log.L(ctx).Errorf("Invalid messageId from transport: %s", protoToJSON(msg))
		return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
	}

	var correlationID *uuid.UUID
	if msg.CorrelationId != nil {
		parsedUUID, err := uuid.Parse(*msg.CorrelationId)
		if err != nil {
			log.L(ctx).Errorf("Invalid correlationId from transport: %s", protoToJSON(msg))
			return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidMessage)
		}
		correlationID = &parsedUUID
	}

	return &components.ReceivedMessage{
		FromNode:      fromNode,
		MessageID:     msgID,
		CorrelationID: correlationID,
		MessageType:   msg.MessageType,
		Payload:       msg.Payload,
	}, nil

}

// Transport callback to the transport manager when a message is received
func (t *transport) ReceiveMessage(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	if err := t.checkInit(ctx); err != nil {
		return nil, err
	}

	msg := req.Message

	rMsg, err := parseReceivedMessage(ctx, req.FromNode, msg)
	if err != nil {
		return nil, err
	}

	p, err := t.tm.getPeer(ctx, req.FromNode, false /* we do not require a connection for sending here */)
	if err != nil {
		return nil, err
	}

	p.updateReceivedStats(msg)

	log.L(ctx).Debugf("transport %s message received from %s id=%s (cid=%s)", t.name, p.Name, rMsg.MessageID, pldtypes.StrOrEmpty(msg.CorrelationId))
	if log.IsTraceEnabled() {
		log.L(ctx).Tracef("transport %s message received: %s", t.name, protoToJSON(msg))
	}

	if err := t.deliverMessage(ctx, p, msg.Component, rMsg); err != nil {
		return nil, err
	}

	return &prototk.ReceiveMessageResponse{}, nil
}

func (t *transport) deliverMessage(ctx context.Context, p *peer, component prototk.PaladinMsg_Component, msg *components.ReceivedMessage) error {

	switch component {
	case prototk.PaladinMsg_RELIABLE_MESSAGE_HANDLER:
		_ = t.tm.reliableMsgWriter.Queue(ctx, &reliableMsgOp{
			p:   p,
			msg: msg,
		})
	case prototk.PaladinMsg_TRANSACTION_ENGINE:
		t.tm.privateTxManager.HandlePaladinMsg(ctx, msg)
	case prototk.PaladinMsg_IDENTITY_RESOLVER:
		t.tm.identityResolver.HandlePaladinMsg(ctx, msg)
	default:
		log.L(ctx).Errorf("Component not found for message '%s': %s", msg.MessageID, component)
		return i18n.NewError(ctx, msgs.MsgTransportComponentNotFound, component.String())
	}

	return nil
}

func (t *transport) GetTransportDetails(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
	if req.Node == t.tm.localNodeName {
		return nil, i18n.NewError(ctx, msgs.MsgTransportInvalidLocalNode, req.Node)
	}

	// Do a cache-optimized in the registry manager to get the details of the transport.
	// We expect this to succeed because we did it before sending (see notes on Send() function)
	var transportDetails string
	availableTransports, err := t.tm.registryManager.GetNodeTransports(ctx, req.Node)
	for _, atd := range availableTransports {
		if atd.Transport == t.name {
			transportDetails = atd.Details
			break
		}
	}
	if transportDetails == "" {
		return nil, i18n.WrapError(ctx, err, msgs.MsgTransportDetailsNotAvailable, t.name, req.Node)
	}
	return &prototk.GetTransportDetailsResponse{
		TransportDetails: transportDetails,
	}, nil
}

func (t *transport) getLocalDetails(ctx context.Context) (string, error) {
	res, err := t.api.GetLocalDetails(ctx, &prototk.GetLocalDetailsRequest{})
	if err != nil {
		return "", err
	}
	return res.TransportDetails, nil
}

func (t *transport) close() {
	t.cancelCtx()
	<-t.initDone
}
