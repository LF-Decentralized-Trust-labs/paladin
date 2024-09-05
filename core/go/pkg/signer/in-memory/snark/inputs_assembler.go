package snark

import (
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
)

func assembleInputs_anon(inputs *commonWitnessInputs, keyEntry *core.KeyEntry) map[string]interface{} {
	witnessInputs := map[string]interface{}{
		"inputCommitments":      inputs.inputCommitments,
		"inputValues":           inputs.inputValues,
		"inputSalts":            inputs.inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
	}
	return witnessInputs
}

func assembleInputs_anon_enc(inputs *commonWitnessInputs, keyEntry *core.KeyEntry) map[string]interface{} {
	nonce := utxo.NewEncryptionNonce()
	witnessInputs := map[string]interface{}{
		"inputCommitments":      inputs.inputCommitments,
		"inputValues":           inputs.inputValues,
		"inputSalts":            inputs.inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
		"encryptionNonce":       nonce,
	}
	return witnessInputs
}
