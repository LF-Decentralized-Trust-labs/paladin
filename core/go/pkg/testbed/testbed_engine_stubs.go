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
	"time"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (tb *testbed) ExecTransactionSync(ctx context.Context, tx *pldapi.TransactionInput) (receipt *pldapi.TransactionReceipt, err error) {
	txm := tb.c.TxManager()
	txID, err := tb.c.TxManager().SendTransaction(ctx, tx)
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		<-ticker.C
		receipt, err = txm.GetTransactionReceiptByID(ctx, *txID)
		if err != nil {
			return nil, fmt.Errorf("error checking for transaction receipt: %s", err)
		}
		if receipt != nil {
			break
		}
	}
	if !receipt.Success {
		return nil, fmt.Errorf("transaction failed: %s", receipt.FailureMessage)
	}
	return receipt, nil
}

func (tb *testbed) execBaseLedgerDeployTransaction(ctx context.Context, signer string, txInstruction *components.EthDeployTransaction) (receipt *pldapi.TransactionReceipt, err error) {
	var data []byte
	if txInstruction.Inputs != nil {
		data, err = tktypes.StandardABISerializer().SerializeJSONCtx(ctx, txInstruction.Inputs)
		if err != nil {
			return nil, err
		}
	}
	tx := &pldapi.TransactionInput{
		Transaction: pldapi.Transaction{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: signer,
			Data: data,
		},
		ABI:      abi.ABI{txInstruction.ConstructorABI},
		Bytecode: tktypes.HexBytes(txInstruction.Bytecode),
	}
	return tb.ExecTransactionSync(ctx, tx)
}

func (tb *testbed) execBaseLedgerTransaction(ctx context.Context, signer string, txInstruction *components.EthTransaction) (receipt *pldapi.TransactionReceipt, err error) {
	var data []byte
	if txInstruction.Inputs != nil {
		data, err = tktypes.StandardABISerializer().SerializeJSONCtx(ctx, txInstruction.Inputs)
		if err != nil {
			return nil, err
		}
	}
	tx := &pldapi.TransactionInput{
		Transaction: pldapi.Transaction{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: txInstruction.FunctionABI.String(),
			From:     signer,
			To:       &txInstruction.To,
			Data:     data,
		},
		ABI: abi.ABI{txInstruction.FunctionABI},
	}
	return tb.ExecTransactionSync(ctx, tx)
}

func (tb *testbed) ExecBaseLedgerCall(ctx context.Context, result any, tx *pldapi.TransactionInput) error {
	return tb.Components().TxManager().CallTransaction(ctx, result, tx)
}

func (tb *testbed) ResolveKey(ctx context.Context, fqLookup, algorithm, verifierType string) (resolvedKey *pldapi.KeyMappingAndVerifier, err error) {
	keyMgr := tb.c.KeyManager()
	unqualifiedLookup, err := tktypes.PrivateIdentityLocator(fqLookup).Identity(ctx)
	if err == nil {
		resolvedKey, err = keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, algorithm, verifierType)
	}
	return resolvedKey, err
}

func (tb *testbed) gatherSignatures(ctx context.Context, tx *components.PrivateTransaction) error {
	tx.PostAssembly.Signatures = []*prototk.AttestationResult{}
	for _, ar := range tx.PostAssembly.AttestationPlan {
		if ar.AttestationType == prototk.AttestationType_SIGN {
			for _, partyName := range ar.Parties {
				resolvedKey, err := tb.ResolveKey(ctx, partyName, ar.Algorithm, ar.VerifierType)
				if err != nil {
					return fmt.Errorf("failed to resolve local signer for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				signaturePayload, err := tb.c.KeyManager().Sign(ctx, resolvedKey, ar.PayloadType, ar.Payload)
				if err != nil {
					return fmt.Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", partyName, resolvedKey.Verifier.Verifier, ar.Algorithm, err)
				}
				tx.PostAssembly.Signatures = append(tx.PostAssembly.Signatures, &prototk.AttestationResult{
					Name:            ar.Name,
					AttestationType: ar.AttestationType,
					Verifier: &prototk.ResolvedVerifier{
						Lookup:       partyName,
						Algorithm:    ar.Algorithm,
						Verifier:     resolvedKey.Verifier.Verifier,
						VerifierType: ar.VerifierType,
					},
					Payload:     signaturePayload,
					PayloadType: &ar.PayloadType,
				})
			}
		}
	}
	return nil
}

func toEndorsableList(states []*components.FullState) []*prototk.EndorsableState {
	endorsableList := make([]*prototk.EndorsableState, len(states))
	for i, input := range states {
		endorsableList[i] = &prototk.EndorsableState{
			Id:            input.ID.String(),
			SchemaId:      input.Schema.String(),
			StateDataJson: string(input.Data),
		}
	}
	return endorsableList
}

func (tb *testbed) gatherEndorsements(dCtx components.DomainContext, psc components.DomainSmartContract, tx *components.PrivateTransaction) error {

	keyMgr := tb.c.KeyManager()
	attestations := []*prototk.AttestationResult{}
	for _, ar := range tx.PostAssembly.AttestationPlan {
		if ar.AttestationType == prototk.AttestationType_ENDORSE {
			for _, partyName := range ar.Parties {
				// Look up the endorser
				resolvedKey, err := tb.ResolveKey(dCtx.Ctx(), partyName, ar.Algorithm, ar.VerifierType)
				if err != nil {
					return fmt.Errorf("failed to resolve (local in testbed case) endorser for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				// Invoke the domain
				endorseRes, err := psc.EndorseTransaction(dCtx, &components.PrivateTransactionEndorseRequest{
					TransactionSpecification: tx.PreAssembly.TransactionSpecification,
					Verifiers:                tx.PreAssembly.Verifiers,
					Signatures:               tx.PostAssembly.Signatures,
					InputStates:              toEndorsableList(tx.PostAssembly.InputStates),
					ReadStates:               toEndorsableList(tx.PostAssembly.ReadStates),
					OutputStates:             toEndorsableList(tx.PostAssembly.OutputStates),
					Endorsement:              ar,
					Endorser: &prototk.ResolvedVerifier{
						Lookup:       partyName,
						Algorithm:    ar.Algorithm,
						Verifier:     resolvedKey.Verifier.Verifier,
						VerifierType: ar.VerifierType,
					},
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
					signaturePayload, err := keyMgr.Sign(dCtx.Ctx(), resolvedKey, ar.PayloadType, endorseRes.Payload)
					if err != nil {
						return fmt.Errorf("failed to endorse for party %s (verifier=%s,algorithm=%s): %s", partyName, resolvedKey.Verifier.Verifier, ar.Algorithm, err)
					}
					result.Payload = signaturePayload
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
	var buildParsed map[string]tktypes.RawJSON
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

func mustParseBuildBytecode(buildJSON []byte) tktypes.HexBytes {
	var buildParsed map[string]tktypes.RawJSON
	var byteCode tktypes.HexBytes
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
