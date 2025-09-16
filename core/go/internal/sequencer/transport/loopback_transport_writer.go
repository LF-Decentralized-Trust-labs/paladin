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

package transport

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
)

type LoopbackTransportManager interface {
	Send(ctx context.Context, send *components.FireAndForgetMessageSend) error
}

func NewLoopbackTransportWriter(loopbackHandler func(ctx context.Context, message *components.ReceivedMessage)) *loopbackTransportWriter {
	return &loopbackTransportWriter{
		loopbackHandler: loopbackHandler,
	}
}

type loopbackTransportWriter struct {
	loopbackHandler func(ctx context.Context, message *components.ReceivedMessage)
}

func (ltw *loopbackTransportWriter) Send(ctx context.Context, send *components.FireAndForgetMessageSend) error {
	receivedMessage := &components.ReceivedMessage{
		FromNode:    send.Node,
		MessageType: send.MessageType,
		Payload:     send.Payload,
	}
	if send.MessageID != nil {
		receivedMessage.MessageID = *send.MessageID
	}
	if send.CorrelationID != nil {
		receivedMessage.CorrelationID = send.CorrelationID
	}
	ltw.loopbackHandler(ctx, receivedMessage)
	return nil
}
