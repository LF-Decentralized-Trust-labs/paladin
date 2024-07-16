// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugins

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"fmt"

	"github.com/google/uuid"
	interPaladinProto "github.com/kaleido-io/talaria/pkg/plugins/proto"
	pluginInterfaceProto "github.com/kaleido-io/talaria/pkg/talaria/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

/*
	This is the gRPC plugin that Talaria will talk to in order to send comms to other paladin
	nodes in the network. In theory, there's little stopping us connecting the gRPC transport
	layer at Talaria directly to another Talaria, but a dedicated plugin here proves that the
	plugin architecture not only works, but is in use.
	
	Here we implement 2 comms flows:

	1. gRPC to boundary Talaria layer using unix domain sockets
	2. gRPC over TLS to another gRPC residing on another Paladin node (potentially)
	
	It's important to note that it's possible for a transacting entity to need to send comms
	to another transacting entity that is actually on the same node, if that's the case then
	we still treat it as an outbound connection, but the actual call is to loopback.
*/

type GRPCRoutingInformation struct {
	// In theory this is an opaque object in the Registry that only this plugin knows how to
	// use and decode, for gRPC the amount of information we need is quite minimal though

	// TODO: mTLS credentials need to go here
	Address string `json:"address"`
}

type GRPCTransportPlugin struct {
	interPaladinProto.UnimplementedInterPaladinTransportServer
	pluginInterfaceProto.UnimplementedPluginInterfaceServer
	
	socketName string
	port       int
}

func (gtp *GRPCTransportPlugin) InterPaladinMessageFlow(ctx context.Context, in *interPaladinProto.InterPaladinMessage) (*interPaladinProto.InterPaladinReceipt, error) {
	// TODO: This is dumb, but also I don't know what a receipt should look like right now
	log.Printf("Got (external) message content %s", in.Content)
	return &interPaladinProto.InterPaladinReceipt{
		Content: "ACK",
	}, nil
}

func (gtp *GRPCTransportPlugin) PluginMessageFlow(ctx context.Context, in *pluginInterfaceProto.PaladinMessage) (*pluginInterfaceProto.PaladinMessageReceipt, error) {
	// TODO: Review of logging
	log.Printf("Got message content %s, forming inter paladin message", in.MessageContent)

	// TODO: What is routing information? Doesn't feel like it makes much sense
	routingInfo := &GRPCRoutingInformation{}
	err := json.Unmarshal([]byte(in.RoutingInformation), routingInfo)
	if err != nil {
		log.Printf("Could not unmarshal routing information, err: %v", err)
		return nil, err
	}

	// TODO: mTLS for TCP connections
	conn, err := grpc.NewClient(routingInfo.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to establish a client, err: %s", err)
	}
	defer conn.Close()

	client := interPaladinProto.NewInterPaladinTransportClient(conn)

	r, err := client.InterPaladinMessageFlow(ctx, &interPaladinProto.InterPaladinMessage{
		Content: in.MessageContent,
	})
	if err != nil {
		log.Fatalf("error sending message through gRPC: %v", err)
	}
	log.Printf("response was: %s", r.GetContent())

	return &pluginInterfaceProto.PaladinMessageReceipt{
		Content: "ACK",
	}, nil
}

// TODO: What is the different between initialise and start if one does essentially nothing?
func (gtp *GRPCTransportPlugin) Initialise(ctx context.Context) {
	gtp.socketName = fmt.Sprintf("/tmp/%s.sock", uuid.New().String())
}

func (gtp *GRPCTransportPlugin) Start(ctx context.Context) {
	// TODO: Review of threading model

	log.Printf("initialising connection to local socket %s\n", gtp.socketName)
	lis, err := net.Listen("unix", gtp.socketName)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	go func(){
		s := grpc.NewServer()
		pluginInterfaceProto.RegisterPluginInterfaceServer(s, &GRPCTransportPlugin{})
		log.Printf("server listening at %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	log.Printf("initialising connection for inbound gRPC connections %s\n", gtp.socketName)
	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", gtp.port))
	if err != nil {
		log.Fatalf("failed to listen for grpc connections: %v", err)
	}

	go func(){
		s := grpc.NewServer()
		interPaladinProto.RegisterInterPaladinTransportServer(s, &GRPCTransportPlugin{})
		log.Printf("grpc server listening at %v", grpcLis.Addr())
		if err := s.Serve(grpcLis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}

// TODO: Not this
//
// I am super not-sold on how plugin registration is being done here, I have lots of thoughts of
// why this is not what we want, but for now this is good enough to get this sample working. It
// sounds like there's going to be a tonne of precedent from other places in the code base for how
// plugins are going to fit in, so we're going to want to do something like that
func (gtp *GRPCTransportPlugin) GetRegistration() PluginRegistration {
	return PluginRegistration{
		Name: "grpc-transport-plugin",
		SocketLocation: gtp.socketName,
	}
}

func NewGRPCTransportPlugin(port int) *GRPCTransportPlugin {
	return &GRPCTransportPlugin{
		port: port,
	}
}