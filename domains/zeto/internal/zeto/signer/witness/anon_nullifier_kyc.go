package witness

import (
	"context"
	"math/big"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/common"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
)

type FungibleNullifierKycWitnessInputs struct {
	FungibleWitnessInputs
	Extras *pb.ProvingRequestExtras_NullifiersKyc
}

func (inputs *FungibleNullifierKycWitnessInputs) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	nullifiers, utxoRoot, utxoProofs, enabled, kycRoot, kycProofs, delegate, err := inputs.prepareInputsForNullifiers(ctx, inputs.Extras, keyEntry)
	if err != nil {
		return nil, err
	}

	m, err := inputs.FungibleWitnessInputs.Assemble(ctx, keyEntry)
	if err != nil {
		return nil, err
	}
	m["nullifiers"] = nullifiers
	m["utxosRoot"] = utxoRoot
	m["utxosMerkleProof"] = utxoProofs
	m["enabled"] = enabled
	m["identitiesRoot"] = kycRoot
	m["identitiesMerkleProof"] = kycProofs
	if delegate != nil {
		m["lockDelegate"] = delegate
	}
	return m, nil
}

func (inputs *FungibleNullifierKycWitnessInputs) prepareInputsForNullifiers(ctx context.Context, extras *pb.ProvingRequestExtras_NullifiersKyc, keyEntry *core.KeyEntry) ([]*big.Int, *big.Int, [][]*big.Int, []*big.Int, *big.Int, [][]*big.Int, *big.Int, error) {
	// calculate the nullifiers for the input UTXOs
	nullifiers := make([]*big.Int, len(inputs.inputCommitments))
	for i := 0; i < len(inputs.inputCommitments); i++ {
		// if the input commitment is 0, as a filler, the nullifier is 0
		if inputs.inputCommitments[i].Cmp(big.NewInt(0)) == 0 {
			nullifiers[i] = big.NewInt(0)
			continue
		}
		nullifier, err := common.CalculateNullifier(inputs.inputValues[i], inputs.inputSalts[i], keyEntry.PrivateKeyForZkp)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorCalcNullifier, err)
		}
		nullifiers[i] = nullifier
	}

	utxoRoot, utxoProofs, enabled, err := inputs.decodeSmtProofObject(ctx, extras.SmtUtxoProof)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	kycRoot, kycProofs, _, err := inputs.decodeSmtProofObject(ctx, extras.SmtKycProof)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	var delegate *big.Int
	var ok bool
	if extras.Delegate != "" {
		delegate, ok = new(big.Int).SetString(strings.TrimPrefix(extras.Delegate, "0x"), 16)
		if !ok {
			return nil, nil, nil, nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeDelegateExtras, extras.Delegate)
		}
	}

	return nullifiers, utxoRoot, utxoProofs, enabled, kycRoot, kycProofs, delegate, nil
}

func (inputs *FungibleNullifierKycWitnessInputs) decodeSmtProofObject(ctx context.Context, proofObj *pb.MerkleProofObject) (*big.Int, [][]*big.Int, []*big.Int, error) {
	root, ok := new(big.Int).SetString(proofObj.Root, 16)
	if !ok {
		return nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeRootExtras)
	}
	proofs := make([][]*big.Int, len(proofObj.MerkleProofs))
	for i, proof := range proofObj.MerkleProofs {
		mp := make([]*big.Int, len(proof.Nodes))
		for j, node := range proof.Nodes {
			n, ok := new(big.Int).SetString(node, 16)
			if !ok {
				return nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeMTPNodeExtras)
			}
			mp[j] = n
		}
		proofs[i] = mp
	}
	enabled := make([]*big.Int, len(proofObj.Enabled))
	for i, e := range proofObj.Enabled {
		if e {
			enabled[i] = big.NewInt(1)
		} else {
			enabled[i] = big.NewInt(0)
		}
	}
	return root, proofs, enabled, nil
}
