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

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type tbPrivateSmartContract struct {
	tb      *testbed
	data    ethtypes.HexBytes0xPrefix
	domain  *tbDomain
	address *ethtypes.Address0xHex
}

func (psc *tbPrivateSmartContract) validateInvoke(ctx context.Context, invocation *types.PrivateContractInvoke) (*uuid.UUID, *proto.TransactionSpecification, error) {

	if invocation.From == "" {
		return nil, nil, fmt.Errorf("no from address specified for transaction")
	}

	functionABI := &invocation.Function

	confirmedBlockHeight, err := psc.tb.blockindexer.GetConfirmedBlockHeight(ctx)
	if err != nil {
		return nil, nil, err
	}

	functionParams, err := functionABI.Inputs.ParseJSONCtx(ctx, invocation.Inputs)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid parameters for function %s: %s", functionABI.SolString(), err)
	}

	functionABIJSON, _ := json.Marshal(functionABI)
	functionParamsJSON, _ := types.StandardABISerializer().SerializeJSONCtx(ctx, functionParams)

	txID := uuid.New()
	return &txID, &proto.TransactionSpecification{
		TransactionId:      uuidToHexBytes32(txID).String(),
		From:               invocation.From,
		ContractAddress:    psc.address.String(),
		ContractConfig:     psc.data,
		FunctionAbiJson:    string(functionABIJSON),
		FunctionSignature:  functionABI.String(),
		FunctionParamsJson: string(functionParamsJSON),
		BaseBlock:          int64(confirmedBlockHeight),
	}, nil
}

func (psc *tbPrivateSmartContract) gatherSignatures(ctx context.Context, requests []*proto.AttestationRequest) ([]*proto.AttestationResult, error) {
	tb := psc.tb
	attestations := []*proto.AttestationResult{}
	for _, ar := range requests {
		if ar.AttestationType == proto.AttestationType_SIGN {
			for _, partyName := range ar.Parties {
				keyHandle, verifier, err := tb.keyMgr.ResolveKey(ctx, partyName, ar.Algorithm)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve local signer for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				signaturePayload, err := tb.keyMgr.Sign(ctx, &proto.SignRequest{
					KeyHandle: keyHandle,
					Algorithm: ar.Algorithm,
					Payload:   ar.Payload,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, ar.Algorithm, err)
				}
				attestations = append(attestations, &proto.AttestationResult{
					Name:            ar.Name,
					AttestationType: ar.AttestationType,
					Verifier: &proto.ResolvedVerifier{
						Lookup:    partyName,
						Algorithm: ar.Algorithm,
						Verifier:  verifier,
					},
					Payload: signaturePayload.Payload,
				})
			}
		}
	}
	return attestations, nil
}

