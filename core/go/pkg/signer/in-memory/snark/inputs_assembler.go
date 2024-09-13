package snark

import (
	"fmt"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	pb "github.com/kaleido-io/paladin/core/pkg/proto"
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

func assembleInputs_anon_enc(inputs *commonWitnessInputs, extras *pb.ProvingRequestExtras_Encryption, keyEntry *core.KeyEntry) (map[string]any, map[string]string, error) {
	var nonce *big.Int
	if extras != nil && extras.EncryptionNonce != "" {
		n, ok := new(big.Int).SetString(extras.EncryptionNonce, 10)
		if !ok {
			return nil, nil, fmt.Errorf("failed to parse encryption nonce")
		}
		nonce = n
	} else {
		nonce = crypto.NewEncryptionNonce()
	}
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
	publicInputs := map[string]string{
		"encryptionNonce": nonce.Text(10),
	}
	return witnessInputs, publicInputs, nil
}
