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
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type domainSimulatorFn func(callbacks simCallbacks, iReq pb.Message) (pb.Message, error)

type domainSimulator struct {
	tb              *testbed
	grpcClient      grpc.ClientConnInterface
	messageHandlers map[protoreflect.FullName]domainSimulatorFn
	done            chan struct{}
}

// simCallbacks let UT simulation domains make gRPC calls with request/response semantics
// back to the testbed as if they were real domains.
type simCallbacks interface {
	FindAvailableStates(schemaID string, query *filters.QueryJSON) (s []*proto.StoredState, err error)
}

func newDomainSimulator(t *testing.T, messageHandlers map[protoreflect.FullName]domainSimulatorFn) (context.Context, func(res interface{}, method string, params ...interface{}) error, func()) {

	url, tb, done := newUnitTestbed(t)

	rpcCall := newSimulatorRPCClient(t, url)

	conn, err := grpc.NewClient("unix:"+tb.socketFile, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	ds := &domainSimulator{
		tb:              tb,
		grpcClient:      conn,
		messageHandlers: messageHandlers,
		done:            make(chan struct{}),
	}

	toDomain, err := tb.bus.Broker().Listen(tb.ctx, tb.destToDomain)
	assert.NoError(t, err)

	go ds.messageHandler(toDomain)
	return tb.ctx, rpcCall, func() {
		done()
		ds.waitDone()
	}
}

func (ds *domainSimulator) messageHandler(toDomain commsbus.MessageHandler) {
	defer close(ds.done)
	tb := ds.tb
	for {
		select {
		case <-tb.ctx.Done():
			log.L(tb.ctx).Infof("Domain simulator event handler shutting down")
			return
		case msgToDomain := <-toDomain.Channel:
			msgType := msgToDomain.Body.ProtoReflect().Descriptor().FullName()
			log.L(tb.ctx).Infof("Domain simulator received received %s [%s]", msgToDomain.ID, msgType)
			inflight := tb.getInflight(msgToDomain.CorrelationID)
			if inflight != nil {
				inflight.done <- &msgToDomain
			} else if msgToDomain.ReplyTo != nil {
				// Need to get off this routine to handle it
				go ds.handleRequestToDomain(msgType, msgToDomain)
			}
		}
	}
}

func (ds *domainSimulator) handleRequestToDomain(msgType protoreflect.FullName, msgToDomain commsbus.Message) {
	tb := ds.tb
	handler := ds.messageHandlers[msgType]
	var res pb.Message
	var err error
	if handler == nil {
		err = fmt.Errorf("no handler for type %s", msgType)
	} else {
		log.L(tb.ctx).Infof("Calling simulation of %s", msgType)
		res, err = handler(ds, msgToDomain.Body)
	}
	if err != nil {
		res = &proto.DomainAPIError{
			ErrorMessage: err.Error(),
		}
	}
	_ = tb.bus.Broker().SendMessage(tb.ctx, commsbus.Message{
		ID:            uuid.New().String(),
		CorrelationID: &msgToDomain.ID,
		Destination:   *msgToDomain.ReplyTo,
		Body:          res,
	})
}

func (ds *domainSimulator) FindAvailableStates(schemaID string, query *filters.QueryJSON) ([]*proto.StoredState, error) {
	var res *proto.FindAvailableStatesResponse
	qs, err := json.Marshal(query)
	if err == nil {
		tb := ds.tb
		req := &proto.FindAvailableStatesRequest{
			SchemaId:  schemaID,
			QueryJson: string(qs),
		}
		err = syncExchange(tb.ctx, tb, tb.destFromDomain, tb.destToDomain, req, &res)
	}
	if err != nil {
		return nil, err
	}
	return res.States, err
}

func (ds *domainSimulator) waitDone() {
	<-ds.done
}

func simRequestToProto[T pb.Message](t *testing.T, iReq pb.Message) T {
	req := new(T)
	assert.Equal(t, (*req).ProtoReflect().Descriptor().FullName(), iReq.ProtoReflect().Descriptor().FullName())
	*req = iReq.(T)
	return *req
}

func newSimulatorRPCClient(t *testing.T, url string) func(res interface{}, method string, params ...interface{}) error {
	rpcClient := rpcbackend.NewRPCClient(resty.New().SetBaseURL(url))
	return func(res interface{}, method string, params ...interface{}) error {
		ctx, cancelCtx := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancelCtx()
		err := rpcClient.CallRPC(ctx, &res, method, params...)
		if err != nil {
			return err.Error()
		}
		t.Logf("%s: %v", method, res)
		return nil
	}
}
