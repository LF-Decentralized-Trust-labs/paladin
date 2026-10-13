/*
 * Copyright © 2025 Kaleido, Inc.
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

package testutil

// This file contains utilities to abstract the complexities of the PrivateTransaction struct for use in tests to help make them more readable
// and to reduce the amount of boilerplate code needed to create a Transaction
import (
	"context"
	"fmt"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signpayloads"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
)

type identityForTesting struct {
	identity        string
	identityLocator string
	verifier        string
	keyHandle       string
}

type PrivateTransactionBuilderForTesting struct {
	id                         uuid.UUID
	originatorName             string
	originatorNode             string
	originator                 *identityForTesting
	domain                     string
	address                    pldtypes.EthAddress
	signerAddress              *pldtypes.EthAddress
	numberOfEndorsers          int
	numberOfEndorsements       int
	numberOfOutputStates       int
	inputStateIDs              []string
	readStateIDs               []string
	endorsers                  []*identityForTesting
	revertReason               *string
	chainedDependencies        []uuid.UUID
	preAssemblyOverride        *prototk.TransactionPreAssembly
	postAssemblyOverride       *components.TransactionPostAssembly
	preparedPrivateTransaction *pldapi.TransactionInput
	preparedPublicTransaction  *pldapi.TransactionInput
	signer                     *string
}

// useful for creating multiple transactions in a test, from the same originator
type PrivateTransactionBuilderListForTesting []*PrivateTransactionBuilderForTesting

func NewPrivateTransactionBuilderListForTesting(num int) PrivateTransactionBuilderListForTesting {

	builders := make(PrivateTransactionBuilderListForTesting, num)
	for i := 0; i < num; i++ {
		builders[i] = NewPrivateTransactionBuilderForTesting()
	}
	return builders
}

// Function BuildSparse creates a slice of PrivateTransactions with only the PreAssembly populated
func (b PrivateTransactionBuilderListForTesting) BuildSparse() []*components.PrivateTransaction {
	transactions := make([]*components.PrivateTransaction, len(b))
	for i, builder := range b {
		transactions[i] = builder.BuildSparse()
	}
	return transactions
}

func (b PrivateTransactionBuilderListForTesting) Build() []*components.PrivateTransaction {
	transactions := make([]*components.PrivateTransaction, len(b))
	for i, builder := range b {
		transactions[i] = builder.Build()
	}
	return transactions
}

func (b PrivateTransactionBuilderListForTesting) Address(address pldtypes.EthAddress) PrivateTransactionBuilderListForTesting {
	for _, builder := range b {
		builder.Address(address)
	}
	return b
}

// initialize originator identity locator e.g. name@node
func (b PrivateTransactionBuilderListForTesting) Originator(originator string) PrivateTransactionBuilderListForTesting {
	for _, builder := range b {
		builder.Originator(originator)
	}
	return b
}

func (b PrivateTransactionBuilderListForTesting) OriginatorName(originatorName string) PrivateTransactionBuilderListForTesting {
	for _, builder := range b {
		builder.OriginatorName(originatorName)
	}
	return b
}

func (b PrivateTransactionBuilderListForTesting) OriginatorNode(originatorNode string) PrivateTransactionBuilderListForTesting {
	for _, builder := range b {
		builder.OriginatorNode(originatorNode)
	}
	return b
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields
// use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewPrivateTransactionBuilderForTesting() *PrivateTransactionBuilderForTesting {

	builder := &PrivateTransactionBuilderForTesting{
		id:                   uuid.New(),
		domain:               "defaultDomain",
		address:              *pldtypes.RandAddress(),
		originatorName:       "sender",
		originatorNode:       "senderNode",
		signerAddress:        nil,
		numberOfEndorsers:    3,
		numberOfEndorsements: 0,
		numberOfOutputStates: 0,
	}

	return builder
}

func (b *PrivateTransactionBuilderForTesting) Domain(domain string) *PrivateTransactionBuilderForTesting {
	b.domain = domain
	return b
}

func (b *PrivateTransactionBuilderForTesting) Address(address pldtypes.EthAddress) *PrivateTransactionBuilderForTesting {
	b.address = address
	return b
}

func (b *PrivateTransactionBuilderForTesting) Originator(originator string) *PrivateTransactionBuilderForTesting {

	name, node, err := pldtypes.PrivateIdentityLocator(originator).Validate(context.Background(), "", false)
	if err != nil {
		//this is only used for testing so panic is fine
		panic(err)
	}
	b.originatorName = name
	b.originatorNode = node
	return b
}

func (b *PrivateTransactionBuilderForTesting) OriginatorName(originatorName string) *PrivateTransactionBuilderForTesting {
	b.originatorName = originatorName
	return b
}

func (b *PrivateTransactionBuilderForTesting) OriginatorNode(originatorNode string) *PrivateTransactionBuilderForTesting {
	b.originatorNode = originatorNode
	return b
}

func (b *PrivateTransactionBuilderForTesting) NumberOfRequiredEndorsers(num int) *PrivateTransactionBuilderForTesting {
	b.numberOfEndorsers = num
	return b
}

func (b *PrivateTransactionBuilderForTesting) NumberOfEndorsements(num int) *PrivateTransactionBuilderForTesting {
	b.numberOfEndorsements = num
	return b
}

func (b *PrivateTransactionBuilderForTesting) EndorsementComplete() *PrivateTransactionBuilderForTesting {
	b.numberOfEndorsements = b.numberOfEndorsers
	return b
}

func (b *PrivateTransactionBuilderForTesting) NumberOfOutputStates(num int) *PrivateTransactionBuilderForTesting {
	b.numberOfOutputStates = num
	return b
}

func (b *PrivateTransactionBuilderForTesting) InputStateIDs(stateIDs ...string) *PrivateTransactionBuilderForTesting {
	b.inputStateIDs = stateIDs
	return b
}

func (b *PrivateTransactionBuilderForTesting) ReadStateIDs(stateIDs ...string) *PrivateTransactionBuilderForTesting {
	b.readStateIDs = stateIDs
	return b
}

func (b *PrivateTransactionBuilderForTesting) ChainedDependencies(transactionIDs ...uuid.UUID) *PrivateTransactionBuilderForTesting {
	b.chainedDependencies = transactionIDs
	return b
}

func (b *PrivateTransactionBuilderForTesting) Reverts(revertReason string) *PrivateTransactionBuilderForTesting {
	b.revertReason = &revertReason
	return b
}

// ID sets the transaction ID used when Build() or BuildSparse() is called.
func (b *PrivateTransactionBuilderForTesting) ID(id uuid.UUID) *PrivateTransactionBuilderForTesting {
	b.id = id
	return b
}

// PreAssembly sets an optional override; when set, Build() uses this instead of BuildPreAssembly().
func (b *PrivateTransactionBuilderForTesting) PreAssembly(pa *prototk.TransactionPreAssembly) *PrivateTransactionBuilderForTesting {
	b.preAssemblyOverride = pa
	return b
}

// PostAssembly sets an optional override; when set, Build() uses this instead of BuildPostAssembly().
func (b *PrivateTransactionBuilderForTesting) PostAssembly(pa *components.TransactionPostAssembly) *PrivateTransactionBuilderForTesting {
	b.postAssemblyOverride = pa
	return b
}

// PreparedPrivateTransaction sets the prepared private transaction on the built PrivateTransaction.
func (b *PrivateTransactionBuilderForTesting) PreparedPrivateTransaction(tx *pldapi.TransactionInput) *PrivateTransactionBuilderForTesting {
	b.preparedPrivateTransaction = tx
	return b
}

// PreparedPublicTransaction sets the prepared public transaction on the built PrivateTransaction.
func (b *PrivateTransactionBuilderForTesting) PreparedPublicTransaction(tx *pldapi.TransactionInput) *PrivateTransactionBuilderForTesting {
	b.preparedPublicTransaction = tx
	return b
}

// Signer sets the signer identity on the built PrivateTransaction.
func (b *PrivateTransactionBuilderForTesting) Signer(signer string) *PrivateTransactionBuilderForTesting {
	b.signer = &signer
	return b
}

func (b *PrivateTransactionBuilderForTesting) GetEndorsementName(endorserIndex int) string {
	return fmt.Sprintf("endorse-%d", endorserIndex)
}

func (b *PrivateTransactionBuilderForTesting) GetEndorserIdentityLocator(endorserIndex int) string {
	return b.endorsers[endorserIndex].identityLocator
}

func (b *PrivateTransactionBuilderForTesting) GetNumberOfEndorsers() int {
	return b.numberOfEndorsers
}

func (b *PrivateTransactionBuilderForTesting) initializeOriginator() {

	b.originator = &identityForTesting{
		identityLocator: fmt.Sprintf("%s@%s", b.originatorName, b.originatorNode),
		identity:        b.originatorName,
		verifier:        pldtypes.RandAddress().String(),
		keyHandle:       b.originatorName + "_KeyHandle",
	}
}

func (b *PrivateTransactionBuilderForTesting) initializeEndorsers() {
	b.endorsers = make([]*identityForTesting, b.numberOfEndorsers)
	for i := 0; i < b.numberOfEndorsers; i++ {
		endorserName := fmt.Sprintf("endorser-%d", i)
		endorserNode := fmt.Sprintf("node-%d", i)
		b.endorsers[i] = &identityForTesting{
			identity:        endorserName,
			identityLocator: endorserName + "@" + endorserNode,
			verifier:        pldtypes.RandAddress().String(),
			keyHandle:       endorserName + "KeyHandle",
		}
	}
}

// Function Build creates a new complete private transaction with all fields populated as per the builder's configuration using defaults
// for any values not explicitly set by the builder
// To create a partial transaction (e.g. with no PostAssembly) use the BuildPreAssembly etc methods
func (b *PrivateTransactionBuilderForTesting) Build() *components.PrivateTransaction {

	b.initializeOriginator()
	b.initializeEndorsers()
	preAssembly := b.BuildPreAssembly()
	if b.preAssemblyOverride != nil {
		preAssembly = b.preAssemblyOverride
	}
	postAssembly := b.BuildPostAssembly()
	if b.postAssemblyOverride != nil {
		postAssembly = b.postAssemblyOverride
	}
	if len(b.chainedDependencies) > 0 {
		chainedDependsOn := make([]string, len(b.chainedDependencies))
		for i, id := range b.chainedDependencies {
			chainedDependsOn[i] = id.String()
		}
		preAssembly.ChainedDependsOn = chainedDependsOn
	}
	pt := &components.PrivateTransaction{
		ID:           b.id,
		Domain:       b.domain,
		Address:      b.address,
		PreAssembly:  preAssembly,
		PostAssembly: postAssembly,
	}
	if b.preparedPrivateTransaction != nil {
		pt.PreparedPrivateTransaction = b.preparedPrivateTransaction
	}
	if b.preparedPublicTransaction != nil {
		pt.PreparedPublicTransaction = b.preparedPublicTransaction
	}
	if b.signer != nil {
		pt.Signer = *b.signer
	}
	return pt
}

// Function BuildSparse creates a new private transaction with only the PreAssembly populated
func (b *PrivateTransactionBuilderForTesting) BuildSparse() *components.PrivateTransaction {
	b.initializeOriginator()
	b.initializeEndorsers()
	preAssembly := b.BuildPreAssembly()
	if b.preAssemblyOverride != nil {
		preAssembly = b.preAssemblyOverride
	}
	return &components.PrivateTransaction{
		ID:          b.id,
		Domain:      b.domain,
		Address:     b.address,
		PreAssembly: preAssembly,
	}
}

// Function BuildPreAssembly creates a new PreAssembly with all fields populated as per the builder's configuration using defaults unless explicitly set
func (b *PrivateTransactionBuilderForTesting) BuildPreAssembly() *prototk.TransactionPreAssembly {
	preAssembly := &prototk.TransactionPreAssembly{
		TransactionSpecification: &prototk.TransactionSpecification{
			TransactionId: pldtypes.Bytes32UUIDFirst16(b.id).String(),
		},
		RequiredVerifiers: make([]*prototk.ResolveVerifierRequest, b.numberOfEndorsers+1),
	}

	preAssembly.RequiredVerifiers[0] = &prototk.ResolveVerifierRequest{
		Lookup:       b.originator.identityLocator,
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
	}

	for i := 0; i < b.numberOfEndorsers; i++ {
		preAssembly.RequiredVerifiers[i+1] = &prototk.ResolveVerifierRequest{
			Lookup:       b.endorsers[i].identityLocator,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		}
	}

	return preAssembly
}

func (b *PrivateTransactionBuilderForTesting) buildResolvedVerifiers() []*prototk.ResolvedVerifier {
	resolvedVerifiers := make([]*prototk.ResolvedVerifier, b.numberOfEndorsers+1)
	resolvedVerifiers[0] = &prototk.ResolvedVerifier{
		Lookup:       b.originator.identityLocator,
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     pldtypes.RandAddress().String(),
	}
	for i := 0; i < b.numberOfEndorsers; i++ {
		resolvedVerifiers[i+1] = &prototk.ResolvedVerifier{
			Lookup:       b.endorsers[i].identityLocator,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     b.endorsers[i].verifier,
		}
	}
	return resolvedVerifiers
}

// Function BuildEndorsement creates a new AttestationResult for the given endorserIndex
func (b *PrivateTransactionBuilderForTesting) BuildEndorsement(endorserIndex int) *prototk.AttestationResult {

	attReqName := b.GetEndorsementName(endorserIndex)
	return &prototk.AttestationResult{
		Name:            attReqName,
		AttestationType: prototk.AttestationType_ENDORSE,
		Payload:         pldtypes.RandBytes(32),
		Verifier: &prototk.ResolvedVerifier{
			Lookup:       b.endorsers[endorserIndex].identityLocator,
			Verifier:     b.endorsers[endorserIndex].verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}
}

// Function BuildPostAssembly creates a new PostAssembly with all fields populated as per the builder's configuration using defaults unless explicitly set
func (b *PrivateTransactionBuilderForTesting) BuildPostAssembly() *components.TransactionPostAssembly {

	if b.revertReason != nil {
		return &components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   b.revertReason,
			},
		}
	}

	//it is normal to have one AttestationRequest for the originator to sign the pre-assembly
	attestationPlan := make([]*prototk.AttestationRequest, b.numberOfEndorsers+1)
	attestationPlan[0] = &prototk.AttestationRequest{
		Name:            "sign",
		AttestationType: prototk.AttestationType_SIGN,
		Algorithm:       algorithms.ECDSA_SECP256K1,
		VerifierType:    verifiers.ETH_ADDRESS,
		PayloadType:     signpayloads.OPAQUE_TO_RSV,
		Parties: []string{
			b.originator.identityLocator,
		},
	}

	signatures := []*prototk.AttestationResult{
		{
			Name:            "sign",
			AttestationType: prototk.AttestationType_SIGN,
			Payload:         pldtypes.RandBytes(32),
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       b.originator.identityLocator,
				Verifier:     b.originator.verifier,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
			PayloadType: ptrTo(signpayloads.OPAQUE_TO_RSV),
		},
	}

	for i := 0; i < b.numberOfEndorsers; i++ {
		attestationPlan[i+1] = &prototk.AttestationRequest{
			Name:            fmt.Sprintf("endorse-%d", i),
			AttestationType: prototk.AttestationType_ENDORSE,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			PayloadType:     signpayloads.OPAQUE_TO_RSV,
			Parties: []string{
				b.endorsers[i].identityLocator,
			},
		}
	}

	endorsements := make([]*prototk.AttestationResult, b.numberOfEndorsements)
	for i := 0; i < b.numberOfEndorsements; i++ {
		endorsements[i] = b.BuildEndorsement(i)
	}

	var endorsableInputStates []*prototk.EndorsableState
	for _, inputStateID := range b.inputStateIDs {
		schema := pldtypes.Bytes32(pldtypes.RandBytes(32))
		endorsableInputStates = append(endorsableInputStates, &prototk.EndorsableState{
			Id:            inputStateID,
			SchemaId:      schema.String(),
			StateDataJson: `{"data":"hello"}`,
		})
	}

	var endorsableReadStates []*prototk.EndorsableState
	for _, readStateID := range b.readStateIDs {
		schema := pldtypes.Bytes32(pldtypes.RandBytes(32))
		endorsableReadStates = append(endorsableReadStates, &prototk.EndorsableState{
			Id:            readStateID,
			SchemaId:      schema.String(),
			StateDataJson: `{"data":"hello"}`,
		})
	}

	var outputStates []*prototk.EndorsableState
	for i := 0; i < b.numberOfOutputStates; i++ {
		outputStates = append(outputStates, &prototk.EndorsableState{
			Id: pldtypes.HexBytes(pldtypes.RandBytes(32)).String(),
		})
	}

	postAssembly := &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AssemblyResult:    prototk.AssembleTransactionResponse_OK,
			ResolvedVerifiers: b.buildResolvedVerifiers(),
			AttestationPlan:   attestationPlan,
			Signatures:        signatures,
			Endorsements:      endorsements,
			InputStates:       endorsableInputStates,
			ReadStates:        endorsableReadStates,
		},
		OutputStates: outputStates,
	}
	return postAssembly

}

func ptrTo[T any](v T) *T {
	return &v
}
