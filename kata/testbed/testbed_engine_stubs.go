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

	_ "embed"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

//go:embed abis/IPaladinContract_V0.json
var iPaladinContractABIJSON []byte

var iPaladinContractABI = mustParseBuildABI(iPaladinContractABIJSON)
var iPaladinNewSmartContract_V0_Signature = mustEventSignatureHash(iPaladinContractABI, "PaladinNewSmartContract_V0")

type iPaladinNewSmartContract_V0_Type struct {
	Domain *ethtypes.Address0xHex    `json:"domain"`
	TXID   ethtypes.HexBytes0xPrefix `json:"txId"`
	Data   ethtypes.HexBytes0xPrefix `json:"data"`
}

func (tb *testbed) execBaseLedgerDeployTransaction(ctx context.Context, abi abi.ABI, signer string, txInstruction *prototk.BaseLedgerDeployTransaction) error {

	var abiFunc ethclient.ABIFunctionClient
	ec := tb.components.EthClientFactory().HTTPClient()
	abiClient, err := ec.ABI(ctx, abi)
	if err == nil {
		abiFunc, err = abiClient.Constructor(ctx, txInstruction.Bytecode)
	}
	if err != nil {
		return fmt.Errorf("failed to process ABI constructor: %s", err)
	}

	// Send the transaction
	txHash, err := abiFunc.R(ctx).
		Signer(signer).
		Input(txInstruction.ParamsJson).
		SignAndSend()
	if err == nil {
		_, err = tb.components.BlockIndexer().WaitForTransaction(ctx, txHash.String())
	}
	if err != nil {
		return fmt.Errorf("failed to send base deploy ledger transaction: %s", err)
	}
	return nil
}

func (tb *testbed) execBaseLedgerTransaction(ctx context.Context, abi abi.ABI, to *ethtypes.Address0xHex, signer string, txInstruction *prototk.BaseLedgerTransaction) error {

	var abiFunc ethclient.ABIFunctionClient
	ec := tb.components.EthClientFactory().HTTPClient()
	abiClient, err := ec.ABI(ctx, abi)
	if err == nil {
		abiFunc, err = abiClient.Function(ctx, txInstruction.FunctionName)
	}
	if err != nil {
		return fmt.Errorf("function %q does not exist on base ledger ABI: %s", txInstruction.FunctionName, err)
	}

	// Send the transaction
	txHash, err := abiFunc.R(ctx).
		Signer(signer).
		To(to).
		Input(txInstruction.ParamsJson).
		SignAndSend()
	if err == nil {
		_, err = tb.components.BlockIndexer().WaitForTransaction(ctx, txHash.String())
	}
	if err != nil {
		return fmt.Errorf("failed to send base ledger transaction: %s", err)
	}
	return nil
}

