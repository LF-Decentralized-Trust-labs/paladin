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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

func (tb *testbed) ExecTransactionSync(ctx context.Context, tx *pldapi.TransactionInput) (receipt *pldapi.TransactionReceipt, err error) {
	txm := tb.c.TxManager()
	var txIDs []uuid.UUID
	err = tb.Components().Persistence().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		txIDs, err = tb.c.TxManager().SendTransactions(ctx, dbTX, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	txID := txIDs[0]

	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		<-ticker.C
		receipt, err = txm.GetTransactionReceiptByID(ctx, txID)
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
		data, err = pldtypes.StandardABISerializer().SerializeJSONCtx(ctx, txInstruction.Inputs)
		if err != nil {
			return nil, err
		}
	}
	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: signer,
			Data: data,
		},
		ABI:      abi.ABI{txInstruction.ConstructorABI},
		Bytecode: pldtypes.HexBytes(txInstruction.Bytecode),
	}
	return tb.ExecTransactionSync(ctx, tx)
}

func (tb *testbed) execBaseLedgerTransaction(ctx context.Context, signer string, txInstruction *components.EthTransaction) (receipt *pldapi.TransactionReceipt, err error) {
	var data []byte
	if txInstruction.Inputs != nil {
		data, err = pldtypes.StandardABISerializer().SerializeJSONCtx(ctx, txInstruction.Inputs)
		if err != nil {
			return nil, err
		}
	}
	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
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

func (tb *testbed) ExecBaseLedgerCall(ctx context.Context, result any, tx *pldapi.TransactionCall) error {
	return tb.Components().TxManager().CallTransaction(ctx, tb.c.Persistence().NOTX(), result, tx)
}

func (tb *testbed) ResolveKey(ctx context.Context, fqLookup, algorithm, verifierType string) (resolvedKey *pldapi.KeyMappingAndVerifier, err error) {
	keyMgr := tb.c.KeyManager()
	unqualifiedLookup, err := pldtypes.PrivateIdentityLocator(fqLookup).Identity(ctx)
	if err == nil {
		resolvedKey, err = keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, algorithm, verifierType)
	}
	return resolvedKey, err
}

func (tb *testbed) gatherSignatures(ctx context.Context, tx *testbedTransaction) error {
	tx.ptx.PostAssembly.Signatures = []*prototk.AttestationResult{}
	for _, ar := range tx.ptx.PostAssembly.AttestationPlan {
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
				tx.ptx.PostAssembly.Signatures = append(tx.ptx.PostAssembly.Signatures, &prototk.AttestationResult{
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

func (tb *testbed) writeNullifiersToContext(dCtx components.DomainContext, tx *components.PrivateTransaction) error {

	distributions, err := tb.c.PrivateTxManager().BuildStateDistributions(tb.ctx, tx)
	if err != nil {
		return err
	}

	if len(distributions.Remote) > 0 {
		log.L(tb.ctx).Errorf("States for remote nodes: %+v", distributions.Remote)
		return fmt.Errorf("testbed does not support states for remote nodes")
	}

	nullifiers, err := tb.c.PrivateTxManager().BuildNullifiers(tb.ctx, distributions.Local)
	if err != nil {
		return err
	}

	return dCtx.UpsertNullifiers(nullifiers...)

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

func (tb *testbed) gatherEndorsements(dCtx components.DomainContext, tx *testbedTransaction) error {

	keyMgr := tb.c.KeyManager()
	attestations := []*prototk.AttestationResult{}
	for _, ar := range tx.ptx.PostAssembly.AttestationPlan {
		if ar.AttestationType == prototk.AttestationType_ENDORSE {
			for _, partyName := range ar.Parties {
				// Look up the endorser
				resolvedKey, err := tb.ResolveKey(dCtx.Ctx(), partyName, ar.Algorithm, ar.VerifierType)
				if err != nil {
					return fmt.Errorf("failed to resolve (local in testbed case) endorser for %s (algorithm=%s): %s", partyName, ar.Algorithm, err)
				}
				// Invoke the domain
				endorseRes, err := tx.psc.EndorseTransaction(dCtx, tb.c.Persistence().NOTX(), &components.PrivateTransactionEndorseRequest{
					TransactionSpecification: tx.ptx.PreAssembly.TransactionSpecification,
					Verifiers:                tx.ptx.PreAssembly.Verifiers,
					Signatures:               tx.ptx.PostAssembly.Signatures,
					InputStates:              toEndorsableList(tx.ptx.PostAssembly.InputStates),
					ReadStates:               toEndorsableList(tx.ptx.PostAssembly.ReadStates),
					OutputStates:             toEndorsableList(tx.ptx.PostAssembly.OutputStates),
					InfoStates:               toEndorsableList(tx.ptx.PostAssembly.InfoStates),
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
	tx.ptx.PostAssembly.Endorsements = attestations
	return nil
}

//nolint:unused // May be used in future
func mustParseBuildABI(buildJSON []byte) abi.ABI {
	var buildParsed map[string]pldtypes.RawJSON
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

//nolint:unused // May be used in future
func mustParseBuildBytecode(buildJSON []byte) pldtypes.HexBytes {
	var buildParsed map[string]pldtypes.RawJSON
	var byteCode pldtypes.HexBytes
	err := json.Unmarshal(buildJSON, &buildParsed)
	if err == nil {
		err = json.Unmarshal(buildParsed["bytecode"], &byteCode)
	}
	if err != nil {
		panic(err)
	}
	return byteCode
}

//nolint:unused // May be used in future
func mustParseABIEntry(abiEntryJSON string) *abi.Entry {
	var abiEntry abi.Entry
	err := json.Unmarshal([]byte(abiEntryJSON), &abiEntry)
	if err != nil {
		panic(err)
	}
	return &abiEntry
}
