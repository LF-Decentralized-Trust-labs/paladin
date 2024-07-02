package domain

import (
	"context"
	"io"

	"github.com/hyperledger/firefly-common/pkg/log"

	pb "github.com/kalaeido-io/paladin/internal/protos/domain"
)

type Domain interface {
	Listen() error
}

type domain struct {
	eventStream pb.PaladinService_RegisterDomainServer
	domainId    string
}

func NewDomain(domainEventStream pb.PaladinService_RegisterDomainServer) Domain {
	return &domain{
		eventStream: domainEventStream,
	}
}

func (d *domain) waitForReply(ctx context.Context, commandId string) (*pb.DomainEvent, error) {
	for {
		event, err := d.eventStream.Recv()
		if err == io.EOF {
			log.L(ctx).Info("EOF")
			return nil, nil
		}
		if err != nil {
			log.L(ctx).Error("Error receiving", err)
			return nil, err
		}
		if event.CommandId == commandId {
			return event, nil
		}
	}
}

func (d *domain) Listen() error {

	ctx := d.eventStream.Context()
	log.L(ctx).Infof("Registering domain. Waiting for first event with domain initialiser")

	domainInitialiser, err := d.waitForReply(ctx, "0")
	if domainInitialiser == nil {
		log.L(ctx).Error("Error waiting for domainInitialiser", err)
		return err
	}
	log.L(ctx).Infof("Received domainInitialiser %v", domainInitialiser)
	d.domainId = domainInitialiser.DomainId

	log.L(ctx).Info("Sending assemble command")
	if err := d.eventStream.Send(&pb.DomainCommand{
		Command:   "Assemble",
		DomainId:  d.domainId,
		Arguments: []string{"txn:00001"},
		Id:        "1",
	}); err != nil {
		log.L(ctx).Error("Error sending", err)
		return err
	}

	assembleReply, err := d.waitForReply(ctx, "1")
	if assembleReply == nil {
		log.L(ctx).Error("Error waiting for assembleReply", err)
		return err
	}
	log.L(ctx).Infof("Received response to assemble command %v", assembleReply)
	log.L(ctx).Info("Sending endorse command")

	if err := d.eventStream.Send(&pb.DomainCommand{
		Command:   "Endorse",
		DomainId:  d.domainId,
		Arguments: []string{"txn:00001"},
		Id:        "2",
	}); err != nil {
		log.L(ctx).Error("Error sending", err)
		return err
	}
	endorseReply, err := d.waitForReply(ctx, "2")
	if endorseReply == nil {
		log.L(ctx).Error("Error waiting for endorseReply", err)
		return err
	}
	return nil
}
