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

// TODO: Transport Engine should really be initialising the plugins and then doing IPC to those processes
// TODO: Need to find some way to plug in peering information
// TODO: What does the output of this section look like?

// talaria is repsonsible for:
//  - Registry lookups
//  - Handling calls to send messages
//  - Lifecycle of plugins
//  - Doing gRPC to the plugins

type TransportProvider interface {
	SendMessage(context.Context, string, string) error
}

type talaria struct {
	// This should be some external call out to the registry to get
	// information on the peer, but for now we're going to fake it
	registryProvider RegistryProvider
	plugins          []plugins.TransportPlugin
	pluginLocations  map[string]string
}

// TODO: Terrible hack because no config for plugins
func Newtalaria(rp RegistryProvider, port int) *talaria {
	transportPlugins := []plugins.TransportPlugin{}

	grpcPlugin := plugins.NewGRPCTransportPlugin(port)
	transportPlugins = append(transportPlugins, grpcPlugin)

	return &talaria{
		registryProvider: rp,
		plugins: transportPlugins,
		pluginLocations: make(map[string]string),
	}
}

func (t *talaria) InitialisePlugins(ctx context.Context) {
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
func (t *talaria) SendMessage(ctx context.Context, paladinNode, content string) error {
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