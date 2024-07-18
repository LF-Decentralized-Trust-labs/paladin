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
	"log"
	"fmt"
	"io"
	
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pluginInterfaceProto "github.com/kaleido-io/talaria/pkg/talaria/proto"
	plugins "github.com/kaleido-io/talaria/pkg/plugins"
)

/*
	Talaria feels very amorphous at the moment but I think from code that it has the following reponsibilties
	right now:

	1. Handling inbound requests to send messages to other Paladin transacting entities (PTE)
	2. Performing lookups with whatever our registry looks like to figure out how to talk to other PTE's
	3. Doing lifecycle of transport plugins, making sure they're initialised and handling connections
	4. Doing comms to the plugins
*/

// TODO: Talaria should really be initialising the plugins and then doing IPC to those processes
// TODO: Need to find some way to plug in peering information
// TODO: What does the output of this section look like?
// TODO: How are we going to manage config for plugins (feels like not just a me-problem)
// TODO: What happens if we exceed buffer sizes? Do we block?
// TODO: Review Todo's

type TransportProvider interface {
	Initialise(ctx context.Context)
	SendMessage(ctx context.Context, node string, content string) error
	GetMessages() <- chan string
}

type PluginID string
type PluginMessage struct {
	MessageContent     string
	RoutingInformation string
}

type Talaria struct {
	// This should be some external call out to the registry to get
	// information on the peer, but for now we're going to fake it
	registryProvider RegistryProvider
	plugins          []plugins.TransportPlugin
	pluginLocations  map[PluginID]string

	recvMessages     chan string
	sendingMessages  map[PluginID]chan PluginMessage
}

// TODO: Terrible hack because no config for plugins
func NewTalaria(rp RegistryProvider, port int) *Talaria {
	transportPlugins := []plugins.TransportPlugin{}

	grpcPlugin := plugins.NewGRPCTransportPlugin(port)
	transportPlugins = append(transportPlugins, grpcPlugin)

	return &Talaria{
		registryProvider: rp,
		plugins: transportPlugins,
		pluginLocations: make(map[PluginID]string),
		recvMessages: make(chan string, len(transportPlugins) * 10),
		sendingMessages: make(map[PluginID]chan PluginMessage),
	}
}

func (t *Talaria) GetMessages() <- chan string {
	return t.recvMessages
}

func (t *Talaria) Initialise(ctx context.Context) {
	// TODO: Need to figure out what the actual framework for putting in plugins looks like here
	for _, plugin := range t.plugins {
		reg := plugin.GetRegistration()
		t.pluginLocations[PluginID(reg.Name)] = reg.SocketLocation
		plugin.Start(ctx)

		// For each of the plugins spin up the comms threads
		t.sendingMessages[PluginID(reg.Name)] = make(chan PluginMessage, 10)
		
		socketLocationFormatted := fmt.Sprintf("unix://%s", reg.SocketLocation)
		conn, err := grpc.NewClient(socketLocationFormatted, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to establish connection to plugin: %s, err: %v", reg.Name, err)
		}

		client := pluginInterfaceProto.NewPluginInterfaceClient(conn)
		stream, err := client.PluginMessageFlow(context.Background())
		if err != nil {
			return
		}

		// Handle inbound messages back from the plugin
		go func(){
			for {
				select {
					case <- ctx.Done():
						return
					default: {
						returnedMessage, err := stream.Recv()
						if err == io.EOF {
							log.Println("shutdown")
							conn.Close()
							return
						}
						if err != nil {
							log.Printf("receive error %v", err)
							continue
						}

						t.recvMessages <- returnedMessage.MessageContent
					}
				}
			}
		}()

		go func(){
			for {
				select {
				case <- ctx.Done():
					return
				case message := <-t.sendingMessages[PluginID(reg.Name)]: {
					req := &pluginInterfaceProto.PaladinMessage{
							MessageContent: message.MessageContent,
							RoutingInformation: message.RoutingInformation,
						}
						if err := stream.Send(req); err != nil {
							log.Fatalf("can not send %v", err)
						}
				}
				}
			}
		}()
	}
}

// This is the client-facing API
func (t *Talaria) SendMessage(ctx context.Context, paladinNode, content string) error {
	transTarget, err := t.registryProvider.LookupPaladinEntity(paladinNode)
	if err != nil {
		log.Printf("could not find entity from the DB, err: %v", err)
		return err
	}

	// TODO: Plugin determination
	t.sendingMessages["grpc-transport-plugin"] <- PluginMessage{
		MessageContent: content,
		RoutingInformation: transTarget.RoutingInformation,
	}

	return nil
}