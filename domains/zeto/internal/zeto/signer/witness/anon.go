package witness

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/common"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
)

type FungibleWitnessInputs struct {
	CommonWitnessInputs
	inputValues  []*big.Int
	outputValues []*big.Int
}

func (f *FungibleWitnessInputs) Build(ctx context.Context, commonInputs *pb.ProvingRequestCommon) error {

	var tokenData pb.TokenSecrets_Fungible
	if err := json.Unmarshal(commonInputs.TokenSecrets, &tokenData); err != nil {
		return i18n.NewError(ctx, msgs.MsgErrorUnmarshalTokenSecretsNonFungible, err)
	}

	// construct the output UTXOs based on the values and owner public keys
	outputCommitments := make([]*big.Int, len(tokenData.OutputValues))
	outputSalts := make([]*big.Int, len(tokenData.OutputValues))
	outputOwnerPublicKeys := make([][]*big.Int, len(tokenData.OutputValues))
	outputValues := make([]*big.Int, len(tokenData.OutputValues))

	for i := 0; i < len(commonInputs.OutputSalts); i++ {
		salt, ok := new(big.Int).SetString(commonInputs.OutputSalts[i], 16)
		if !ok {
			return i18n.NewError(ctx, msgs.MsgErrorParseOutputSalt)
		}
		outputSalts[i] = salt

		if salt.Cmp(big.NewInt(0)) == 0 {
			outputOwnerPublicKeys[i] = []*big.Int{big.NewInt(0), big.NewInt(0)}
			outputValues[i] = big.NewInt(0)
			outputCommitments[i] = big.NewInt(0)
		} else {
			ownerPubKey, err := common.DecodeBabyJubJubPublicKey(commonInputs.OutputOwners[i])
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
			}
			outputOwnerPublicKeys[i] = []*big.Int{ownerPubKey.X, ownerPubKey.Y}
			value := tokenData.OutputValues[i]
			outputValues[i] = new(big.Int).SetUint64(value)
			u := utxo.NewFungible(new(big.Int).SetUint64(value), ownerPubKey, salt)
			hash, err := u.GetHash()
			if err != nil {
				return err
			}
			outputCommitments[i] = hash
		}
	}

	inputCommitments := make([]*big.Int, len(commonInputs.InputCommitments))
	inputValues := make([]*big.Int, len(tokenData.InputValues))
	inputSalts := make([]*big.Int, len(commonInputs.InputSalts))
	for i, c := range commonInputs.InputCommitments {
		// commitment
		commitment, ok := new(big.Int).SetString(c, 16)
		if !ok {
			return i18n.NewError(ctx, msgs.MsgErrorParseInputCommitment)
		}
		inputCommitments[i] = commitment
		inputValues[i] = new(big.Int).SetUint64(tokenData.InputValues[i])

		// slat
		salt, ok := new(big.Int).SetString(commonInputs.InputSalts[i], 16)
		if !ok {
			return i18n.NewError(ctx, msgs.MsgErrorParseInputSalt)
		}
		inputSalts[i] = salt
	}

	f.inputCommitments = inputCommitments
	f.inputValues = inputValues
	f.inputSalts = inputSalts
	f.outputValues = outputValues
	f.outputCommitments = outputCommitments
	f.outputSalts = outputSalts
	f.outputOwnerPublicKeys = outputOwnerPublicKeys

	return nil
}

func (f *FungibleWitnessInputs) Validate(ctx context.Context, inputs *pb.ProvingRequestCommon) error {
	if inputs.TokenType != pb.TokenType_fungible {
		return i18n.NewError(ctx, msgs.MsgErrorTokenTypeMismatch, inputs.TokenType, pb.TokenType_fungible)
	}

	var token pb.TokenSecrets_Fungible
	if err := json.Unmarshal(inputs.TokenSecrets, &token); err != nil {
		return i18n.NewError(ctx, msgs.MsgErrorUnmarshalTokenSecretsFungible, err)
	}

	if len(inputs.InputCommitments) != len(token.InputValues) || len(inputs.InputCommitments) != len(inputs.InputSalts) {
		return i18n.NewError(ctx, msgs.MsgErrorInputsDiffLength)
	}
	if len(token.OutputValues) != len(inputs.OutputOwners) {
		return i18n.NewError(ctx, msgs.MsgErrorOutputsDiffLength)
	}
	return nil
}

func (inputs *FungibleWitnessInputs) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	m := inputs.CommonWitnessInputs.Assemble(keyEntry)
	m["inputValues"] = inputs.inputValues
	m["outputValues"] = inputs.outputValues
	return m, nil
}
