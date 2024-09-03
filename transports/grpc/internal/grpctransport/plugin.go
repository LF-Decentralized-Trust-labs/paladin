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
	"io"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type GRPCTransport struct {
	prototk.UnimplementedPluginControllerServer
	externalServer   *externalGRPCServer
	controllerClient grpc.BidiStreamingClient[prototk.TransportMessage, prototk.TransportMessage]

	pluginID   string
	connString string

	sendMessages chan *ExternalMessage

	ServerWg     sync.WaitGroup // TODO: This should not be exported (done to make the init call work)
	serverConn   *grpc.ClientConn
	serverCtx    context.Context
	serverCancel context.CancelFunc

	Config      *GRPCConfig
	Initialized bool
}

func NewGRPCTransport(pluginID, connString string) (*GRPCTransport, error) {
	return &GRPCTransport{
		pluginID:     pluginID,
		connString:   connString,
		sendMessages: make(chan *ExternalMessage, 1),
	}, nil
}

func (gpt *GRPCTransport) Init() error {
	gpt.serverCtx, gpt.serverCancel = context.WithCancel(context.Background())
	log.L(gpt.serverCtx).Info("grpctransport.Init")

	// Create a gRPC Connection back to the plugin controller
	pluginControllerConn, err := grpc.NewClient(gpt.connString, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	gpt.serverConn = pluginControllerConn

	client := prototk.NewPluginControllerClient(pluginControllerConn)

	pluginControllerClient, err := client.ConnectTransport(gpt.serverCtx)
	if err != nil {
		return err
	}
	gpt.controllerClient = pluginControllerClient
	pcCtx := pluginControllerClient.Context()

	// Start listening for events on the plugin stream
	gpt.ServerWg.Add(1)
	go func() {
		for {
			inboundMessage, err := pluginControllerClient.Recv()
			if err == io.EOF {
				log.L(pcCtx).Info("grpctransport: EOF received")
				gpt.ServerWg.Done()
				return
			}
			if err != nil {
				continue
			}

			switch input := inboundMessage.RequestToTransport.(type) {
			case *prototk.TransportMessage_ConfigureTransport:
				config := input.ConfigureTransport.ConfigYaml
				upc := &UnprocessedGRPCConfig{}
				err := yaml.Unmarshal([]byte(config), upc)
				if err != nil {
					log.L(pcCtx).Errorf("grpctransport: Could not unmarshal config %v", err)
					break
				}

				processedConfig, err := ProcessGRPCConfig(upc)
				if err != nil {
					log.L(pcCtx).Errorf("grpctransport: Could not process config %v", err)
					break
				}

				gpt.Config = processedConfig

				pluginControllerClient.Send(&prototk.TransportMessage{
					ResponseFromTransport: &prototk.TransportMessage_ConfigureTransportRes{
						ConfigureTransportRes: &prototk.ConfigureTransportResponse{
							TransportConfig: &prototk.TransportConfig{}, // TODO: What is the shape of the response here?
						},
					},
				})
			case *prototk.TransportMessage_InitTransport:
				// Now bring up the external endpoint other Paladin's are going to speak to us on
				if gpt.externalServer == nil {
					gpt.externalServer, err = NewExternalGRPCServer(gpt.serverCtx, gpt.Config.ExternalPort, gpt.Config.ServerCertificate, gpt.Config.ClientCertificate)
					if err != nil {
						log.L(pcCtx).Errorf("grpctransport: Could not start external server %v", err)
						break
					}
				}

				gpt.InitializeExternalListener()

				pluginControllerClient.Send(&prototk.TransportMessage{
					ResponseFromTransport: &prototk.TransportMessage_InitTransportRes{
						InitTransportRes: &prototk.InitTransportResponse{},
					},
				})
			case *prototk.TransportMessage_SendMessage:
				gpt.sendMessages <- &ExternalMessage{
					Body:             input.SendMessage.Body,
					TransportDetails: input.SendMessage.TransportDetails,
				}

				pluginControllerClient.Send(&prototk.TransportMessage{
					ResponseFromTransport: &prototk.TransportMessage_SendMessageRes{
						SendMessageRes: &prototk.SendMessageResponse{},
					},
				})
			}
		}
	}()

	go func() {
		for {
			select {
			case <-pcCtx.Done():
				return
			case msg := <-gpt.sendMessages:
				err := gpt.externalServer.QueueMessageForSend(msg.Body, msg.TransportDetails)
				if err != nil {
					log.L(pcCtx).Errorf("grpctransport: Could not send message to external server for send")
				}
			}
		}
	}()

	return nil
}

func (gpt *GRPCTransport) InitializeExternalListener() {
	gpt.ServerWg.Add(1)
	go func() {
		for {
			select {
			case <-gpt.serverCtx.Done():
				gpt.ServerWg.Done()
				return
			case msg := <-gpt.externalServer.recvMessages:
				serializedTransportMessage, err := yaml.Marshal(msg)
				if err != nil {
					continue
				}

				err = gpt.controllerClient.Send(&prototk.TransportMessage{
					RequestFromTransport: &prototk.TransportMessage_Recieve{
						Recieve: &prototk.ReceiveMessageRequest{
							Body: string(serializedTransportMessage),
						},
					},
				})
				if err != nil {
					log.L(gpt.serverCtx).Error("grpctransport: Could not call transport manager to return message")
				}
			}
		}
	}()
	gpt.Initialized = true
}

func (gpt *GRPCTransport) Shutdown() error {
	if gpt.Initialized {
		err := gpt.serverConn.Close()
		if err != nil {
			return err
		}
	}

	gpt.serverCancel()

	// Closing the connection should stop the goroutine to the plugin
	// and therefore release anyone waiting on the wait group

	return nil
}
