package server

import (
	"context"
	"net"

	"github.com/hyperledger/firefly-common/pkg/log"
	"google.golang.org/grpc"

	"github.com/kalaeido-io/paladin/internal/domain"
	pb "github.com/kalaeido-io/paladin/internal/protos/domain"
)

// server is used to implement example.GreeterServer.
type server struct {
	pb.UnimplementedPaladinServiceServer
}

// SayHello implements example.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "Hello " + in.GetName()}, nil
}

func (s *server) RegisterDomain(stream pb.PaladinService_RegisterDomainServer) error {

	ctx := stream.Context()
	// go domain.DomainListener(stream)
	domain.DomainListener(stream)

	log.L(ctx).Info("RegisteredDomain")
	return nil
}

func Run() {
	ctx := context.Background()
	//lis, err := net.Listen("tcp", ":50051")
	lis, err := net.Listen("unix", "/tmp/grpc.sock")

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
