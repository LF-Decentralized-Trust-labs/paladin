package grpctransport

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	interPaladinPB "github.com/kaleido-io/paladin/kata/pkg/proto/interpaladin"
)

type ExternalServer interface {
	QueueMessageForSend(msg *proto.Message)
	GetMessages(dest destination) (chan *proto.Message, error)
	Shutdown()
}

type externalGRPCServer struct {
	interPaladinPB.UnimplementedInterPaladinTransportServer

	grpcListener net.Listener

	// TODO: We probably don't want to do this, what happens when we're not consuming messages correctly?
	recvMessages map[destination]chan *proto.Message
	sendMessages chan *proto.Message
	port         int
}

func NewExternalGRPCServer(ctx context.Context, port int, bufferSize int) *externalGRPCServer {
	server := &externalGRPCServer{
		recvMessages: make(map[destination]chan *proto.Message, bufferSize),
		sendMessages: make(chan *proto.Message, bufferSize),
		port:         port,
	}

	server.initializeExternalListener(ctx)

	return server
}

func (egs *externalGRPCServer) QueueMessageForSend(msg *proto.Message) {
	egs.sendMessages <- msg
}

func (egs *externalGRPCServer) GetMessages(dest destination) (chan *proto.Message, error) {
	if egs.recvMessages[dest] == nil {
		return nil, fmt.Errorf("could not find entries for the provided destination")
	}

	return egs.recvMessages[dest], nil
}

func (egs *externalGRPCServer) Shutdown() {
	egs.grpcListener.Close()
}

func (egs *externalGRPCServer) initializeExternalListener(ctx context.Context) error {
	externalGRPCListener, err := net.Listen("tcp", fmt.Sprintf(":%d", egs.port))
	if err != nil {
		log.L(ctx).Errorf("grpctransport: failed to listen for external grpc connections: %v", err)
		return err
	}

	egs.grpcListener = externalGRPCListener
	s := grpc.NewServer()
	interPaladinPB.RegisterInterPaladinTransportServer(s, egs)

	// Monitor new inbound messages coming in
	go func() {
		log.L(ctx).Infof("grpctransport: external gRPC endpoint listening at %v", externalGRPCListener.Addr())
		if err := s.Serve(externalGRPCListener); err != nil {
			log.L(ctx).Errorf("failed to serve: %v", err)
		}
	}()

	// And also monitor the send queue
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _ = <-egs.sendMessages:
				{
					// TODO: Serialisation of the message into bytes and then send it over TCP
					// TODO: mTLS plug point goes here
					return
				}
			}
		}
	}()

	return nil
}

func (egs *externalGRPCServer) SendInterPaladinMessage(ctx context.Context, message *interPaladinPB.InterPaladinMessage) (*interPaladinPB.Empty, error) {
	// TODO: Deserialisation of the message here back into a proto.Message and then send through the comms bus
	return nil, nil
}
