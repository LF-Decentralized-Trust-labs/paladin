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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	pb "google.golang.org/protobuf/proto"
)

var DOMAIN_API_ERROR = (&proto.DomainAPIError{}).ProtoReflect().Descriptor().FullName()
var CONFIGURE_DOMAIN = (&proto.ConfigureDomainRequest{}).ProtoReflect().Descriptor().FullName()
var INIT_DOMAIN = (&proto.InitDomainRequest{}).ProtoReflect().Descriptor().FullName()
var INIT_DEPLOY = (&proto.InitDeployTransactionRequest{}).ProtoReflect().Descriptor().FullName()
var PREPARE_DEPLOY = (&proto.PrepareDeployTransactionRequest{}).ProtoReflect().Descriptor().FullName()
var INIT_TRANSACTION = (&proto.InitTransactionRequest{}).ProtoReflect().Descriptor().FullName()
var ASSEMBLE_TRANSACTION = (&proto.AssembleTransactionRequest{}).ProtoReflect().Descriptor().FullName()
var ENDORSE_TRANSACTION = (&proto.EndorseTransactionRequest{}).ProtoReflect().Descriptor().FullName()
var PREPARE_TRANSACTION = (&proto.PrepareTransactionRequest{}).ProtoReflect().Descriptor().FullName()

var FIND_AVAILABLE_STATES = (&proto.FindAvailableStatesRequest{}).ProtoReflect().Descriptor().FullName()

type inflightRequest struct {
	req    *commsbus.Message
	queued time.Time
	done   chan *commsbus.Message
}

func (tb *testbed) addInflight(ctx context.Context, msg *commsbus.Message) *inflightRequest {
	inFlight := &inflightRequest{
		req:    msg,
		queued: time.Now(),
		done:   make(chan *commsbus.Message, 1),
	}
	log.L(ctx).Infof("--> %s [%s]", msg.ID, msg.Body.ProtoReflect().Descriptor().FullName())
	tb.inflightLock.Lock()
	defer tb.inflightLock.Unlock()
	tb.inflight[msg.ID] = inFlight
	return inFlight
}

func (tb *testbed) getInflight(correlID *string) *inflightRequest {
	if correlID == nil {
		return nil
	}
	tb.inflightLock.Lock()
	defer tb.inflightLock.Unlock()
	return tb.inflight[*correlID]
}

func (tb *testbed) waitInFlight(ctx context.Context, inFlight *inflightRequest) (*commsbus.Message, error) {
	select {
	case <-ctx.Done():
		log.L(ctx).Errorf("<!- %s", inFlight.req.ID)
		return nil, fmt.Errorf("timeout")
	case reply := <-inFlight.done:
		log.L(ctx).Infof("<-- %s", inFlight.req.ID)
		return reply, nil
	}
}

func (tb *testbed) clearInFlight(inFlight *inflightRequest) {
	tb.inflightLock.Lock()
	defer tb.inflightLock.Unlock()
	delete(tb.inflight, inFlight.req.ID)
}

func syncExchange[I, O pb.Message](ctx context.Context, tb *testbed, to, from string, in I, out *O) error {

	id := uuid.New().String()
	requestMsg := commsbus.Message{
		Destination: to,
		ReplyTo:     &from,
		ID:          id,
		Body:        in,
	}
	inFlight := tb.addInflight(ctx, &requestMsg)
	defer tb.clearInFlight(inFlight)

	if err := tb.bus.Broker().SendMessage(ctx, requestMsg); err != nil {
		return fmt.Errorf("failed to send request: %s", err)
	}

	reply, err := tb.waitInFlight(ctx, inFlight)
	if err != nil {
		return err
	}

	if reply.Body.ProtoReflect().Descriptor().FullName() == DOMAIN_API_ERROR {
		pbErr, ok := reply.Body.(*proto.DomainAPIError)
		if ok {
			return fmt.Errorf(pbErr.ErrorMessage)
		}
	}

	var ok bool
	*out, ok = reply.Body.(O)
	if !ok {
		return fmt.Errorf("bad response expected=%T actual=%T", *out, reply.Body)
	}

	return nil
}

func (tb *testbed) eventHandler() {
	for {
		select {
		case <-tb.ctx.Done():
			log.L(tb.ctx).Infof("Testbed event handler shutting down")
			return
		case msgFromDomain := <-tb.fromDomain.Channel:
			msgType := msgFromDomain.Body.ProtoReflect().Descriptor().FullName()
			log.L(tb.ctx).Infof("Testbed received %s [%s]", msgFromDomain.ID, msgType)
			inflight := tb.getInflight(msgFromDomain.CorrelationID)
			if inflight != nil {
				inflight.done <- &msgFromDomain
			} else if msgFromDomain.ReplyTo != nil {
				// Need to get off this routine to handle it
				go tb.handleRequestFromDomain(msgFromDomain)
			}
		}
	}
}

func (tb *testbed) handleRequestFromDomain(msgFromDomain commsbus.Message) {
	var reply pb.Message
	var err error
	defer func() {
		recovered := recover()
		if recovered != nil {
			log.L(tb.ctx).Errorf(string(debug.Stack()))
			reply = &proto.DomainAPIError{
				ErrorMessage: fmt.Sprintf("panic: %s", recovered),
			}
		} else if err != nil {
			reply = &proto.DomainAPIError{
				ErrorMessage: err.Error(),
			}
		}
		_ = tb.bus.Broker().SendMessage(tb.ctx, commsbus.Message{
			ID:            uuid.New().String(),
			CorrelationID: &msgFromDomain.ID,
			Destination:   *msgFromDomain.ReplyTo,
			Body:          reply,
		})
	}()

	msgType := msgFromDomain.Body.ProtoReflect().Descriptor().FullName()
	switch msgType {
	case FIND_AVAILABLE_STATES:
		reply, err = tb.findAvailableStates(msgFromDomain.Body.(*proto.FindAvailableStatesRequest))
	default:
		err = fmt.Errorf("unknown request type: %s", msgType)
	}
}

func (tb *testbed) findAvailableStates(req *proto.FindAvailableStatesRequest) (*proto.FindAvailableStatesResponse, error) {

	domain := tb.getDomainByUUID(req.DomainUuid)
	if domain == nil {
		return nil, fmt.Errorf("domain with runtime instance UUID %q not found", req.DomainUuid)
	}

	var query filters.QueryJSON
	err := json.Unmarshal([]byte(req.QueryJson), &query)
	if err != nil {
		return nil, fmt.Errorf("invalid query JSON: %s", err)
	}

	var states []*statestore.State
	err = tb.stateStore.RunInDomainContext(domain.name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		states, err = dsi.FindAvailableStates(req.SchemaId, &query)
		return err
	})
	if err != nil {
		return nil, err
	}

	pbStates := make([]*proto.StoredState, len(states))
	for i, s := range states {
		pbStates[i] = &proto.StoredState{
			HashId:   s.ID.String(),
			SchemaId: s.Schema.String(),
			StoredAt: s.CreatedAt.UnixNano(),
			DataJson: string(s.Data),
		}
		if s.Locked != nil {
			pbStates[i].Lock = &proto.StateLock{
				Sequence: s.Locked.Sequence.String(),
				Creating: s.Locked.Creating,
				Spending: s.Locked.Spending,
			}
		}
	}
	return &proto.FindAvailableStatesResponse{
		States: pbStates,
	}, nil

}
