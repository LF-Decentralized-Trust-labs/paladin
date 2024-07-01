package main

import "C"
import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kalaeido-io/paladin/pkg/domain"
	"github.com/kalaeido-io/paladin/pkg/transaction"
)

type domainA struct {
}

func (d *domainA) Assemble(request transaction.Request) (transaction.Assembly, error) {
	return transaction.Assembly{}, nil
}

func (d *domainA) Define() (string, error) {
	return "Domain a is really cool", nil
}

func LoadDomain() domain.Domain {
	ctx := context.Background()
	log.L(ctx).Infof("Hello from domain A")
	return &domainA{}
}

func main() {}
