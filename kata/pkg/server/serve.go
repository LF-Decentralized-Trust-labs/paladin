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
	"os"
	"strconv"

	"github.com/hyperledger/firefly-common/pkg/log"
	"google.golang.org/grpc"

	"github.com/kaleido-io/paladin/kata/internal/transaction"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

func newRPCServer(socketAddress string) (net.Listener, *grpc.Server, error) {
	ctx := log.WithLogField(context.Background(), "pid", strconv.Itoa(os.Getpid()))
	log.L(ctx).Infof("server starting at unix socket %s", socketAddress)
	l, err := net.Listen("unix", socketAddress)
	if err != nil {
		log.L(ctx).Error("failed to listen: ", err)
		return nil, nil, err
	}
	s := grpc.NewServer()

	proto.RegisterPaladinTransactionServiceServer(s, &transaction.PaladinTransactionService{})
	log.L(ctx).Infof("server listening at %v", l.Addr())
	return l, s, nil
}

func Run(socketAddress string) {
	l, s, err := newRPCServer(socketAddress)
	if err != nil {
		return
	}
	_ = s.Serve(l)
}
