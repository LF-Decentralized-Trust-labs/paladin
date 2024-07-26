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
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"google.golang.org/protobuf/encoding/protojson"
	pb "google.golang.org/protobuf/proto"
)

var DOMAIN_API_ERROR = string((&proto.DomainAPIError{}).ProtoReflect().Descriptor().FullName())
var CONFIGURE_REQUEST = string((&proto.ConfigureDomainRequest{}).ProtoReflect().Descriptor().FullName())
var INIT_DOMAIN_REQUEST = string((&proto.InitDomainRequest{}).ProtoReflect().Descriptor().FullName())

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
	log.L(ctx).Infof("--> %s [%s]", msg.ID, msg.Type)
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

func (tb *testbed) syncExchangeToDomain(ctx context.Context, in, out pb.Message) error {

	jsonIn, err := protojson.Marshal(in)
	if err != nil {
		return fmt.Errorf("bad request: %s", err)
	}
	id := uuid.New().String()
	requestMsg := commsbus.Message{
		Destination: tb.destToDomain,
		ReplyTo:     &tb.destFromDomain,
		ID:          id,
		Body:        jsonIn,
		Type:        string(in.ProtoReflect().Descriptor().FullName()),
	}
	inFlight := tb.addInflight(ctx, &requestMsg)
	defer tb.clearInFlight(inFlight)

	if err = tb.bus.Broker().SendMessage(ctx, requestMsg); err != nil {
		return fmt.Errorf("failed to send request: %s", err)
	}

	reply, err := tb.waitInFlight(ctx, inFlight)
	if err != nil {
		return err
	}

	if reply.Type == DOMAIN_API_ERROR {
		var pbErr proto.DomainAPIError
		err = protojson.Unmarshal(reply.Body, &pbErr)
		if err == nil {
			return fmt.Errorf(pbErr.ErrorMessage)
		}
	}

	err = protojson.Unmarshal(reply.Body, out)
	if err != nil {
		return fmt.Errorf("bad response: %s", err)
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
			log.L(tb.ctx).Infof("Testbed received %s [%s]", msgFromDomain.ID, msgFromDomain.Type)
			inflight := tb.getInflight(msgFromDomain.CorrelationID)
			if inflight != nil {
				inflight.done <- &msgFromDomain
			} else {
				tb.handleRequestFromDomain(msgFromDomain)
			}
		}
	}
}

func (tb *testbed) handleRequestFromDomain(msgFromDomain commsbus.Message) {
	var reply pb.Message
	switch msgFromDomain.Type {
	default:
		reply = &proto.DomainAPIError{
			ErrorMessage: fmt.Sprintf("unknown request type: %s", msgFromDomain.Type),
		}
	}
	resBytes, _ := protojson.Marshal(reply)
	_ = tb.bus.Broker().SendMessage(tb.ctx, commsbus.Message{
		ID:            uuid.New().String(),
		Type:          string(reply.ProtoReflect().Descriptor().FullName()),
		CorrelationID: &msgFromDomain.ID,
		Destination:   *msgFromDomain.ReplyTo,
		Body:          resBytes,
	})
}
