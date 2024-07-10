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
package server

import (
	"context"
	"net"

	"github.com/hyperledger/firefly-common/pkg/log"
	"google.golang.org/grpc"

	"github.com/kaleido-io/paladin/internal/domain"
	pb "github.com/kaleido-io/paladin/internal/protos/domain"
)

// server is used to implement example.GreeterServer.
type server struct {
	pb.UnimplementedPaladinServiceServer
}

func (s *server) GetStates(ctx context.Context, in *pb.GetStatesRequest) (*pb.GetStatesReply, error) {

	return &pb.GetStatesReply{StateId: []string{"stateA", "stateB"}}, nil
}

func (s *server) RegisterDomain(stream pb.PaladinService_RegisterDomainServer) error {

	ctx := stream.Context()
	newDomain := domain.NewDomain(stream)
	log.L(ctx).Info("RegisteredDomain")

	err := newDomain.Listen()
	if err != nil {
		log.L(ctx).Error("Error listening", err)
		return err
	}
	//if we exit from this function, the stream will be closed
	log.L(ctx).Info("ClosingDomain")
	return nil
}

func Run() {
	ctx := context.Background()
	lis, err := net.Listen("tcp", ":50051")
	//lis, err := net.Listen("unix", "/tmp/grpc.sock")

	if err != nil {
		log.L(ctx).Error("failed to listen: ", err)
	}
	s := grpc.NewServer()
	pb.RegisterPaladinServiceServer(s, &server{})
	log.L(ctx).Infof("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.L(ctx).Error("failed to serve: ", err)
	}
}
