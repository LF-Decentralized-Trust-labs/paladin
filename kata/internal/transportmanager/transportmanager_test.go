package transportmanager

import (
	"context"
	"sync"
	"time"

	// "fmt"
	// "net"
	// "os"
	// "path"
	"testing"

	// "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	// "google.golang.org/grpc"
	// "google.golang.org/protobuf/types/known/anypb"

	"github.com/kaleido-io/paladin/kata/pkg/proto"
	// grpctransportpb "github.com/kaleido-io/paladin/kata/pkg/proto/grpctransport"
	transportmanagerpb "github.com/kaleido-io/paladin/kata/pkg/proto/transportmanager"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type fakeTransportProvider struct {
	transportmanagerpb.UnimplementedTransportManagerServer
}

func (ftp *fakeTransportProvider) Status(ctx context.Context, in *emptypb.Empty) (*transportmanagerpb.PluginStatus, error) {
	return &transportmanagerpb.PluginStatus{
		Ok: true,
	}, nil
}
func (ftp *fakeTransportProvider) Transport(transportmanagerpb.TransportManager_TransportServer) {
	return
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

	_ = <-tm.sendMessages[TransportType(fakeTransportType)]
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
