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

package talaria

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"

	"github.com/hyperledger/firefly-common/pkg/log"
	plugins "github.com/kaleido-io/talaria/pkg/plugins"
	pluginInterfaceProto "github.com/kaleido-io/talaria/pkg/talaria/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TODO: Talaria is a plugin that speaks to other plugins, all of the code in here at the moment for
// orchestration of those plugins should be removed and replaced by whatever framework we have in Kata

// TODO: There is a fundamental problem if the context is cancelled with the in-memory sending messages
// buffer that will be lost if kept in memory. This might be a concern of the higher-level components
// above Talaria

var PluginBufferSize = 10

type TransportProvider interface {
	Initialise(ctx context.Context)
	SendMessage(ctx context.Context, node string, content []byte) error
	GetMessages() <- chan []byte
}

type PluginID string
type PluginMessage struct {
	Payload            []byte
	RoutingInformation []byte
}

type Talaria struct {
	// This should be some external call out to the registry to get
	// information on the peer, but for now we're going to fake it
	registryProvider RegistryProvider
	plugins          []plugins.TransportPlugin
	pluginLocations  map[PluginID]string

	recvMessages     chan []byte
	sendingMessages  map[PluginID]chan PluginMessage
}

// TODO: Terrible hack because no config for plugins (this will not be Talaria's concern)
func NewTalaria(ctx context.Context, rp RegistryProvider, port int) *Talaria {
	transportPlugins := []plugins.TransportPlugin{}

	grpcPlugin := plugins.NewGRPCTransportPlugin(ctx, port, tls.Certificate{})
	transportPlugins = append(transportPlugins, grpcPlugin)

	return &Talaria{
		registryProvider: rp,
		plugins: transportPlugins,
		pluginLocations: make(map[PluginID]string),
		// TODO: Inbound messages here are buffered, but the read channel isn't which means
		// we're able to cope with whole bunch of inbound messages with some seperation to
		// the sending of those messages, but read is always one by one.
		recvMessages: make(chan []byte, len(transportPlugins) * 10),
		sendingMessages: make(map[PluginID]chan PluginMessage),
	}
}

func (t *Talaria) GetMessages() <- chan []byte {
	return t.recvMessages
}

func (t *Talaria) Initialise(ctx context.Context) {
	for _, plugin := range t.plugins {
		pluginCtx, cancel := context.WithCancel(ctx)

		reg := plugin.GetRegistration()
		t.pluginLocations[PluginID(reg.Name)] = reg.SocketLocation
		plugin.Start(pluginCtx)

		// For each of the plugins spin up the comms threads
		t.sendingMessages[PluginID(reg.Name)] = make(chan PluginMessage, PluginBufferSize)
		
		socketLocationFormatted := fmt.Sprintf("unix://%s", reg.SocketLocation)
		conn, err := grpc.NewClient(socketLocationFormatted, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.L(ctx).Errorf("Failed to establish connection to plugin: %s, err: %v", reg.Name, err)
			cancel()
			return
		}

		client := pluginInterfaceProto.NewPluginInterfaceClient(conn)
		stream, err := client.PluginMessageFlow(pluginCtx)
		if err != nil {
			cancel()
			return
		}

		go func(){
			<-ctx.Done()
			conn.Close()
			cancel()
		}()
			
		// Handle inbound messages back from the plugin
		go func(){
			for {
				select {
				case <- pluginCtx.Done():
					return
				default:
				}

				returnedMessage, err := stream.Recv() 
				if err == io.EOF {
					log.L(ctx).Debugf("Shutting down Talaria listener")
					return
				}
				if err != nil {
					log.L(ctx).Errorf("Talaria receive error %v", err)
					continue
				}

				t.recvMessages <- returnedMessage.Payload
			}
		}()

		go func(){
			for {
				select {
				case <- pluginCtx.Done():
					return
				case message := <-t.sendingMessages[PluginID(reg.Name)]: {
					req := &pluginInterfaceProto.PaladinMessage{
							Payload: message.Payload,
							RoutingInformation: message.RoutingInformation,
						}
						// TODO: When is this a blocking operation? What happens if a message cannot be sent?
						// TODO: What is our retry mechanism?
						if err := stream.Send(req); err != nil {
							log.L(ctx).Errorf("Talaria send error, can not send %v", err)
						}
				}
				}
			}
		}()
	}
}

// This is the client-facing API
func (t *Talaria) SendMessage(ctx context.Context, paladinNode string, content []byte) error {
	transpTarget, err := t.registryProvider.LookupPaladinEntity(paladinNode)
	if err != nil {
		log.L(ctx).Errorf("could not find entity from the DB, err: %v", err)
		return err
	}

	// TODO: Plugin determination
	t.sendingMessages["grpc-transport-plugin"] <- PluginMessage{
		Payload: content,
		RoutingInformation: []byte(transpTarget.RoutingInformation),
	}

	return nil
}