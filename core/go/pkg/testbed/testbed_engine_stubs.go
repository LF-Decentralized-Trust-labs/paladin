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

package testbed

import (
	"context"
	"encoding/json"
	"fmt"

	_ "embed"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/core/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

func (tb *testbed) execBaseLedgerDeployTransaction(ctx context.Context, signer string, txInstruction *components.EthDeployTransaction) error {

	var abiFunc ethclient.ABIFunctionClient
	ec := tb.c.EthClientFactory().HTTPClient()
	abiFunc, err := ec.ABIConstructor(ctx, txInstruction.ConstructorABI, types.HexBytes(txInstruction.Bytecode))
	if err != nil {
		return err
	}

	// Send the transaction
	txHash, err := abiFunc.R(ctx).
		Signer(signer).
		Input(txInstruction.Inputs).
		SignAndSend()
	if err == nil {
		_, err = tb.c.BlockIndexer().WaitForTransaction(ctx, *txHash)
	}
	if err != nil {
		return fmt.Errorf("failed to send base deploy ledger transaction: %s", err)
	}
	return nil
}

func (tb *testbed) execBaseLedgerTransaction(ctx context.Context, signer string, txInstruction *components.EthTransaction) error {

	var abiFunc ethclient.ABIFunctionClient
	ec := tb.c.EthClientFactory().HTTPClient()
	abiFunc, err := ec.ABIFunction(ctx, txInstruction.FunctionABI)
	if err != nil {
		return err
	}

	// Send the transaction
	addr := ethtypes.Address0xHex(txInstruction.To)
	txHash, err := abiFunc.R(ctx).
		Signer(signer).
		To(&addr).
		Input(txInstruction.Inputs).
		SignAndSend()
	if err == nil {
		_, err = tb.c.BlockIndexer().WaitForTransaction(ctx, *txHash)
	}
	if err != nil {
		return fmt.Errorf("failed to send base ledger transaction: %s", err)
	}
	return nil
}

func (tb *testbed) gatherSignatures(ctx context.Context, tx *components.PrivateTransaction) error {
	tx.PostAssembly.Signatures = []*prototk.AttestationResult{}
	for _, ar := range tx.PostAssembly.AttestationPlan {
		if ar.AttestationType == prototk.AttestationType_SIGN {
			for _, partyName := range ar.Parties {
				keyHandle, verifier, err := tb.c.KeyManager().ResolveKey(ctx, partyName, ar.Algorithm)
				if err != nil {
					return fmt.Errorf("failed to resolve local signer for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				signaturePayload, err := tb.c.KeyManager().Sign(ctx, &proto.SignRequest{
					KeyHandle: keyHandle,
					Algorithm: ar.Algorithm,
					Payload:   ar.Payload,
				})
				if err != nil {
					return fmt.Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, ar.Algorithm, err)
				}
				tx.PostAssembly.Signatures = append(tx.PostAssembly.Signatures, &prototk.AttestationResult{
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
	return nil
}

func (tb *testbed) gatherEndorsements(ctx context.Context, psc components.DomainSmartContract, tx *components.PrivateTransaction) error {

	keyMgr := tb.c.KeyManager()
	attestations := []*prototk.AttestationResult{}
	for _, ar := range tx.PostAssembly.AttestationPlan {
		if ar.AttestationType == prototk.AttestationType_ENDORSE {
			for _, partyName := range ar.Parties {
				// Look up the endorser
				keyHandle, verifier, err := keyMgr.ResolveKey(ctx, partyName, ar.Algorithm)
				if err != nil {
					return fmt.Errorf("failed to resolve (local in testbed case) endorser for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				// Invoke the domain
				endorseRes, err := psc.EndorseTransaction(ctx, tx, ar, &prototk.ResolvedVerifier{
					Lookup:    partyName,
					Algorithm: ar.Algorithm,
					Verifier:  verifier,
				})
				if err != nil {
					return err
				}
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
					return fmt.Errorf("reverted: %s", revertReason)
				case prototk.EndorseTransactionResponse_SIGN:
					// Build the signature
					signaturePayload, err := keyMgr.Sign(ctx, &proto.SignRequest{
						KeyHandle: keyHandle,
						Algorithm: ar.Algorithm,
						Payload:   endorseRes.Payload,
					})
					if err != nil {
						return fmt.Errorf("failed to endorse for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, ar.Algorithm, err)
					}
					result.Payload = signaturePayload.Payload
				case prototk.EndorseTransactionResponse_ENDORSER_SUBMIT:
					result.Constraints = append(result.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
				}
				attestations = append(attestations, result)
			}
		}
	}
	tx.PostAssembly.Endorsements = attestations
	return nil
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

func mustParseBuildBytecode(buildJSON []byte) types.HexBytes {
	var buildParsed map[string]types.RawJSON
	var byteCode types.HexBytes
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
