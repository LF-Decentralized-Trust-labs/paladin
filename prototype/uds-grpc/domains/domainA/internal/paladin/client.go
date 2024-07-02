package paladin

import (
	"context"
	"time"

	"github.com/hyperledger/firefly-common/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/kaleido-io/paladin-domains/domainA/internal/protos/domain"
)

func DoThatThing() {

	ctx := context.Background()
	//https://github.com/grpc/grpc/blob/master/doc/naming.md#name-syntax
	conn, err := grpc.NewClient("unix:/tmp/grpc.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
	//conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.L(ctx).Error("fail to dial: ", err)
	}
	defer conn.Close()

	client := pb.NewPaladinServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	response, err := client.SayHello(ctx, &pb.HelloRequest{
		Name: "domain A",
	})
	if err != nil {
		log.L(ctx).Error("Could not call SayHello: ", err)
	}
	log.L(ctx).Infof("Response from server: %v, -- %s", response, response.GetMessage())

	commandStream, err := client.RegisterDomain(ctx)
	if err != nil {
		log.L(ctx).Error("failed to register domain", err)
		return
	}

	if err := commandStream.Send(&pb.DomainEvent{
		CommandId: "0",
		DomainId:  "domainA",
		Arguments: []string{},
	}); err != nil {
		log.L(ctx).Error("failed to send event", err)
		return
	}

}
