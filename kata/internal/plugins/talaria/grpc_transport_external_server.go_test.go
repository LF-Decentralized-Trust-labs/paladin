package grpctransport

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/proto/interpaladin"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"

	interPaladinPB "github.com/kaleido-io/paladin/kata/pkg/proto/interpaladin"
)

type fakePayload struct {
	Key   string
	Value string
}

var (
	testPort        = 10002
	testBufferSize  = 1
	loopbackAddress = fmt.Sprintf("localhost:%d", testPort)
	sendingAddress  = fmt.Sprintf("localhost:%d", testPort+1)
	fakeDesintation = "somewhereoverthemoon"
)

type fakeExternalGRPCServer struct {
	interPaladinPB.UnimplementedInterPaladinTransportServer

	listener net.Listener
}

func (fegs *fakeExternalGRPCServer) SendInterPaladinMessage(ctx context.Context, message *interPaladinPB.InterPaladinMessage) (*interPaladinPB.InterPaladinMessage, error) {
	fegs.listener.Close()
	return nil, nil
}

func TestOutboundMessageFlow(t *testing.T) {
	ctx := context.Background()
	server, err := NewExternalGRPCServer(ctx, testPort, testBufferSize)
	defer server.Shutdown()
	assert.NoError(t, err)

	// Start a server to recieve messages through
	testLis, err := net.Listen("tcp", fmt.Sprintf(":%d", testPort+1))
	assert.NoError(t, err)
	fakeServer := &fakeExternalGRPCServer{
		listener: testLis,
	}
	s := grpc.NewServer()
	defer s.GracefulStop()
	defer testLis.Close()
	interPaladinPB.RegisterInterPaladinTransportServer(s, fakeServer)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_ = s.Serve(testLis)
		wg.Done()
	}()

	// Create a fake message and queue it for sending
	fakeInternalMessage := &proto.Message{
		Id:          "some-uuid",
		Destination: fmt.Sprintf("localhost:%d", testPort+1),
	}

	fakeMessage := &ExternalMessage{
		Message:         *fakeInternalMessage,
		ExternalAddress: sendingAddress,
	}

	server.QueueMessageForSend(fakeMessage)
	wg.Wait()
}

func TestInboundMessageFlow(t *testing.T) {
	ctx := context.Background()
	server, err := NewExternalGRPCServer(ctx, testPort, testBufferSize)
	defer server.Shutdown()
	assert.NoError(t, err)

	conn, err := grpc.NewClient(loopbackAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer conn.Close()

	client := interpaladin.NewInterPaladinTransportClient(conn)

	fakeInternalMessage := &proto.Message{
		Id:          "some-uuid",
		Destination: fakeDesintation,
	}

	fakeMessage := &ExternalMessage{
		Message:         *fakeInternalMessage,
		ExternalAddress: loopbackAddress,
	}

	mPay, err := anypb.New(fakeMessage)
	assert.NoError(t, err)

	_, err = client.SendInterPaladinMessage(ctx, &interpaladin.InterPaladinMessage{
		Body: mPay,
	})
	assert.NoError(t, err)

	recvMessageFlow, err := server.GetMessages(destination(fakeDesintation))
	assert.NoError(t, err)

	msg := <-recvMessageFlow
	assert.NotNil(t, msg)
}

func TestInitializeExternalListener(t *testing.T) {
	ctx := context.Background()
	_, err := NewExternalGRPCServer(ctx, 10002, 1)
	assert.NoError(t, err)
}
