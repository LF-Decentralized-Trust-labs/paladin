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

package noto

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"time"

	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type domain struct {
	conn         *grpc.ClientConn
	dest         *string
	client       pb.KataMessageServiceClient
	stream       pb.KataMessageService_ListenClient
	stopListener context.CancelFunc
	done         chan bool
}

func Start(ctx context.Context, addr string) (*domain, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect gRPC: %v", err)
	}

	d := &domain{
		conn:   conn,
		client: pb.NewKataMessageServiceClient(conn),
	}
	return d, d.waitForReady(ctx)
}

func (d *domain) waitForReady(ctx context.Context) error {
	status, err := d.client.Status(ctx, &pb.StatusRequest{})
	delay := 0
	for !status.GetOk() {
		time.Sleep(time.Second)
		delay++
		if delay > 2 {
			return fmt.Errorf("server was not ready after 2 seconds")
		}
		status, err = d.client.Status(ctx, &pb.StatusRequest{})
	}
	if err != nil {
		return err
	}
	if !status.GetOk() {
		return fmt.Errorf("got non-OK status from server")
	}
	return nil
}

func (d *domain) Close() error {
	if d.stream != nil {
		if err := d.stream.CloseSend(); err != nil {
			return err
		}
		d.done <- true
		d.stopListener()
	}
	if d.conn != nil {
		if err := d.conn.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (d *domain) Listen(ctx context.Context, dest string) error {
	d.dest = &dest
	d.done = make(chan bool, 1)

	var err error
	var listenerContext context.Context

	listenerContext, d.stopListener = context.WithCancel(ctx)
	d.stream, err = d.client.Listen(listenerContext, &pb.ListenRequest{Destination: dest})
	if err != nil {
		return fmt.Errorf("failed to listen for domain events: %v", err)
	}

	go d.handler()
	return nil
}

func (d *domain) sendReply(ctx context.Context, message *pb.Message, reply proto.Message) error {
	body, err := anypb.New(reply)
	if err == nil {
		_, err = d.client.SendMessage(ctx, &pb.Message{
			Destination:   *message.ReplyTo,
			CorrelationId: &message.Id,
			Body:          body,
			ReplyTo:       d.dest,
		})
	}
	return err
}

func (d *domain) handler() {
	ctx := context.Background()
	for {
		in, err := d.stream.Recv()
		select {
		case <-d.done:
			return
		default:
			// do nothing
		}
		if err != nil {
			log.Printf("Error receiving message - terminating handler loop: %v", err)
			return
		}
		err = d.handleMessage(ctx, in)
		if err != nil {
			log.Printf("Error handling message - terminating handler loop: %v", err)
			return
		}
	}
}

func (d *domain) handleMessage(ctx context.Context, message *pb.Message) error {
	body, err := message.Body.UnmarshalNew()
	if err != nil {
		return err
	}

	switch m := body.(type) {
	case *pb.ConfigureDomainRequest:
		log.Printf("Configuring domain: %s", m.Name)
		response := &pb.ConfigureDomainResponse{
			DomainConfig: &pb.DomainConfig{
				ConstructorAbiJson: `{
						"inputs": [
							{
							"internalType": "address",
							"name": "notary",
							"type": "address"
							}
						],
						"stateMutability": "nonpayable",
						"type": "constructor"
					}`,
				FactoryContractAddress: "0x9180ff8fa5c502b9bfe5dfeaf477e157dbfaba5c",
				FactoryContractAbiJson: "[]",
				AbiStateSchemasJson:    []string{},
			},
		}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.InitDomainRequest:
		log.Printf("Initializing domain: %s", m.AbiStateSchemaIds)
		response := &pb.InitDomainResponse{}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.DomainAPIError:
		log.Printf("Received error: %s", m.ErrorMessage)

	default:
		log.Printf("Unknown type: %s", reflect.TypeOf(m))
	}

	return nil
}
