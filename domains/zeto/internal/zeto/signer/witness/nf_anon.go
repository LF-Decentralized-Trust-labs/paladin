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

type NonFungibleWitnessInputs struct {
	CommonWitnessInputs
	tokenIDs  []*big.Int
	tokenURIs []*big.Int
}

func (f *NonFungibleWitnessInputs) Build(ctx context.Context, commonInputs *pb.ProvingRequestCommon) error {

	// input UTXOs
	inputCommitments := make([]*big.Int, len(commonInputs.InputCommitments))
	inputSalts := make([]*big.Int, len(commonInputs.InputSalts))
	for i := range commonInputs.InputCommitments {
		commitment, ok := new(big.Int).SetString(commonInputs.InputCommitments[i], 16)
		if !ok {
			return i18n.NewError(ctx, msgs.MsgErrorParseInputCommitment)
		}
		inputCommitments[i] = commitment

		salt, ok := new(big.Int).SetString(commonInputs.InputSalts[i], 16)
		if !ok {
			return i18n.NewError(ctx, msgs.MsgErrorParseInputSalt)
		}
		inputSalts[i] = salt
	}

	// output UTXOs
	outputCommitments := make([]*big.Int, len(commonInputs.OutputCommitments))
	outputSalts := make([]*big.Int, len(commonInputs.OutputSalts))
	outputOwnerPublicKeys := make([][]*big.Int, len(commonInputs.OutputSalts))
	for i := range commonInputs.OutputOwners {
		ownerPubKey, err := common.DecodeBabyJubJubPublicKey(commonInputs.OutputOwners[i])
		if err != nil {
			return i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}
		outputOwnerPublicKeys[i] = []*big.Int{ownerPubKey.X, ownerPubKey.Y}
	}
	for i := range commonInputs.OutputSalts {
		salt, ok := new(big.Int).SetString(commonInputs.OutputSalts[i], 16)
		if !ok {
			return i18n.NewError(ctx, msgs.MsgErrorParseOutputSalt)
		}
		outputSalts[i] = salt
	}

	for i := range commonInputs.OutputCommitments {
		commitment, ok := new(big.Int).SetString(commonInputs.OutputCommitments[i], 16)
		if !ok {
			return i18n.NewError(ctx, msgs.MsgErrorParseOutputStates)
		}
		outputCommitments[i] = commitment
	}

	var tokenData pb.TokenSecrets_NonFungible
	if err := json.Unmarshal(commonInputs.TokenSecrets, &tokenData); err != nil {
		return i18n.NewError(ctx, msgs.MsgErrorUnmarshalTokenSecretsNonFungible, err)
	}

	tokenIDs := make([]*big.Int, len(tokenData.TokenIds))
	for i, id := range tokenData.TokenIds {
		t, k := new(big.Int).SetString(id, 0)
		if !k {
			return i18n.NewError(ctx, msgs.MsgErrorTokenIDToString, id)
		}
		tokenIDs[i] = t
	}

	tokenURIs := make([]*big.Int, len(tokenData.TokenUris))
	for i := range tokenData.TokenUris {
		uri, err := utxo.HashTokenUri(tokenData.TokenUris[i])
		if err != nil {
			return i18n.NewError(ctx, msgs.MsgErrorHashState, err)
		}
		tokenURIs[i] = uri
	}

	f.inputCommitments = inputCommitments
	f.inputSalts = inputSalts
	f.outputCommitments = outputCommitments
	f.outputSalts = outputSalts
	f.outputOwnerPublicKeys = outputOwnerPublicKeys
	f.tokenIDs = tokenIDs
	f.tokenURIs = tokenURIs

	return nil
}

func (f *NonFungibleWitnessInputs) Validate(ctx context.Context, inputs *pb.ProvingRequestCommon) error {
	if inputs.TokenType != pb.TokenType_nunFungible {
		return i18n.NewError(ctx, msgs.MsgErrorTokenTypeMismatch, inputs.TokenType, pb.TokenType_nunFungible)
	}

	var token pb.TokenSecrets_NonFungible
	if err := json.Unmarshal(inputs.TokenSecrets, &token); err != nil {
		return i18n.NewError(ctx, msgs.MsgErrorUnmarshalTokenSecretsNonFungible, err)
	}

	if len(inputs.InputCommitments) != len(inputs.InputSalts) {
		return i18n.NewError(ctx, msgs.MsgErrorInputsDiffLength)
	}
	return nil
}

func (inputs *NonFungibleWitnessInputs) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	m := inputs.CommonWitnessInputs.Assemble(keyEntry)
	m["tokenIds"] = inputs.tokenIDs
	m["tokenUris"] = inputs.tokenURIs
	return m, nil
}
