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
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type TransactionInputs struct {
	Domain   string                                  `json:"domain"`
	From     string                                  `json:"from"`
	To       tktypes.EthAddress                      `json:"to"`
	Function *abi.Entry                              `json:"function"`
	Inputs   tktypes.RawJSON                         `json:"inputs"`
	Intent   prototk.TransactionSpecification_Intent `json:"intent"`
}

type TransactionPreAssembly struct {
	TransactionSpecification *prototk.TransactionSpecification `json:"transaction_specification"`
	RequiredVerifiers        []*prototk.ResolveVerifierRequest `json:"required_verifiers"`
	Verifiers                []*prototk.ResolvedVerifier       `json:"verifiers"`
}
type FullState struct {
	ID     tktypes.HexBytes `json:"id"`
	Schema tktypes.Bytes32  `json:"schema"`
	Data   tktypes.RawJSON  `json:"data"`
}

type EthTransaction struct {
	FunctionABI *abi.Entry
	To          tktypes.EthAddress
	Inputs      *abi.ComponentValue
}

type EthDeployTransaction struct {
	ConstructorABI *abi.Entry
	Bytecode       tktypes.HexBytes
	Inputs         *abi.ComponentValue
}

type TransactionPostAssembly struct {
	AssemblyResult        prototk.AssembleTransactionResponse_Result `json:"assembly_result"`
	OutputStatesPotential []*prototk.NewState                        `json:"output_states_potential"` // the raw result of assembly, before sequence allocation
	InputStates           []*FullState                               `json:"input_states"`
	ReadStates            []*FullState                               `json:"read_states"`
	OutputStates          []*FullState                               `json:"output_states"`
	AttestationPlan       []*prototk.AttestationRequest              `json:"attestation_plan"`
	Signatures            []*prototk.AttestationResult               `json:"signatures"`
	Endorsements          []*prototk.AttestationResult               `json:"endorsements"`
	ExtraData             *string                                    `json:"extra_data"`
}

// PrivateTransaction is the critical exchange object between the engine and the domain manager,
// as it hops between the states in the state machine (on multiple paladin nodes) to reach
// a state that it can successfully (and anonymously) submitted it to the blockchain.
type PrivateTransaction struct {
	ID uuid.UUID `json:"id"`

	// INPUTS: Items that come in from the submitter of the transaction
	Inputs *TransactionInputs `json:"inputs"`

	// ASSEMBLY PHASE: Items that get added to the transaction as it goes on its journey through
	// assembly, signing and endorsement (possibly going back through the journey many times)
	PreAssembly  *TransactionPreAssembly  `json:"pre_assembly"`  // the bit of the assembly phase state that can be retained across re-assembly
	PostAssembly *TransactionPostAssembly `json:"post_assembly"` // the bit of the assembly phase state that must be completely discarded on re-assembly

	// DISPATCH PHASE: Once the transaction has reached sufficient confidence of success, we move on to submission.
	// Each private transaction may result in a public transaction which should be submitted to the
	// base ledger, or another private transaction which should go around the transaction loop again.
	Signer                     string                                  `json:"signer"`
	PreparedPublicTransaction  *pldapi.TransactionInput                `json:"-"`
	PreparedPrivateTransaction *pldapi.TransactionInput                `json:"-"`
	PreparedTransactionIntent  prototk.TransactionSpecification_Intent `json:"-"`

	PublicTxOptions pldapi.PublicTxOptions `json:"-"`
}

// PrivateContractDeploy is a simpler transaction type that constructs new private smart contract instances
// within a domain, according to the constructor specification of that domain.
type PrivateContractDeploy struct {

	// INPUTS: Items that come in from the submitter of the transaction to send to the constructor
	ID     uuid.UUID
	Domain string
	Inputs tktypes.RawJSON

	// ASSEMBLY PHASE
	TransactionSpecification *prototk.DeployTransactionSpecification
	RequiredVerifiers        []*prototk.ResolveVerifierRequest
	Verifiers                []*prototk.ResolvedVerifier

	// DISPATCH PHASE
	Signer            string
	InvokeTransaction *EthTransaction
	DeployTransaction *EthDeployTransaction
}

type PrivateTransactionEndorseRequest struct {
	TransactionSpecification *prototk.TransactionSpecification
	Verifiers                []*prototk.ResolvedVerifier
	Signatures               []*prototk.AttestationResult
	InputStates              []*prototk.EndorsableState
	ReadStates               []*prototk.EndorsableState
	OutputStates             []*prototk.EndorsableState
	Endorsement              *prototk.AttestationRequest
	Endorser                 *prototk.ResolvedVerifier
}
