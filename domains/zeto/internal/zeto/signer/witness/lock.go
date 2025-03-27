package witness

import (
	"context"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
)

type LockWitnessInputs struct {
	FungibleWitnessInputs
}

func (inputs *LockWitnessInputs) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	return map[string]interface{}{
		"commitments":     inputs.inputCommitments,
		"values":          inputs.inputValues,
		"salts":           inputs.inputSalts,
		"ownerPrivateKey": keyEntry.PrivateKeyForZkp,
	}, nil
}
