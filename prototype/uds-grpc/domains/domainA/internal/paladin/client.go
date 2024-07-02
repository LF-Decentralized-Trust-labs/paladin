package paladin

import (
	"context"
	"io"

	"github.com/hyperledger/firefly-common/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/kaleido-io/paladin-domains/domainA/internal/protos/domain"
)

func DoThatThing() {

	ctx, cancel := context.WithCancel(context.Background())

	//https://github.com/grpc/grpc/blob/master/doc/naming.md#name-syntax
	conn, err := grpc.NewClient("unix:/tmp/grpc.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
	//conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.L(ctx).Error("fail to dial: ", err)
	}
	defer conn.Close()

	client := pb.NewPaladinServiceClient(conn)

	defer cancel()

	commandStream, err := client.RegisterDomain(ctx)
	if err != nil {
		log.L(ctx).Error("failed to register domain", err)
		return
	}
	deadline, ok := commandStream.Context().Deadline()
	log.L(ctx).Infof("RegisteredDomain: %v, %v", deadline, ok)

	if err := commandStream.Send(&pb.DomainEvent{
		CommandId: "0",
		DomainId:  "domainA",
		Arguments: []string{},
	}); err != nil {
		log.L(ctx).Error("failed to send event", err)
		return
	}

	//go into an endless loop waiting for commands from the server or interupts
	for {
		command, err := commandStream.Recv()
		if err == io.EOF {
			log.L(ctx).Infof("EOF")
			// read done.
			return
		}
		if err != nil {
			log.L(ctx).Error("failed to receive a command", err)
		}
		log.L(ctx).Infof("Received command: %v", command)
		switch command.Command {
		case "Assemble":
			log.L(ctx).Infof("Processing Assemble command")
			unspentStates, err := client.GetStates(ctx, &pb.GetStatesRequest{
				MetaState: "unspent",
			})
			if err != nil {
				log.L(ctx).Error("Could not call GetStates: ", err)
			}
			log.L(ctx).Infof("Response from server: %v", unspentStates)
			if err := commandStream.Send(&pb.DomainEvent{
				CommandId: command.Id,
				DomainId:  "domainA",
				Arguments: []string{"Reply to assemble"},
			}); err != nil {
				log.L(ctx).Error("failed to send event", err)
				return
			}
		case "Endorse":
			log.L(ctx).Infof("Processing Endorse command")

			if err := commandStream.Send(&pb.DomainEvent{
				CommandId: command.Id,
				DomainId:  "domainA",
				Arguments: []string{"Reply to endorse"},
			}); err != nil {
				log.L(ctx).Error("failed to send event", err)
				return
			}
		default:
			log.L(ctx).Infof("Received unexpected command %v", command)
		}
	}
}
