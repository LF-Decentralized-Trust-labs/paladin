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

package nonfungible

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
)

// this is a helper function to generate a random number
// it is a variable so that it can be mocked in tests
var Rand256 func() (*big.Int, error) = common.CryptoRandBN254

var NewSalt func() *big.Int = crypto.NewSalt

func makeNFToken(stateData string) (*types.ZetoNFToken, error) {
	token := &types.ZetoNFToken{}
	err := json.Unmarshal([]byte(stateData), &token)
	return token, err
}

func makeNewState(ctx context.Context, stateSchema *pb.StateSchema, useNullifiers bool, token *types.ZetoNFToken, name, owner string) (*pb.NewState, error) {
	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return nil, err
	}
	hash, err := token.Hash(ctx)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashState, err)
	}

	hashStr := common.IntTo32ByteHexString(hash.Int())
	newState := &pb.NewState{
		Id:               &hashStr,
		SchemaId:         stateSchema.Id,
		StateDataJson:    string(tokenJSON),
		DistributionList: []string{owner},
	}

	if useNullifiers {
		newState.NullifierSpecs = []*pb.NullifierSpec{
			{
				Party:        owner,
				Algorithm:    getAlgoZetoSnarkBJJ(name),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
				PayloadType:  zetosignerapi.PAYLOAD_DOMAIN_ZETO_NULLIFIER,
			},
		}
	}
	return newState, nil
}

func prepareOutputsForTransfer(ctx context.Context, useNullifiers bool, params []*types.NonFungibleTransferParamEntry, resolvedVerifiers []*pb.ResolvedVerifier, stateSchema *pb.StateSchema, name string) ([]*types.ZetoNFToken, []*pb.NewState, error) {
	var tokens []*types.ZetoNFToken
	var newStates []*pb.NewState
	for _, param := range params {
		resolvedRecipient := findVerifierFunc(param.To, getAlgoZetoSnarkBJJ(name), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, resolvedVerifiers)
		if resolvedRecipient == nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, param.To)
		}
		recipientKey, err := common.LoadBabyJubKey([]byte(resolvedRecipient.Verifier))
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}

		tokenID := param.TokenID
		if tokenID.NilOrZero() { // generate tokenID only when empty (this can happen when minting a new token)
			r, err := Rand256()
			if err != nil {
				return nil, nil, i18n.NewError(ctx, msgs.MsgErrorGenerateRandomNumber, err)
			}
			tokenID = (*pldtypes.HexUint256)(r)
		}

		newToken := types.NewZetoNFToken(tokenID, param.URI, recipientKey, NewSalt())

		newState, err := makeNewState(ctx, stateSchema, useNullifiers, newToken, name, param.To)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorCreateNewState, err)
		}
		tokens = append(tokens, newToken)
		newStates = append(newStates, newState)
	}
	return tokens, newStates, nil
}

func findAvailableStates(ctx context.Context, callbacks plugintk.DomainCallbacks, stateSchema *pb.StateSchema, useNullifiers bool, stateQueryContext, query string) ([]*pb.StoredState, error) {
	req := &pb.FindAvailableStatesRequest{
		StateQueryContext: stateQueryContext,
		SchemaId:          stateSchema.Id,
		QueryJson:         query,
		UseNullifiers:     &useNullifiers,
	}
	res, err := callbacks.FindAvailableStates(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.States, nil
}

func processTokens(ctx context.Context, tokens []*types.ZetoNFToken) ([]string, []string, []string, []string, []string, error) {
	tokenLen := len(tokens)
	if tokenLen == 0 {
		return nil, nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorNoTokensForTransfer)
	}

	commitments := make([]string, tokenLen)
	tokenURIs := make([]string, tokenLen)
	tokenIDs := make([]string, tokenLen)
	salts := make([]string, tokenLen)
	owners := make([]string, tokenLen)

	for i := 0; i < tokenLen; i++ {

		hash, err := tokens[i].Hash(ctx)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}

		commitments[i] = hash.Int().Text(16)
		salts[i] = tokens[i].Salt.Int().Text(16)

		tokenURIs[i] = tokens[i].URI

		owners[i] = tokens[i].Owner.HexString()
		tokenIDs[i] = tokens[i].TokenID.String()
	}

	return commitments, salts, tokenURIs, tokenIDs, owners, nil
}
