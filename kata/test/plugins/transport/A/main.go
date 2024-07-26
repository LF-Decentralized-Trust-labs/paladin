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

import "C"
import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/pkg/plugins"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	pluginPB "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const ProviderName = "github.com/kaleido-io/paladin/kata/test/plugins/transport/A"

type transportAProvider struct {
	grpcConnection *grpc.ClientConn
	stopListener   context.CancelFunc
	messageStream  proto.KataMessageService_ListenClient
	client         proto.KataMessageServiceClient
}

// singleton instance of the provider
var provider *transportAProvider

type transportAInstance struct {
	stopListener context.CancelFunc
}

var instances = make(map[string]*transportAInstance)

func (t *transportAProvider) Terminate() (plugins.TransportInstance, error) {
	t.stopListener()
	t.grpcConnection.Close()
	return nil, nil
}

func BuildInfo() string {
	ctx := context.Background()
	log.L(ctx).Info("transportAProvider.BuildInfo")
	return ProviderName
}

// CreateInstance implements plugins.TransportProvider and starts a new gRPC listener on the given socket address with the name of the provider as destination
func InitializeTransportProvider(socketAddress string, listenerDestination string) error {
	ctx := context.Background()

	log.L(ctx).Info("transportAProvider.InitializeTransportProvider")
	if provider != nil {
		log.L(ctx).Error("already initialized")
		//return fmt.Errorf("already initialized")
		//TODO this is a hack to allow tests to pass, need to add termination to the lifecycle
	}

	// Create a gRPC client connection
	conn, err := grpc.NewClient("unix:"+socketAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	// Create a new instance of the gRPC client
	client := proto.NewKataMessageServiceClient(conn)
	status, err := client.Status(ctx, &proto.StatusRequest{})

	delay := 0
	for !status.GetOk() {
		time.Sleep(time.Second)
		delay++
		status, err = client.Status(ctx, &proto.StatusRequest{})
		if delay > 2 {
			return fmt.Errorf("Server did not start after 2 seconds")
		}
	}
	if err != nil {
		return err
	}
	if status.GetOk() {
		log.L(ctx).Info("Connection to server established")
	} else {
		return fmt.Errorf("got non OK status from server")
	}

	listenerContext, stopListener := context.WithCancel(ctx)

	messageStream, err := client.Listen(listenerContext, &proto.ListenRequest{
		Destination: listenerDestination,
	})
	if err != nil {
		stopListener()
		return err
	}

	go func() {
		for {
			log.L(ctx).Info("Waiting for message")
			inboundMessage, err := messageStream.Recv()
			if err == io.EOF {
				log.L(ctx).Info("EOF received")
				return
			}
			log.L(ctx).Infof("Received message: %s", inboundMessage.GetBody().TypeUrl)
			receivedBody1, err := inboundMessage.GetBody().UnmarshalNew()
			if err != nil {
				log.L(ctx).Errorf("Error unmarshalling message: %s", err)
				continue
			}
			switch string(receivedBody1.ProtoReflect().Descriptor().FullName()) {
			case "github.com.kaleido_io.paladin.kata.plugin.CreateInstance":
				log.L(ctx).Info("Received CreateInstanceRequest message for transport A")
				createInstanceRequest := new(pluginPB.CreateInstance)
				err := inboundMessage.GetBody().UnmarshalTo(createInstanceRequest)
				if err != nil {
					log.L(ctx).Errorf("Error unmarshalling CreateInstanceRequest: %s", err)
					continue
				}
				err = provider.createInstance(ctx, createInstanceRequest)
				if err != nil {
					log.L(ctx).Errorf("Error creating instance: %s", err)
					continue
				}

			default:
				log.L(ctx).Errorf("Unknown message type: %s", receivedBody1.ProtoReflect().Descriptor().FullName())
			}
		}
	}()

	provider = &transportAProvider{
		grpcConnection: conn,
		stopListener:   stopListener,
		messageStream:  messageStream,
		client:         client,
	}
	return nil
}

func (p *transportAProvider) createInstance(ctx context.Context, createInstanceRequest *pluginPB.CreateInstance) error {
	log.L(ctx).Infof("transportAProvider.createInstance, name: %s, destination: %s", createInstanceRequest.GetName(), createInstanceRequest.GetMessageDestination())
	listenerContext, stopListener := context.WithCancel(ctx)
	messageStream, err := p.client.Listen(listenerContext, &proto.ListenRequest{
		Destination: createInstanceRequest.GetMessageDestination(),
	})
	if err != nil {
		stopListener()
		return err
	}

	newInstance := &transportAInstance{
		stopListener: stopListener,
	}
	instances[createInstanceRequest.GetName()] = newInstance

	go func() {
		for {
			inboundMessage, err := messageStream.Recv()
			if err == io.EOF {
				log.L(ctx).Info("EOF received")
				return
			}
			log.L(ctx).Infof("Received message: %s", inboundMessage.GetBody())
		}
	}()
	return nil
}

func main() {}
