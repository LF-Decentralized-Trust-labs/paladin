package domain

import (
	"github.com/kalaeido-io/paladin/pkg/transaction"
)

type Domain interface {
	Assemble(request transaction.Request) (transaction.Assembly, error)
	Define() (string, error)
}
