package grpctransport

import (
	"context"
	"fmt"
	"net"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	interPaladinPB "github.com/kaleido-io/paladin/kata/pkg/proto/interpaladin"
	"google.golang.org/protobuf/types/known/anypb"
)

type ExternalMessage struct {
	proto.Message

	ExternalAddress string

	// TODO: mTLS certs
}

type ExternalServer interface {
	QueueMessageForSend(msg *ExternalMessage)
	GetMessages(dest destination) (chan *proto.Message, error)
	Shutdown()
}

type externalGRPCServer struct {
	interPaladinPB.UnimplementedInterPaladinTransportServer

	grpcListener net.Listener
	server       *grpc.Server

	// TODO: We probably don't want to do this, what happens when we're not consuming messages correctly?
	recvMessages map[destination]chan *proto.Message
	sendMessages chan *ExternalMessage
	port         int
}

func NewExternalGRPCServer(ctx context.Context, port int, bufferSize int) (*externalGRPCServer, error) {
	server := &externalGRPCServer{
		recvMessages: make(map[destination]chan *proto.Message, bufferSize),
		sendMessages: make(chan *ExternalMessage, bufferSize),
		port:         port,
	}

	err := server.initializeExternalListener(ctx)
	if err != nil {
		log.L(ctx).Errorf("grpcexternal: Error initializing external listener: %v", err)
		return nil, err
	}

	return server, nil
}

func (egs *externalGRPCServer) QueueMessageForSend(msg *ExternalMessage) {
	egs.sendMessages <- msg
}

func (egs *externalGRPCServer) GetMessages(dest destination) (chan *proto.Message, error) {
	if egs.recvMessages[dest] == nil {
		return nil, fmt.Errorf("could not find entries for the provided destination")
	}

	return egs.recvMessages[dest], nil
}

func (egs *externalGRPCServer) Shutdown() {
	egs.server.GracefulStop()
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
	egs.server = s
	interPaladinPB.RegisterInterPaladinTransportServer(s, egs)

	// Monitor new inbound messages coming in
	go func() {
		log.L(ctx).Infof("grpctransport: external gRPC endpoint listening at %v", externalGRPCListener.Addr())
		if err := s.Serve(externalGRPCListener); err != nil {
			if err == io.EOF {
				return
			}
			log.L(ctx).Errorf("failed to serve: %v", err)
		}
	}()

	// And also monitor the send queue
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sendMsg := <-egs.sendMessages:
				{
					bytes, err := anypb.New(sendMsg)
					if err != nil {
						log.L(ctx).Errorf("grpctransport: could not send message")
						continue
					}

					inpalMessage := &interPaladinPB.InterPaladinMessage{
						Body: bytes,
					}

					conn, err := grpc.NewClient(sendMsg.ExternalAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err != nil {
						log.L(ctx).Errorf("Failed to establish a client, err: %s", err)
					}
					defer conn.Close()
				
					client := interPaladinPB.NewInterPaladinTransportClient(conn)
				
					_, err = client.SendInterPaladinMessage(ctx, inpalMessage)
					if err != nil {
						log.L(ctx).Errorf("error sending message: %s", err.Error())
					}

					// TODO: mTLS plug point goes here
					return
				}
			}
		}
	}()

	return nil
}

func (egs *externalGRPCServer) SendInterPaladinMessage(ctx context.Context, message *interPaladinPB.InterPaladinMessage) (*interPaladinPB.InterPaladinMessage, error) {
	recvMessage := &ExternalMessage{}
	err := message.GetBody().UnmarshalTo(recvMessage)
	if err != nil {
		return nil, err
	}

	if egs.recvMessages[destination(recvMessage.Destination)] == nil {
		egs.recvMessages[destination(recvMessage.Destination)] = make(chan *proto.Message, 1)
	} 
	
	egs.recvMessages[destination(recvMessage.Destination)] <- &recvMessage.Message
	return nil, nil
}