func (psc *tbPrivateSmartContract) restoreStateOrder(originalList []*proto.StateRef, queriedStates []*statestore.State) ([]*statestore.State, error) {
	orderedList := make([]*statestore.State, len(originalList))
	for i, sr := range originalList {
		found := false
		for _, s := range queriedStates {
			if s.Schema.String() == sr.SchemaId && s.Hash.String() == sr.HashId {
				orderedList[i] = s
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("State %d with schema=%s id=%s not found", i, sr.SchemaId, sr.HashId)
		}
	}
	return orderedList, nil
}

func (psc *tbPrivateSmartContract) toEndorsableList(states []*statestore.State) []*proto.EndorsableState {
	endorsableList := make([]*proto.EndorsableState, len(states))
	for i, input := range states {
		endorsableList[i] = &proto.EndorsableState{
			HashId:        input.Hash.String(),
			SchemaId:      input.Schema.String(),
			StateDataJson: string(input.Data),
		}
	}
	return endorsableList
}

func (psc *tbPrivateSmartContract) gatherEndorsements(ctx context.Context,
	txSpec *proto.TransactionSpecification,
	signatures []*proto.AttestationResult,
	spentStates []*proto.StateRef,
	newStates []*statestore.State,
	resolvedVerifiers []*proto.ResolvedVerifier,
	attestationRequests []*proto.AttestationRequest,
) (string, []*statestore.State, []*proto.AttestationResult, error) {
	domain := psc.domain

	inputIDsBySchema := map[string][]types.RawJSON{}
	for _, s := range spentStates {
		inputIDsBySchema[s.SchemaId] = append(inputIDsBySchema[s.SchemaId], types.JSONString(s.HashId))
	}

	// We have to gather all the input states for the endorsement
	// No optimization here for the fact we might have them in memory in the test bed domain, as in the real
	// distributed transaction management case this would be most likely to run on a different node
	// that needs to be certain it also sees the states as available.
	endorserSubmitConstraint := ""
	inputStates := []*statestore.State{}
	err := psc.tb.stateStore.RunInDomainContext(domain.name, func(ctx context.Context, dsi statestore.DomainStateInterface) error {
		for schemaID, stateIDs := range inputIDsBySchema {
			statesForSchema, err := dsi.FindAvailableStates(schemaID, &filters.QueryJSON{
				Statements: filters.Statements{
					Ops: filters.Ops{
						In: []*filters.OpMultiVal{
							{Op: filters.Op{Field: "id"}, Values: stateIDs},
						},
					},
				},
			})
			if err != nil {
				return err
			}
			inputStates = append(inputStates, statesForSchema...)
		}
		return nil
	})
	if err == nil {
		inputStates, err = psc.restoreStateOrder(spentStates, inputStates)
	}
	if err != nil {
		return "", nil, nil, err
	}

	attestations := []*proto.AttestationResult{}
	tb := psc.tb
	for _, ar := range attestationRequests {
		if ar.AttestationType == proto.AttestationType_SIGN {
			for _, partyName := range ar.Parties {
				// Look up the endorser
				keyHandle, verifier, err := tb.keyMgr.ResolveKey(ctx, partyName, ar.Algorithm)
				if err != nil {
					return "", nil, nil, fmt.Errorf("failed to resolve (local in testbed case) endorser for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				// Build the input
				endorseReq := &proto.EndorseTransactionRequest{
					Transaction:       txSpec,
					ResolvedVerifiers: resolvedVerifiers,
					Inputs:            psc.toEndorsableList(inputStates),
					Outputs:           psc.toEndorsableList(newStates),
					Signatures:        signatures,
					EndorsementVerifier: &proto.ResolvedVerifier{
						Lookup:    partyName,
						Algorithm: ar.Algorithm,
						Verifier:  verifier,
					},
				}
				// Invoke the domain
				var endorseRes *proto.EndorseTransactionResponse
				if err := syncExchange(ctx, tb, tb.destToDomain, tb.destFromDomain, endorseReq, &endorseRes); err != nil {
					return "", nil, nil, fmt.Errorf("endorsement from %q failed: %s", partyName, err)
				}
				result := &proto.AttestationResult{
					Name:            ar.Name,
					AttestationType: ar.AttestationType,
					Verifier: &proto.ResolvedVerifier{
						Lookup:    partyName,
						Algorithm: ar.Algorithm,
						Verifier:  verifier,
					},
				}
				switch endorseRes.EndorsementResult {
				case proto.EndorseTransactionResponse_REVERT:
					revertReason := "(no revert reason)"
					if endorseRes.RevertReason != nil {
						revertReason = *endorseRes.RevertReason
					}
					return "", nil, nil, fmt.Errorf("reverted: %s", revertReason)
				case proto.EndorseTransactionResponse_SIGN:
					// Build the signature
					signaturePayload, err := tb.keyMgr.Sign(ctx, &proto.SignRequest{
						KeyHandle: keyHandle,
						Algorithm: ar.Algorithm,
						Payload:   endorseRes.Payload,
					})
					if err != nil {
						return "", nil, nil, fmt.Errorf("failed to endorse for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, ar.Algorithm, err)
					}
					result.Payload = signaturePayload.Payload
				case proto.EndorseTransactionResponse_ENDORSER_SUBMIT:
					if endorserSubmitConstraint != "" {
						return "", nil, nil, fmt.Errorf("duplicate ENDORSER_SUBMIT responses from %s and %s", endorserSubmitConstraint, partyName)
					}
					endorserSubmitConstraint = partyName
				}
				attestations = append(attestations, result)
			}
		}
	}
	return endorserSubmitConstraint, inputStates, attestations, nil
}

func (psc *tbPrivateSmartContract) determineSubmitterIdentity(txSpec *proto.TransactionSpecification, endorserSubmitConstraint string, endorsements []*proto.AttestationResult) (string, error) {
	if endorserSubmitConstraint != "" {
		return endorserSubmitConstraint, nil
	}
	switch psc.domain.config.BaseLedgerSubmitConfig.SubmitMode {
	case proto.BaseLedgerSubmitConfig_ONE_TIME_USE_KEYS:
		return psc.domain.config.BaseLedgerSubmitConfig.OneTimeUsePrefix + txSpec.TransactionId, nil
	case proto.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION:
		for _, ar := range endorsements {
			if ar.AttestationType == proto.AttestationType_ENDORSE {
				return ar.Verifier.Lookup, nil
			}
		}
		return "", fmt.Errorf("endorser submission requested by domain %s config, but no endorsements were obtained", psc.domain.name)
	default:
		return "", fmt.Errorf("unsupported base ledger submit config: %s", psc.domain.config.BaseLedgerSubmitConfig.SubmitMode)
	}
}

func (psc *tbPrivateSmartContract) validateAndWriteStates(seq uuid.UUID, newStates []*proto.NewState) ([]*statestore.State, []*proto.StateRef, error) {

	domain := psc.domain
	newStatesToWrite := make([]*statestore.NewState, len(newStates))
	for i, s := range newStates {
		schema := domain.schemasByID[s.SchemaId]
		if schema == nil {
			schema = domain.schemasBySignature[s.SchemaId]
		}
		if schema == nil {
			return nil, nil, fmt.Errorf("unknown schema %s", s.SchemaId)
		}
		newStatesToWrite[i] = &statestore.NewState{
			SchemaID: schema.ID(),
			Data:     types.RawJSON(s.StateDataJson),
		}
	}

	var states []*statestore.State
	err := psc.tb.stateStore.RunInDomainContext(domain.name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		states, err = dsi.CreateNewStates(seq, newStatesToWrite)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	newStateIDs := make([]*proto.StateRef, len(states))
	for i, ws := range states {
		newStateIDs[i] = &proto.StateRef{
			SchemaId: ws.Schema.String(),
			HashId:   ws.Hash.String(),
		}
	}
	return states, newStateIDs, nil

}
