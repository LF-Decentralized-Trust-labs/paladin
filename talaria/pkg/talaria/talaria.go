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
// TODO: Review Todo's

type TransportProvider interface {
	SendMessage(context.Context, string, string) error
}

type Talaria struct {
	// This should be some external call out to the registry to get
	// information on the peer, but for now we're going to fake it
	registryProvider RegistryProvider
	plugins          []plugins.TransportPlugin
	pluginLocations  map[string]string
}

// TODO: Terrible hack because no config for plugins
func NewTalaria(rp RegistryProvider, port int) *Talaria {
	transportPlugins := []plugins.TransportPlugin{}

	grpcPlugin := plugins.NewGRPCTransportPlugin(port)
	transportPlugins = append(transportPlugins, grpcPlugin)

	return &Talaria{
		registryProvider: rp,
		plugins: transportPlugins,
		pluginLocations: make(map[string]string),
	}
}

func (t *Talaria) InitialisePlugins(ctx context.Context) {
	// This is the code for spinning off threads for plugins
	
	// TODO: Need to figure out what the actual framework for putting in plugins looks like here
	for _, plugin := range t.plugins {
		go func() {
			plugin.Initialise(ctx)
			reg := plugin.GetRegistration()
			t.pluginLocations[reg.Name] = reg.SocketLocation
			plugin.Start(ctx)
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

	// TODO: Need some code here to do plugin determination, for now gRPC is the only thing supported
	socketLocation := t.pluginLocations["grpc-transport-plugin"]

	// Send the message to the local socket
	socketLocationFormatted := fmt.Sprintf("unix://%s", socketLocation)
	log.Printf("sending message to socket %s\n", socketLocationFormatted)
	conn, err := grpc.NewClient(socketLocationFormatted, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to establish a client, err: %s", err)
	}
	defer conn.Close()

	client := pluginInterfaceProto.NewPluginInterfaceClient(conn)

	r, err := client.PluginMessageFlow(ctx, &pluginInterfaceProto.PaladinMessage{
		RoutingInformation: transTarget.RoutingInformation,
		MessageContent: content,
	})
	if err != nil {
		log.Fatalf("error sending message in the transport engine: %s", err.Error())
	}

	log.Printf("response was: %s", r.GetContent())
	return nil
}