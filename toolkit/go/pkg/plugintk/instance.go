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
package plugintk

import (
	"context"
	"errors"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/retry"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/inflight"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type pluginInstance[M any] struct {
	pluginType string
	id         string
	connString string
	ctx        context.Context
	cancelCtx  context.CancelFunc
	factory    *pluginFactory[M]
	connector  PluginConnector[M]
	impl       PluginImplementation[M]
	retry      *retry.Retry
	done       chan struct{}
}

type pluginRun[M any] struct {
	pi         *pluginInstance[M]
	ctx        context.Context
	cancelCtx  context.CancelFunc
	stream     grpc.BidiStreamingClient[M, M]
	handler    PluginHandler[M]
	inflight   *inflight.InflightManager[uuid.UUID, PluginMessage[M]]
	senderChl  chan *M
	senderDone chan struct{}
}

func newPluginInstance[M any](pf *pluginFactory[M], connString, pluginID string) *pluginInstance[M] {
	pi := &pluginInstance[M]{
		pluginType: pf.pluginType.String(),
		factory:    pf,
		connector:  pf.connector,
		impl:       pf.impl,
		connString: connString,
		id:         pluginID,
		retry:      retry.NewRetryIndefinite(&pldconf.GenericRetryDefaults.RetryConfig),
		done:       make(chan struct{}),
	}
	pi.ctx, pi.cancelCtx = context.WithCancel(log.WithLogField(context.Background(), "plugin", pluginID))
	return pi
}

func (pi *pluginInstance[M]) run() {
	// We run until our context is cancelled
	defer close(pi.done)
	ccErr := pi.retry.Do(pi.ctx, func(attempt int) (retryable bool, err error) {
		// Create a new runner each time round the reconnect retry loop
		pr := &pluginRun[M]{pi: pi}
		return true, pr.run()
	})
	log.L(pi.ctx).Debugf("exiting (%v)", ccErr)
}

func (pi *pluginInstance[M]) connect(conn *grpc.ClientConn) (grpc.BidiStreamingClient[M, M], error) {
	client := prototk.NewPluginControllerClient(conn)
	return pi.connector(pi.ctx, client)
}

func (pr *pluginRun[M]) run() error {
	pr.inflight = inflight.NewInflightManager[uuid.UUID, PluginMessage[M]](uuid.Parse)

	// Ensure we cleanup
	var conn *grpc.ClientConn
	defer func() {
		// Cancel any in-flight requests
		pr.inflight.Close()
		// Close the connection
		if conn != nil {
			_ = conn.Close()
		}
		// Cancel the ctx and wait for the sender to finish
		if pr.cancelCtx != nil {
			pr.cancelCtx()
		}
		if pr.senderDone != nil {
			<-pr.senderDone
		}
	}()

	// Create the long-lived bi-directional stream to the plugin controller
	conn, err := grpc.NewClient(pr.pi.connString, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err == nil {
		pr.stream, err = pr.pi.connect(conn)
	}
	if err != nil {
		return err
	}

	pr.ctx, pr.cancelCtx = context.WithCancel(log.WithLogField(
		pr.stream.Context(), pr.pi.pluginType, pr.pi.id))

	// Start a separate sender routine for our stream
	pr.senderChl = make(chan *M)
	pr.senderDone = make(chan struct{})
	go pr.sender()

	// Send the register
	regMsg := pr.pi.impl.Wrap(new(M))
	regHeader := regMsg.Header()
	regHeader.PluginId = pr.pi.id
	regHeader.MessageId = uuid.NewString()
	regHeader.MessageType = prototk.Header_REGISTER
	pr.send(regMsg.Message())

	// Initialize the implementation
	pr.handler = pr.pi.impl.NewHandler(pr)

	// Serve the receiver
	return pr.serve()
}

func (pr *pluginRun[M]) sender() {
	defer close(pr.senderDone)
	for {
		var msg *M
		select {
		case msg = <-pr.senderChl:
		case <-pr.ctx.Done():
			log.L(pr.ctx).Debugf("sender stopping")
			return
		}
		if err := pr.stream.Send(msg); err != nil {
			// we still just loop round as serve() should detect the error and stop us
			log.L(pr.ctx).Errorf("send failed: %s", err)
		}
	}
}

func (pr *pluginRun[M]) send(msg *M) {
	select {
	case pr.senderChl <- msg:
	case <-pr.ctx.Done():
	}
}

func (pr *pluginRun[M]) serve() error {
	l := log.L(pr.ctx)
	for {
		sMsg, err := pr.stream.Recv()
		if err != nil {
			log.L(pr.ctx).Errorf("received failed: %s", err)
			return err
		}
		msg := pr.pi.impl.Wrap(sMsg)
		header := msg.Header()

		// Handling based on the type
		switch header.MessageType {
		case prototk.Header_REQUEST_TO_PLUGIN:
			// Dispatch to another go routine so we can continue to serve requests
			go pr.handleRequestToPlugin(msg)
		case prototk.Header_RESPONSE_TO_PLUGIN, prototk.Header_ERROR_RESPONSE:
			// Find the in-flight request and complete it
			if header.CorrelationId == nil {
				l.Warnf("[NO_CORRELATION_ID] <== [%s] %T", header.MessageId, msg.ResponseToPlugin())
				continue
			}
			correlID := *header.CorrelationId
			req := pr.inflight.GetInflightStr(correlID)
			if req == nil {
				l.Warnf("[%s] <== [%s] EXPIRED", correlID, header.MessageId)
				continue
			}
			req.Complete(msg)
		default:
			// We don't expect any other message types to be sent to a plugin right now
			// Just log and ignore
			l.Warnf("UNEXPECTED %s", PluginMessageToJSON(msg))
		}
	}

}

func (pr *pluginRun[M]) handleRequestToPlugin(msg PluginMessage[M]) {

	// Log the request and generate a reply identifier
	header := msg.Header()
	timeReceived := time.Now()
	log.L(pr.ctx).Infof("[%s] --> %T", header.MessageId, msg.RequestToPlugin())
	replyID := uuid.NewString()
	var replyHeader *prototk.Header

	// Call the handler
	reply, err := pr.handler.RequestToPlugin(pr.ctx, msg)
	if err != nil {
		// Build an new message containing only the error
		reply = pr.pi.impl.Wrap(new(M))
		replyHeader = reply.Header()
		errorMessage := err.Error()
		replyHeader.MessageType = prototk.Header_ERROR_RESPONSE
		replyHeader.ErrorMessage = &errorMessage
		log.L(pr.ctx).Errorf("[%s] <-- [%s] ERROR [%s]: %s", header.MessageId, replyID, time.Since(timeReceived), errorMessage)
	} else {
		// The handler generated a reply - we just update the header
		replyHeader = reply.Header()
		replyHeader.MessageType = prototk.Header_RESPONSE_FROM_PLUGIN
		log.L(pr.ctx).Infof("[%s] <-- [%s] %T [%s]", header.MessageId, replyID, msg.ResponseFromPlugin(), time.Since(timeReceived))
	}
	replyHeader.PluginId = pr.pi.id
	replyHeader.CorrelationId = &header.MessageId
	replyHeader.MessageId = replyID

	// Send back the reply
	pr.send(reply.Message())
}

func (pr *pluginRun[M]) RequestFromPlugin(ctx context.Context, req PluginMessage[M]) (PluginMessage[M], error) {

	// We are responsible for the header (we ignore anything set by the caller)
	reqID := uuid.New()
	header := req.Header()
	header.PluginId = pr.pi.id
	header.CorrelationId = nil
	header.MessageId = reqID.String()
	header.MessageType = prototk.Header_REQUEST_FROM_PLUGIN
	header.ErrorMessage = nil

	// Log under our context so we get the plugin ID
	l := log.L(pr.ctx)
	l.Infof("[%s] ==> %T", reqID, req.RequestFromPlugin())

	// Create the in-flight record - under the request context (inflight manager will be cancelled if we end)
	inflight := pr.inflight.AddInflight(ctx, reqID)
	defer inflight.Cancel()

	// Send the request
	pr.send(req.Message())

	// Wait for a response, or cancel
	res, err := inflight.Wait()
	if err != nil {
		l.Infof("[%s] <== CANCELLED [%s]", reqID, inflight.Age())
		return nil, err
	}
	if res.Header().MessageType == prototk.Header_ERROR_RESPONSE {
		msg := res.Header().ErrorMessage
		if msg != nil {
			err = errors.New(*msg)
		} else {
			// This is unexpected, but better to handle that give an empty result
			err = i18n.NewError(ctx, pldmsgs.MsgPluginErrorFromServerNoMsg)
		}
		l.Infof("[%s] <== ERROR [%s]: %s", reqID, inflight.Age(), err)
		return nil, err
	}

	l.Infof("[%s] <== [%s] %T [%s]", reqID, res.Header().MessageId, req.ResponseToPlugin(), inflight.Age())
	return res, nil
}
