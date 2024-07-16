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

/**
	This plugin is waiting for clients to create connections, and send through data to be
	sent to other Paladin nodes. When it gets data, it should extract the information and
	then make the connect and send the data.
**/

// This is all the information we should need to be able to find another node in the network
type GRPCRoutingInformation struct {
	Address string `json:"address"`
}

type GRPCTransportPlugin struct {
	interPaladinProto.UnimplementedInterPaladinTransportServer
	pluginInterfaceProto.UnimplementedPluginInterfaceServer
	
	socketName string
	port       int
}

func (gtp *GRPCTransportPlugin) InterPaladinMessageFlow(ctx context.Context, in *interPaladinProto.InterPaladinMessage) (*interPaladinProto.InterPaladinReceipt, error) {
	log.Printf("Got (external) message content %s", in.Content)
	return &interPaladinProto.InterPaladinReceipt{
		Content: "ACK",
	}, nil
}

func (gtp *GRPCTransportPlugin) PluginMessageFlow(ctx context.Context, in *pluginInterfaceProto.PaladinMessage) (*pluginInterfaceProto.PaladinMessageReceipt, error) {
	log.Printf("Got message content %s, forming inter paladin message", in.MessageContent)

	routingInfo := &GRPCRoutingInformation{}
	err := json.Unmarshal([]byte(in.RoutingInformation), routingInfo)
	if err != nil {
		log.Printf("Could not unmarshal routing information, err: %v", err)
		return nil, err
	}

	// TODO: mTLS for TCP connections
	// TODO: mTLS for domain sockets?

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

func (gtp *GRPCTransportPlugin) Initialise(ctx context.Context) {
	gtp.socketName = fmt.Sprintf("/tmp/%s.sock", uuid.New().String())
}

func (gtp *GRPCTransportPlugin) Start(ctx context.Context) {
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