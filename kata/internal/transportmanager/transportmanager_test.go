package transportmanager

import (
	"context"
	"sync"

	"fmt"
	"net"
	"os"
	"path"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kaleido-io/paladin/kata/pkg/proto"
	transportmanagerpb "github.com/kaleido-io/paladin/kata/pkg/proto/transportmanager"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type fakeTransportProvider struct {
	transportmanagerpb.UnimplementedTransportManagerServer

	sendMessages chan *anypb.Any
	recvMessages chan *transportmanagerpb.ExternalMessage
}

func (ftp *fakeTransportProvider) Status(ctx context.Context, in *emptypb.Empty) (*transportmanagerpb.PluginStatus, error) {
	return &transportmanagerpb.PluginStatus{
		Ok: true,
	}, nil
}
func (ftp *fakeTransportProvider) Transport(stream transportmanagerpb.TransportManager_TransportServer) error {
	ctx := stream.Context()

	go func() {
		for {
			recvMessage, err := stream.Recv()
			if err != nil {
				return
			}

			ftp.recvMessages <- recvMessage
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-ftp.sendMessages:
			{
				err := stream.Send(msg)
				if err != nil {
					return err
				}
			}
		}
	}
}

type fakeRegistryClient struct {
	knownIdentity    *ResolveIdentityResponse
	transportDetails map[string]string
}

func (frc *fakeRegistryClient) ResolveIdentity(identity *ResolveIdentityRequest) *ResolveIdentityResponse {
	return frc.knownIdentity
}
func (frc *fakeRegistryClient) GetTransportInformation(identity *ResolveIdentityResponse) map[string]string {
	return frc.transportDetails
}

func TestRegisterNewTransportProviderMessageSendFlow(t *testing.T) {
	// Simulate a message coming back from the plugin
	ctx := context.Background()

	transportType := "carrier_pigeon"
	fakeIdentity := &ResolveIdentityResponse{
		Name: "fakeidentity",
	}
	fakeTransportDetails := map[string]string{
		transportType: "<some serialised json>",
	}

	tm := NewTransportManager(&fakeRegistryClient{
		knownIdentity:    fakeIdentity,
		transportDetails: fakeTransportDetails,
	})
	tm.knownTransportProviders[transportType] = nil

	testingSocketLocation := fmt.Sprintf("%s.sock", uuid.NewString())
	fakeProvider := &fakeTransportProvider{
		sendMessages: make(chan *anypb.Any),
		recvMessages: make(chan *transportmanagerpb.ExternalMessage, 1),
	}
	testDir := os.TempDir()
	testingSocket := path.Join(testDir, testingSocketLocation)
	defer os.Remove(testingSocket)

	pluginListener, err := net.Listen("unix", testingSocket)
	assert.NoError(t, err)
	s := grpc.NewServer()

	transportmanagerpb.RegisterTransportManagerServer(s, fakeProvider)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.Serve(pluginListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	err = tm.RegisterNewTransportProvider(ctx, fmt.Sprintf("unix://%s", testingSocket), "carrier_pigeon")
	assert.NoError(t, err)

	err = tm.Send(ctx, &proto.Message{Destination: "fakeidentity"})
	assert.NoError(t, err)

	<-fakeProvider.recvMessages
}

func TestRegisterNewTransportProviderMessageReturnFlow(t *testing.T) {
	// Simulate a message coming back from the plugin
	ctx, cancel := context.WithCancel(context.Background())
	tm := NewTransportManager(nil)
	testingSocketLocation := fmt.Sprintf("%s.sock", uuid.NewString())

	fakeProvider := &fakeTransportProvider{
		sendMessages: make(chan *anypb.Any),
	}
	testDir := os.TempDir()
	testingSocket := path.Join(testDir, testingSocketLocation)
	defer os.Remove(testingSocket)

	pluginListener, err := net.Listen("unix", testingSocket)
	assert.NoError(t, err)
	s := grpc.NewServer()

	transportmanagerpb.RegisterTransportManagerServer(s, fakeProvider)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.Serve(pluginListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	err = tm.RegisterNewTransportProvider(ctx, fmt.Sprintf("unix://%s", testingSocket), "magicaltransportprovider")
	assert.NoError(t, err)

	// Send a message through our side of the transport link to the transport manager and check that it comes through on the correct channel internally
	fakeMessage := &proto.Message{
		Destination: "somemagicalcomponent",
		Id:          uuid.NewString(),
	}
	fakeMessageAny, err := anypb.New(fakeMessage)
	assert.NoError(t, err)

	fakeProvider.sendMessages <- fakeMessageAny

	var recvMessage *proto.Message
	tm.Recieve(ctx, "somemagicalcomponent", func(m *proto.Message) {
		recvMessage = m
	})

	var msgWg sync.WaitGroup
	msgWg.Add(1)
	go func() {
		for {
			if recvMessage != nil {
				msgWg.Done()
				cancel()
				return
			}

			time.Sleep(100 * time.Millisecond)
		}
	}()
	msgWg.Wait()

	assert.Equal(t, "somemagicalcomponent", recvMessage.Destination)
}

func TestRegisterNewTransportProviderInitialisationFlow(t *testing.T) {
	ctx := context.Background()
	tm := NewTransportManager(nil)

	err := tm.RegisterNewTransportProvider(ctx, "somesocketthatdoesnotexist", "magicaltransportprovider")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "zero addresses")

	testingSocketLocation := fmt.Sprintf("%s.sock", uuid.NewString())

	fakeProvider := &fakeTransportProvider{}
	testDir := os.TempDir()
	testingSocket := path.Join(testDir, testingSocketLocation)
	defer os.Remove(testingSocket)

	pluginListener, err := net.Listen("unix", testingSocket)
	assert.NoError(t, err)
	s := grpc.NewServer()

	transportmanagerpb.RegisterTransportManagerServer(s, fakeProvider)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.Serve(pluginListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	err = tm.RegisterNewTransportProvider(ctx, fmt.Sprintf("unix://%s", testingSocket), "magicaltransportprovider")
	assert.NoError(t, err)
}

func TestSendMessagesFailWhenNoCommonPluginsWithRemote(t *testing.T) {
	ctx := context.Background()
	transportTypeISupport := "carrier_pigeon"
	transportTypeTheySupport := "signal_fires"

	fakeIdentity := &ResolveIdentityResponse{
		Name: "fakeidentity",
	}
	fakeTransportDetails := map[string]string{
		transportTypeTheySupport: "<some serialised json>",
	}

	tm := NewTransportManager(&fakeRegistryClient{
		knownIdentity:    fakeIdentity,
		transportDetails: fakeTransportDetails,
	})
	tm.knownTransportProviders[transportTypeISupport] = nil

	err := tm.Send(ctx, &proto.Message{Destination: "fakeidentity"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no common transport methods ")
}

func TestSendMessagesProviderAvailable(t *testing.T) {
	ctx := context.Background()
	fakeTransportType := "carrier_pigeon"

	fakeIdentity := &ResolveIdentityResponse{
		Name: "fakeidentity",
	}
	fakeTransportDetails := map[string]string{
		fakeTransportType: "<some serialised json>",
	}

	tm := NewTransportManager(&fakeRegistryClient{
		knownIdentity:    fakeIdentity,
		transportDetails: fakeTransportDetails,
	})
	tm.knownTransportProviders[fakeTransportType] = nil

	err := tm.Send(ctx, &proto.Message{Destination: "fakeidentity"})
	assert.NoError(t, err)

	<-tm.sendMessages[TransportType(fakeTransportType)]
}

func TestRecieveMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tm := NewTransportManager(nil)
	fakeComponentName := "fakecomponent"

	fakeMessage := &proto.Message{}
	messageCount := 0

	tm.Recieve(ctx, fakeComponentName, func(msg *proto.Message) {
		messageCount++
	})

	// Send 4 messages
	tm.recvMessages[Component(fakeComponentName)] <- fakeMessage
	tm.recvMessages[Component(fakeComponentName)] <- fakeMessage
	tm.recvMessages[Component(fakeComponentName)] <- fakeMessage
	tm.recvMessages[Component(fakeComponentName)] <- fakeMessage

	// Wait for all the messages to be sent through
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			if messageCount == 4 {
				wg.Done()
				cancel()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	wg.Wait()

	assert.Equal(t, 4, messageCount)
}
