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

package zeto

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

type inflightRequest struct {
	req    *pb.Message
	queued time.Time
	done   chan *pb.Message
}

type replyTracker struct {
	inflight     map[string]*inflightRequest
	inflightLock sync.Mutex
	client       pb.KataMessageServiceClient
}

func (tb *replyTracker) addInflight(ctx context.Context, msg *pb.Message) *inflightRequest {
	inFlight := &inflightRequest{
		req:    msg,
		queued: time.Now(),
		done:   make(chan *pb.Message, 1),
	}
	log.L(ctx).Infof("--> %s [%s]", msg.Id, msg.Body.ProtoReflect().Descriptor().FullName())
	tb.inflightLock.Lock()
	defer tb.inflightLock.Unlock()
	tb.inflight[msg.Id] = inFlight
	return inFlight
}

func (tb *replyTracker) getInflight(correlID *string) *inflightRequest {
	if correlID == nil {
		return nil
	}
	tb.inflightLock.Lock()
	defer tb.inflightLock.Unlock()
	return tb.inflight[*correlID]
}

func (tb *replyTracker) waitInFlight(ctx context.Context, inFlight *inflightRequest) (*pb.Message, error) {
	select {
	case <-ctx.Done():
		log.L(ctx).Errorf("<!- %s", inFlight.req.Id)
		return nil, fmt.Errorf("timeout")
	case reply := <-inFlight.done:
		log.L(ctx).Infof("<-- %s", inFlight.req.Id)
		return reply, nil
	}
}

func (tb *replyTracker) clearInFlight(inFlight *inflightRequest) {
	tb.inflightLock.Lock()
	defer tb.inflightLock.Unlock()
	delete(tb.inflight, inFlight.req.Id)
}

func requestReply[I, O protoreflect.ProtoMessage](ctx context.Context, tb *replyTracker, to, from string, in I, out *O) error {
	id := uuid.New().String()
	body, err := anypb.New(in)
	if err != nil {
		return err
	}

	requestMsg := &pb.Message{
		Destination: to,
		ReplyTo:     &from,
		Id:          id,
		Body:        body,
	}
	inFlight := tb.addInflight(ctx, requestMsg)
	defer tb.clearInFlight(inFlight)

	_, err = tb.client.SendMessage(ctx, requestMsg)
	if err != nil {
		return err
	}

	reply, err := tb.waitInFlight(ctx, inFlight)
	if err != nil {
		return err
	}

	replyBody, err := reply.Body.UnmarshalNew()
	if err != nil {
		return err
	}

	switch m := replyBody.(type) {
	case *pb.DomainAPIError:
		return fmt.Errorf(m.ErrorMessage)
	case O:
		*out = m
		return nil
	default:
		return fmt.Errorf("bad response expected=%T actual=%T", *out, reply.Body)
	}
}
