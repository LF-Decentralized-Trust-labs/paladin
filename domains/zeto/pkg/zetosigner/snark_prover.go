package zetosigner

import (
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/signer"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signerapi"
)

func NewSnarkProver(conf *zetosignerapi.SnarkProverConfig) (signerapi.InMemorySigner, error) {
	return signer.NewSnarkProver(conf)
}
