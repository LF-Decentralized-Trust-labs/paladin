package domain

import (
	"io"

	"github.com/hyperledger/firefly-common/pkg/log"

	pb "github.com/kalaeido-io/paladin/internal/protos/domain"
)

func DomainListener(stream pb.PaladinService_RegisterDomainServer) error {
	ctx := stream.Context()
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			log.L(ctx).Info("EOF")
			return nil
		}
		if err != nil {
			log.L(ctx).Error("Error receiving", err)

			return err
		}

		log.L(ctx).Infof("Registering domain %v", in)

		if err := stream.Send(&pb.DomainCommand{
			Command:   "Assemble",
			DomainId:  in.DomainId,
			Arguments: []string{"txn:00001"},
		}); err != nil {
			return err
		}
	}

}
