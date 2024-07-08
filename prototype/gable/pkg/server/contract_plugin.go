package server

import (
	"context"
	"io"

	"github.com/aidarkhanov/nanoid"
	"github.com/hyperledger/firefly-common/pkg/log"

	"github.com/kaleido-io/paladin/gable/pkg/protobufs"
)

type ContractPlugin interface {
	Listen()
}

type contractPlugin struct {
	eventHandlerDone chan struct{}
	eventStream      protobufs.PaladinContractPluginService_RegisterServer
	contractId       string
}

func NewContractPlugin(contractEventStream protobufs.PaladinContractPluginService_RegisterServer) ContractPlugin {
	return &contractPlugin{
		eventStream: contractEventStream,
	}
}

func (cp *contractPlugin) eventHandler(ctx context.Context) {
	defer close(cp.eventHandlerDone)
	for {
		event, err := cp.eventStream.Recv()
		if err == io.EOF {
			log.L(ctx).Info("EOF - exiting")
			return
		}

		if event.CorrelationId == "" {
			// Always just send back an ack for now
			log.L(ctx).Infof("Received event %s [%s]", event, event.Type)
			if err := cp.eventStream.Send(&protobufs.ContractPluginEvent{
				ContractPluginId: cp.contractId,
				Type:             "ack",
				Arguments:        []string{},
				Id:               nanoid.New(),
				CorrelationId:    event.Id,
			}); err != nil {
				log.L(ctx).Error("Error sending - closing channel", err)
				return
			}
		} else {
			log.L(ctx).Infof("Received reply %s to event %s [%s]", event, event.CorrelationId, event.Type)
		}
	}
}

func (cp *contractPlugin) Listen() {

	cp.eventHandler(log.WithLogField(cp.eventStream.Context(), "contractId", cp.contractId))

}
