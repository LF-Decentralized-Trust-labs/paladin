package witness

import (
	"context"
	"math/big"
	"strings"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/signer/common"
	pb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
)

type FungibleNullifierWitnessInputs struct {
	FungibleWitnessInputs
	Extras *pb.ProvingRequestExtras_Nullifiers
}

func (inputs *FungibleNullifierWitnessInputs) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	nullifiers, root, proofs, enabled, delegate, err := inputs.prepareInputsForNullifiers(ctx, inputs.Extras, keyEntry)
	if err != nil {
		return nil, err
	}

	m, err := inputs.FungibleWitnessInputs.Assemble(ctx, keyEntry)
	if err != nil {
		return nil, err
	}
	m["nullifiers"] = nullifiers
	m["root"] = root
	m["merkleProof"] = proofs
	m["enabled"] = enabled
	if delegate != nil {
		m["lockDelegate"] = delegate
	}
	return m, nil
}

func (inputs *FungibleNullifierWitnessInputs) prepareInputsForNullifiers(ctx context.Context, extras *pb.ProvingRequestExtras_Nullifiers, keyEntry *core.KeyEntry) ([]*big.Int, *big.Int, [][]*big.Int, []*big.Int, *big.Int, error) {
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
			return nil, nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorCalcNullifier, err)
		}
		nullifiers[i] = nullifier
	}
	root, ok := new(big.Int).SetString(extras.Root, 16)
	if !ok {
		return nil, nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeRootExtras)
	}
	var proofs [][]*big.Int
	for _, proof := range extras.MerkleProofs {
		var mp []*big.Int
		for _, node := range proof.Nodes {
			n, ok := new(big.Int).SetString(node, 16)
			if !ok {
				return nil, nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeMTPNodeExtras)
			}
			mp = append(mp, n)
		}
		proofs = append(proofs, mp)
	}
	enabled := make([]*big.Int, len(extras.Enabled))
	for i, e := range extras.Enabled {
		if e {
			enabled[i] = big.NewInt(1)
		} else {
			enabled[i] = big.NewInt(0)
		}
	}
	var delegate *big.Int
	if extras.Delegate != "" {
		delegate, ok = new(big.Int).SetString(strings.TrimPrefix(extras.Delegate, "0x"), 16)
		if !ok {
			return nil, nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeDelegateExtras, extras.Delegate)
		}
	}

	return nullifiers, root, proofs, enabled, delegate, nil
}
