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
	"io"
	"log"
	"time"

	pb "github.com/kaleido-io/paladin/kata/pkg/proto"

	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/domains/noto/internal/noto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	dest        = "to-domain"
	testbedAddr = "http://127.0.0.1:49603"
	grpcAddr    = "unix:/tmp/testbed.paladin.1542386773.sock"
)

func connectGRPC(ctx context.Context) (*grpc.ClientConn, pb.KataMessageService_ListenClient, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(grpcAddr, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect gRPC: %v", err)
	}

	client := pb.NewKataMessageServiceClient(conn)
	status, err := client.Status(ctx, &pb.StatusRequest{})

	delay := 0
	for !status.GetOk() {
		time.Sleep(time.Second)
		delay++
		status, err = client.Status(ctx, &pb.StatusRequest{})
		if delay > 2 {
			return nil, nil, fmt.Errorf("server was not ready after 2 seconds")
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if !status.GetOk() {
		return nil, nil, fmt.Errorf("got non OK status from server")
	}

	stream, err := client.Listen(ctx, &pb.ListenRequest{Destination: dest})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen for domain events: %v", err)
	}
	return conn, stream, nil
}

func listenGRPC(stream pb.KataMessageService_ListenClient, closed chan struct{}) {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			close(closed)
			return
		}
		if err != nil {
			log.Fatalf("Failed to receive a message: %v", err)
		}
		err = noto.HandleDomainMessage(in)
		if err != nil {
			log.Printf("Error handling message: %s", err)
			close(closed)
			return
		}
	}
}

func runTest(ctx context.Context) error {
	conn, stream, err := connectGRPC(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("Listening for gRPC messages on %s", dest)
	closed := make(chan struct{})
	go listenGRPC(stream, closed)

	conf := ffresty.Config{URL: testbedAddr}
	rest := ffresty.NewWithConfig(ctx, conf)
	rpc := rpcbackend.NewRPCClient(rest)

	log.Printf("Calling testbed_configureInit")
	var result map[string]interface{}
	rpcerr := rpc.CallRPC(ctx, &result, "testbed_configureInit", "noto", `{}`)
	if rpcerr != nil {
		return fmt.Errorf("fail to call JSON RPC: %v", rpcerr)
	}

	log.Printf("Closing stream")
	_ = stream.CloseSend()
	log.Printf("Awaiting close")
	<-closed
	return nil
}

func main() {
	ctx := context.Background()
	err := runTest(ctx)
	if err != nil {
		log.Fatalf("%s", err)
	}
}
