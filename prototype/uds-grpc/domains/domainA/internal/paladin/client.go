package paladin

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/kaleido-io/paladin-domains/domainA/internal/protos/domain"
)

func DoThatThing() {

	//https://github.com/grpc/grpc/blob/master/doc/naming.md#name-syntax
	conn, err := grpc.NewClient("unix:/tmp/grpc.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
	//conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewPaladinServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	response, err := client.SayHello(ctx, &pb.HelloRequest{
		Name: "domain A",
	})
	if err != nil {
		log.Fatalf("Could not call SayHello: %v", err)
	}
	log.Printf("Response from server: %v, -- %s", response, response.GetMessage())
}
