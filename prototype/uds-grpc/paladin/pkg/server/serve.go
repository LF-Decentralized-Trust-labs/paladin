package server

import (
	"context"
	"net"

	"github.com/hyperledger/firefly-common/pkg/log"
	"google.golang.org/grpc"

	"github.com/kaleido-io/paladin/internal/domain"
	pb "github.com/kaleido-io/paladin/internal/protos/domain"
)

// server is used to implement example.GreeterServer.
type server struct {
	pb.UnimplementedPaladinServiceServer
}

func (s *server) GetStates(ctx context.Context, in *pb.GetStatesRequest) (*pb.GetStatesReply, error) {

	return &pb.GetStatesReply{StateId: []string{"stateA", "stateB"}}, nil
}

func (s *server) RegisterDomain(stream pb.PaladinService_RegisterDomainServer) error {

	ctx := stream.Context()
	newDomain := domain.NewDomain(stream)
	log.L(ctx).Info("RegisteredDomain")

	err := newDomain.Listen()
	if err != nil {
		log.L(ctx).Error("Error listening", err)
		return err
	}
	//if we exit from this function, the stream will be closed
	log.L(ctx).Info("ClosingDomain")
	return nil
}

func Run() {
	ctx := context.Background()
	lis, err := net.Listen("tcp", ":50051")
	//lis, err := net.Listen("unix", "/tmp/grpc.sock")

	if err != nil {
		log.L(ctx).Error("failed to listen: ", err)
	}
	s := grpc.NewServer()
	pb.RegisterPaladinServiceServer(s, &server{})
	log.L(ctx).Infof("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.L(ctx).Error("failed to serve: ", err)
	}
}