func (tb *testbed) gatherSignatures(ctx context.Context, requests []*prototk.AttestationRequest) ([]*prototk.AttestationResult, error) {
	attestations := []*prototk.AttestationResult{}
	for _, ar := range requests {
		if ar.AttestationType == prototk.AttestationType_SIGN {
			for _, partyName := range ar.Parties {
				keyHandle, verifier, err := tb.components.KeyManager().ResolveKey(ctx, partyName, ar.Algorithm)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve local signer for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				signaturePayload, err := tb.components.KeyManager().Sign(ctx, &proto.SignRequest{
					KeyHandle: keyHandle,
					Algorithm: ar.Algorithm,
					Payload:   ar.Payload,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, ar.Algorithm, err)
				}
				attestations = append(attestations, &prototk.AttestationResult{
					Name:            ar.Name,
					AttestationType: ar.AttestationType,
					Verifier: &prototk.ResolvedVerifier{
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

func (tb *testbed) gatherEndorsements(ctx context.Context, psc components.DomainSmartContract, tx *components.PrivateTransaction) (string, error) {

	keyMgr := tb.components.KeyManager()
	attestations := []*prototk.AttestationResult{}
	endorserSubmitConstraint := ""
	for _, ar := range tx.PostAssembly.AttestationPlan {
		if ar.AttestationType == prototk.AttestationType_ENDORSE {
			for _, partyName := range ar.Parties {
				// Look up the endorser
				keyHandle, verifier, err := keyMgr.ResolveKey(ctx, partyName, ar.Algorithm)
				if err != nil {
					return "", fmt.Errorf("failed to resolve (local in testbed case) endorser for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				// Invoke the domain
				endorseRes, err := psc.EndorseTransaction(ctx, tx, &prototk.ResolvedVerifier{
					Lookup:    partyName,
					Algorithm: ar.Algorithm,
					Verifier:  verifier,
				})
				result := &prototk.AttestationResult{
					Name:            ar.Name,
					AttestationType: ar.AttestationType,
					Verifier:        endorseRes.Endorser,
				}
				switch endorseRes.Result {
				case prototk.EndorseTransactionResponse_REVERT:
					revertReason := "(no revert reason)"
					if endorseRes.RevertReason != nil {
						revertReason = *endorseRes.RevertReason
					}
					return "", fmt.Errorf("reverted: %s", revertReason)
				case prototk.EndorseTransactionResponse_SIGN:
					// Build the signature
					signaturePayload, err := keyMgr.Sign(ctx, &proto.SignRequest{
						KeyHandle: keyHandle,
						Algorithm: ar.Algorithm,
						Payload:   endorseRes.Payload,
					})
					if err != nil {
						return "", fmt.Errorf("failed to endorse for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, ar.Algorithm, err)
					}
					result.Payload = signaturePayload.Payload
				case prototk.EndorseTransactionResponse_ENDORSER_SUBMIT:
					if endorserSubmitConstraint != "" {
						return "", fmt.Errorf("duplicate ENDORSER_SUBMIT responses from %s and %s", endorserSubmitConstraint, partyName)
					}
					endorserSubmitConstraint = partyName
				}
				attestations = append(attestations, result)
			}
		}
	}
	tx.PostAssembly.Endorsements = attestations
	return endorserSubmitConstraint, nil
}

func (tb *testbed) determineSubmitterIdentity(psc components.DomainSmartContract, tx *components.PrivateTransaction, endorserSubmitConstraint string) (string, error) {
	if endorserSubmitConstraint != "" {
		return endorserSubmitConstraint, nil
	}
	switch psc.Domain().Configuration().BaseLedgerSubmitConfig.SubmitMode {
	case prototk.BaseLedgerSubmitConfig_ONE_TIME_USE_KEYS:
		return psc.Domain().Configuration().BaseLedgerSubmitConfig.OneTimeUsePrefix + tx.ID.String(), nil
	case prototk.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION:
		for _, ar := range tx.PostAssembly.Endorsements {
			if ar.AttestationType == prototk.AttestationType_ENDORSE {
				return ar.Verifier.Lookup, nil
			}
		}
		return "", fmt.Errorf("endorser submission requested by domain %s config, but no endorsements were obtained", psc.Domain().Name())
	default:
		return "", fmt.Errorf("unsupported base ledger submit config: %s", psc.Domain().Configuration().BaseLedgerSubmitConfig.SubmitMode)
	}
}

func mustParseBuildABI(buildJSON []byte) abi.ABI {
	var buildParsed map[string]types.RawJSON
	var buildABI abi.ABI
	err := json.Unmarshal(buildJSON, &buildParsed)
	if err == nil {
		err = json.Unmarshal(buildParsed["abi"], &buildABI)
	}
	if err != nil {
		panic(err)
	}
	return buildABI
}

func mustParseBuildBytecode(buildJSON []byte) ethtypes.HexBytes0xPrefix {
	var buildParsed map[string]types.RawJSON
	var byteCode ethtypes.HexBytes0xPrefix
	err := json.Unmarshal(buildJSON, &buildParsed)
	if err == nil {
		err = json.Unmarshal(buildParsed["bytecode"], &byteCode)
	}
	if err != nil {
		panic(err)
	}
	return byteCode
}

func mustParseABIEntry(abiEntryJSON string) *abi.Entry {
	var abiEntry abi.Entry
	err := json.Unmarshal([]byte(abiEntryJSON), &abiEntry)
	if err != nil {
		panic(err)
	}
	return &abiEntry
}

func mustEventSignatureHash(a abi.ABI, eventName string) ethtypes.HexBytes0xPrefix {
	ev := a.Events()[eventName]
	if ev == nil {
		panic("missing event " + eventName)
	}
	sig, err := ev.SignatureHash()
	if err != nil {
		panic(err)
	}
	return sig
}
