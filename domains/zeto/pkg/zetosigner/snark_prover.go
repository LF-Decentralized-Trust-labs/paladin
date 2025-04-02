package zetosigner

import (
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/signer"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/signerapi"
)

func NewSnarkProver(conf *zetosignerapi.SnarkProverConfig) (signerapi.InMemorySigner, error) {
	return signer.NewSnarkProver(conf)
}
