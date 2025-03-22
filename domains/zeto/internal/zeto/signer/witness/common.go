package witness

import (
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
)

type CommonWitnessInputs struct {
	inputCommitments      []*big.Int
	inputSalts            []*big.Int
	outputCommitments     []*big.Int
	outputSalts           []*big.Int
	outputOwnerPublicKeys [][]*big.Int
}

func (inputs *CommonWitnessInputs) Assemble(keyEntry *core.KeyEntry) map[string]interface{} {
	return map[string]interface{}{
		"inputCommitments":      inputs.inputCommitments,
		"inputSalts":            inputs.inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     inputs.outputCommitments,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
	}
}
