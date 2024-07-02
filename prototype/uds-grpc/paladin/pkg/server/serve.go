package server

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"

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

func Run() {
	//lis, err := net.Listen("tcp", ":50051")
	lis, err := net.Listen("unix", "/tmp/grpc.sock")

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterPaladinServiceServer(s, &server{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
