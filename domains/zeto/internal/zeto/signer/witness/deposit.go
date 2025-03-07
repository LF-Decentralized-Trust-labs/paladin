package witness

import (
	"context"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
)

type DepositWitnessInputs struct {
	FungibleWitnessInputs
}

func (inputs *DepositWitnessInputs) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	return map[string]interface{}{
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
	}, nil
}
