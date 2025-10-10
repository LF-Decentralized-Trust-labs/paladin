package zetosigner

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
)

func NewSnarkProver(conf *zetosignerapi.SnarkProverConfig) (signerapi.InMemorySigner, error) {
	return signer.NewSnarkProver(conf)
}
