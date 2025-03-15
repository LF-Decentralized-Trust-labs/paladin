/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package signer

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	pb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
)

type witnessInputs interface {
	validate(ctx context.Context, inputs *pb.ProvingRequestCommon) error
	build(ctx context.Context, commonInputs *pb.ProvingRequestCommon) error
	assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error)
}

type commonWitnessInputs struct {
	inputCommitments      []*big.Int
	inputSalts            []*big.Int
	outputCommitments     []*big.Int
	outputSalts           []*big.Int
	outputOwnerPublicKeys [][]*big.Int
}

var _ witnessInputs = &fungibleWitnessInputs{}

type fungibleWitnessInputs struct {
	commonWitnessInputs
	inputValues  []*big.Int
	outputValues []*big.Int
}

var _ witnessInputs = &depositWitnessInputs{}

type depositWitnessInputs struct {
	fungibleWitnessInputs
}

var _ witnessInputs = &lockWitnessInputs{}

type lockWitnessInputs struct {
	fungibleWitnessInputs
}

var _ witnessInputs = &fungibleEncWitnessInputs{}

type fungibleEncWitnessInputs struct {
	fungibleWitnessInputs
	enc *pb.ProvingRequestExtras_Encryption
}

var _ witnessInputs = &fungibleNullifierWitnessInputs{}

type fungibleNullifierWitnessInputs struct {
	fungibleWitnessInputs
	nul *pb.ProvingRequestExtras_Nullifiers
}

var _ witnessInputs = &nonFungibleWitnessInputs{}

type nonFungibleWitnessInputs struct {
	commonWitnessInputs
	tokenIDs  []*big.Int
	tokenURIs []*big.Int
}

func (f *fungibleWitnessInputs) build(ctx context.Context, commonInputs *pb.ProvingRequestCommon) error {

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
			ownerPubKey, err := DecodeBabyJubJubPublicKey(commonInputs.OutputOwners[i])
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

func (f *nonFungibleWitnessInputs) build(ctx context.Context, commonInputs *pb.ProvingRequestCommon) error {

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
		ownerPubKey, err := DecodeBabyJubJubPublicKey(commonInputs.OutputOwners[i])
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

func (f *fungibleWitnessInputs) validate(ctx context.Context, inputs *pb.ProvingRequestCommon) error {
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

func (f *nonFungibleWitnessInputs) validate(ctx context.Context, inputs *pb.ProvingRequestCommon) error {
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
