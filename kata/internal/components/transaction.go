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

package components

import (
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type TransactionInputs struct {
	Domain   string
	From     string
	To       *types.EthAddress
	Function *abi.Entry
	Inputs   types.RawJSON
}

type TransactionPreAssembly struct {
	TransactionSpecification *prototk.TransactionSpecification
	RequiredVerifiers        []*prototk.ResolveVerifierRequest
	Verifiers                []*prototk.ResolvedVerifier
}

type FullState struct {
	ID     types.Bytes32
	Schema types.Bytes32
	Data   types.RawJSON
}

type TransactionPostAssembly struct {
	AssemblyResult        prototk.AssembleTransactionResponse_Result
	OutputStatesPotential []*prototk.NewState // the raw result of assembly, before sequence allocation
	InputStates           []*FullState
	ReadStates            []*FullState
	OutputStates          []*FullState
	AttestationPlan       []*prototk.AttestationRequest
	Signatures            []*prototk.AttestationResult
	Endorsements          []*prototk.AttestationResult
	AllAttestations       []*prototk.AttestationResult
}

// PrivateTransaction is the critical exchange object between the engine and the domain manager,
// as it hops between the states in the state machine (on multiple paladin nodes) to reach
// a state that it can successfully (and anonymously) submitted it to the blockchain.
//
// TODO: Struct or interface?
type PrivateTransaction struct {
	ID uuid.UUID // TODO: == idempotency key?

	// INPUTS: Items that come in from the submitter of the transaction
	Inputs *TransactionInputs

	// ASSEMBLY PHASE: Items that get added to the transaction as it goes on its journey through
	// assembly, signing and endorsement (possibly going back through the journey many times)
	PreAssembly  *TransactionPreAssembly  // the bit of the assembly phase state that can be retained across re-assembly
	PostAssembly *TransactionPostAssembly // the bit of the assembly phase state that must be completely discarded on re-assembly

	// DISPATCH PHASE: Once the transaction has reached sufficient confidence of success,
	// we move on to submitting it to the blockchain.
	PreparedTransaction *prototk.BaseLedgerTransaction
}

// PrivateContractDeploy is a simpler transaction type that constructs new private smart contract instances
// within a domain, according to the constructor specification of that domain.
type PrivateContractDeploy struct {

	// INPUTS: Items that come in from the submitter of the transaction to send to the constructor
	ID     uuid.UUID // TODO: == idempotency key?
	Domain string
	Inputs types.RawJSON

	// ASSEMBLY PHASE
	TransactionSpecification *prototk.DeployTransactionSpecification
	RequiredVerifiers        []*prototk.ResolveVerifierRequest
	Verifiers                []*prototk.ResolvedVerifier

	// DISPATCH PHASE
	Signer            string
	InvokeTransaction *prototk.BaseLedgerTransaction
	DeployTransaction *prototk.BaseLedgerDeployTransaction
}
