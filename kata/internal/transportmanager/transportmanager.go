package transportmanager

import (
	"context"
	"fmt"
	"io"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"

	transportmanagerpb "github.com/kaleido-io/paladin/kata/pkg/proto/transportmanager"
)

type TransportType string
type Component string

type ExternalTransporter interface {
	Send(ctx context.Context, msg *proto.Message) error
	Receive(ctx context.Context, destination string, newMessageHandler func(chan *proto.Message))
}

type transportManager struct {
	recvMessages            map[Component]chan *proto.Message
	regClient               RegistryClient
	knownTransportProviders map[string]*grpc.ClientConn
	sendMessages            map[TransportType]chan *transportmanagerpb.ExternalMessage
}

func NewTransportManager(registry RegistryClient) *transportManager {
	return &transportManager{
		recvMessages:            make(map[Component]chan *proto.Message),
		regClient:               registry,
		knownTransportProviders: make(map[string]*grpc.ClientConn),
		sendMessages:            make(map[TransportType]chan *transportmanagerpb.ExternalMessage),
	}
}

func (tm *transportManager) RegisterNewTransportProvider(ctx context.Context, socketLocation string, name string) error {
	log.L(ctx).Infof("transportmanager: registering new transport provider")

	// Create a new client to the plugin, and then initialise a transport connection
	pluginConn, err := grpc.NewClient(socketLocation, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.L(ctx).Errorf("transportmanager: could not open a connection to the plugin, err: %v", err)
		return err
	}
	pluginClient := transportmanagerpb.NewTransportManagerClient(pluginConn)
	status, err := pluginClient.Status(ctx, nil)
	if err != nil || status == nil {
		log.L(ctx).Errorf("transportmanager: could not form connection to the plugin, err: %v", err)
		return err
	}

	tm.sendMessages[TransportType(name)] = make(chan *transportmanagerpb.ExternalMessage)
	tm.knownTransportProviders[name] = pluginConn

	transportClient, err := pluginClient.Transport(ctx)
	if err != nil {
		log.L(ctx).Errorf("transportmanager: could not open a transport flow to the plugin, err: %v", err)
		return err
	}

	// TransportManager -> Plugin flow (instructions to send messages)
	go func() {
		for {
			select {
			case <-ctx.Done():
				err = transportClient.CloseSend()
				if err != nil {
					log.L(ctx).Warnf("transportmanager: could not gracefully close client connection to plugin: %v", err)
				}
				return
			case sendMsg := <-tm.sendMessages[TransportType(name)]:
				{
					err := transportClient.Send(sendMsg)
					if err != nil {
						log.L(ctx).Errorf("transportmanager: could not send message to the plugin, err: %v", err)
					}
				}
			}
		}
	}()

	// TransportManager <- Plugin Flow (messages coming from other Paladins)
	go func() {
		for {
			recvMessage, err := transportClient.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				log.L(ctx).Errorf("transportmanager: got error receiving from plugin: %v", err)
				return
			}

			// TODO: Won't need to do this once we've fixed the proto (import issue)
			msg := &proto.Message{}
			err = recvMessage.UnmarshalTo(msg)
			if err != nil {
				log.L(ctx).Errorf("transportmanager: error unmarshalling reply from plugin: %v", err)
			}

			tm.recvMessages[Component(msg.Destination)] <- msg
		}
	}()

	return nil
}

// Implements ExternalTransporter
func (tm *transportManager) Send(ctx context.Context, msg *proto.Message) error {
	// TODO: Error return updating
	// TODO: down the line hierarchy utility

	transportDetails := tm.regClient.ResolveIdentity(&ResolveIdentityRequest{
		Name: msg.Destination,
	})

	// Do plugin determination
	commonTransportProvider := ""
	transportTypes := tm.regClient.GetTransportInformation(transportDetails)
	for transportProvider, _ := range tm.knownTransportProviders {
		if transportTypes[transportProvider] != "" {
			commonTransportProvider = transportProvider
		}
	}
	if commonTransportProvider == "" {
		log.L(ctx).Errorf("transportmanager: no common transport methods with target, cannot send message!")
		return fmt.Errorf("no common transport methods with target, cannot send message")
	}

	// This is a string containing serialised JSON that we do not know the strcuture of, send it black-box to the
	// plugin and let them handle the unmarshalling there
	pluginSpecificTransportInformation := transportTypes[commonTransportProvider]

	// Package the message
	packagedBody, err := anypb.New(msg)
	if err != nil {
		return err
	}

	packagedMessage := &transportmanagerpb.ExternalMessage{
		Body:             packagedBody,
		TransportDetails: []byte(pluginSpecificTransportInformation),
	}

	// For now just queue the message up
	if tm.sendMessages[TransportType(commonTransportProvider)] == nil {
		tm.sendMessages[TransportType(commonTransportProvider)] = make(chan *transportmanagerpb.ExternalMessage, 1)
	}

	tm.sendMessages[TransportType(commonTransportProvider)] <- packagedMessage
	return nil
}

// Implements ExternalTransporter
func (tm *transportManager) Recieve(ctx context.Context, component string, newMessageHandler func(*proto.Message)) {
	if tm.recvMessages[Component(component)] == nil {
		tm.recvMessages[Component(component)] = make(chan *proto.Message)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-tm.recvMessages[Component(component)]:
				{
					newMessageHandler(message)
				}
			}
		}
	}()
}
