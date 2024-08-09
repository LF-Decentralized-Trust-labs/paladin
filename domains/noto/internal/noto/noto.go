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
	"io"
	"log"
	"reflect"
	"time"

	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type domain struct {
	conn   *grpc.ClientConn
	client pb.KataMessageServiceClient
	stream pb.KataMessageService_ListenClient
	closed chan struct{}
}

func Start(ctx context.Context, addr string) (*domain, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect gRPC: %v", err)
	}

	client := pb.NewKataMessageServiceClient(conn)
	status, err := client.Status(ctx, &pb.StatusRequest{})

	delay := 0
	for !status.GetOk() {
		time.Sleep(time.Second)
		delay++
		status, err = client.Status(ctx, &pb.StatusRequest{})
		if delay > 2 {
			return nil, fmt.Errorf("server was not ready after 2 seconds")
		}
	}
	if err != nil {
		return nil, err
	}
	if !status.GetOk() {
		return nil, fmt.Errorf("got non OK status from server")
	}

	return &domain{
		conn:   conn,
		client: client,
	}, nil
}

func (d *domain) Close() {
	if d.stream != nil {
		_ = d.stream.CloseSend()
	}
	if d.closed != nil {
		<-d.closed
	}
	d.stream = nil
	d.closed = nil
	d.conn.Close()
}

func (d *domain) Listen(ctx context.Context, dest string) error {
	var err error
	d.stream, err = d.client.Listen(ctx, &pb.ListenRequest{Destination: dest})
	if err != nil {
		return fmt.Errorf("failed to listen for domain events: %v", err)
	}

	d.closed = make(chan struct{})
	go d.handler()
	return nil
}

func (d *domain) handler() {
	for {
		in, err := d.stream.Recv()
		if err == io.EOF {
			close(d.closed)
			return
		}
		if err != nil {
			log.Fatalf("Failed to receive a message: %v", err)
		}
		err = handleMessage(in)
		if err != nil {
			log.Printf("Error handling message: %s", err)
			close(d.closed)
			return
		}
	}
}

func handleMessage(message *pb.Message) error {
	body, err := message.Body.UnmarshalNew()
	if err != nil {
		return err
	}

	switch m := body.(type) {
	case *pb.ConfigureDomainRequest:
		log.Printf("Configuring domain: %s", m.Name)
	default:
		log.Printf("Unknown type: %s", reflect.TypeOf(m))
	}

	return nil
}
