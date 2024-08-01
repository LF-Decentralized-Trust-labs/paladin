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

package grpctransport

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	pluginPB "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
)

const ProviderName = "github.com/kaleido-io/paladin/kata/internal/plugins/talaria/grpctransport"

type destination string

type grpcTransportInstance struct {
	stopListener func()
}

type grpcTransportProvider struct {
	stopListener func()
	instances    map[destination]*grpcTransportInstance
	client       proto.KataMessageServiceClient

	// TODO: Things that should be config for the plugin
	externalListenPort                  int
	commsBusConnectionRetryLimit        int
	commsBusConnectionRetryAttemptDelay int
}

// Singleton instance of the GRPC Plugin
var provider *grpcTransportProvider
var externalServer ExternalServer

func Shutdown() {
	if provider == nil {
		return
	}

	provider.stopListener()
	provider = nil

	if externalServer == nil {
		return
	}

	externalServer.Shutdown()
	externalServer = nil
}

func BuildInfo() string {
	// TODO: When we know more about build information change this out for something else
	ctx := context.Background()
	log.L(ctx).Infof("grpctransport.BuildInfo: %s", ProviderName)
	return ProviderName
}

func InitializeTransportProvider(socketAddress string, listenerDestination string) error {
	ctx := context.Background()
	log.L(ctx).Info("grpctransport.InitializeTransportProvider")

	if provider != nil {
		return fmt.Errorf("gRPC transport provider already initialized")
	}

	// Create a gRPC client connection to the comms bus
	commsbusConn, err := grpc.NewClient(fmt.Sprintf("unix://%s", socketAddress), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	client := proto.NewKataMessageServiceClient(commsbusConn)

	var connErr error
	var status *proto.StatusResponse
	for retryCount := 0; retryCount < 2; retryCount++ {
		status, connErr = client.Status(ctx, &proto.StatusRequest{})
		if status.GetOk() {
			break
		}
		time.Sleep(time.Second)
	}
	if !status.GetOk() && connErr != nil {
		log.L(ctx).Errorf("grpctransport: Could not form connection to the comms bus after %d retries", 2)
		return connErr
	}

	listenerContext, stopListener := context.WithCancel(ctx)
	messageStream, err := client.Listen(listenerContext, &proto.ListenRequest{
		Destination: listenerDestination,
	})
	if err != nil {
		stopListener()
		return err
	}

	// Start listening for events on the plugin stream
	go func() {
		for {
			log.L(ctx).Info("grpctransport: Waiting for message")
			inboundMessage, err := messageStream.Recv()
			if err == io.EOF {
				log.L(ctx).Info("grpctransport: EOF received")
				return
			}
			log.L(ctx).Infof("grpctransport: Received message: %s", inboundMessage.GetBody().TypeUrl)
			receivedBody1, err := inboundMessage.GetBody().UnmarshalNew()
			if err != nil {
				log.L(ctx).Errorf("grpctransport: Error unmarshalling message: %s", err)
				continue
			}

			switch string(receivedBody1.ProtoReflect().Descriptor().FullName()) {
			case "github.com.kaleido_io.paladin.kata.plugin.CreateInstance":
				log.L(ctx).Info("grpctransport: Received CreateInstanceRequest message for transport A")
				createInstanceRequest := new(pluginPB.CreateInstance)
				err := inboundMessage.GetBody().UnmarshalTo(createInstanceRequest)
				if err != nil {
					log.L(ctx).Errorf("grpctransport: Error unmarshalling CreateInstanceRequest: %s", err)
					continue
				}
				err = provider.createInstance(ctx, createInstanceRequest)
				if err != nil {
					log.L(ctx).Errorf("grpctransport: Error creating instance: %s", err)
					continue
				}
			default:
				log.L(ctx).Errorf("grpctransport: Unknown message type: %s", receivedBody1.ProtoReflect().Descriptor().FullName())
			}
		}
	}()

	// Now bring up the external endpoint other Paladin's are going to speak to us on
	if externalServer == nil {
		// Mostly put behind an interface to make stubbing for UTs easy
		externalServer = NewExternalGRPCServer(ctx, provider.externalListenPort, 10)
	}

	provider = &grpcTransportProvider{
		stopListener:                        stopListener,
		instances:                           make(map[destination]*grpcTransportInstance),
		client:                              client,
		externalListenPort:                  8080,
		commsBusConnectionRetryLimit:        2,
		commsBusConnectionRetryAttemptDelay: 1,
	}

	return nil
}

func (gtp *grpcTransportProvider) createInstance(ctx context.Context, createInstanceRequest *pluginPB.CreateInstance) error {
	log.L(ctx).Infof("grpctransport.createInstance: name: %s, destination: %s", createInstanceRequest.GetName(), createInstanceRequest.GetMessageDestination())

	listenerContext, stopListener := context.WithCancel(ctx)
	messageStream, err := gtp.client.Listen(listenerContext, &proto.ListenRequest{
		Destination: createInstanceRequest.GetMessageDestination(),
	})
	if err != nil {
		stopListener()
		return err
	}

	// Messages coming through the stream here need to be sent to another Paladin, messages coming
	// back from the server need to be fed to the comms bus

	newInstance := &grpcTransportInstance{
		stopListener: stopListener,
	}
	gtp.instances[destination(createInstanceRequest.GetName())] = newInstance

	go func() {
		for {
			inboundMessage, err := messageStream.Recv()
			if err == io.EOF {
				log.L(ctx).Info("grpctransport instance: EOF received")
				return
			}

			externalServer.QueueMessageForSend(inboundMessage)
		}
	}()

	instanceMessageRecvChannel, err := externalServer.GetMessages(destination(createInstanceRequest.GetMessageDestination()))
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-instanceMessageRecvChannel:
				{
					// Need to send the message back to the comms bus so it can then be sent back to the actual destination
					response, err := gtp.client.SendMessage(ctx, msg)
					if err != nil {
						log.L(ctx).Errorf("grpctransport instance: error sending message to comms bus %v", err)
						continue
					}
					if response.Result == proto.SEND_MESSAGE_RESULT_SEND_MESSAGE_FAIL {
						log.L(ctx).Errorf("grpctransport instance: failed response when sending message to comms bus %s", *response.Reason)
						continue
					}
				}
			}
		}
	}()

	return nil
}
