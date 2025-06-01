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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"golang.org/x/crypto/sha3"
)

type identityForTesting struct {
	identity        string
	identityLocator string
	verifier        string
	keyHandle       string
}

type PrivateTransactionBuilderForTesting struct {
	id                     uuid.UUID
	senderName             string
	senderNode             string
	sender                 *identityForTesting
	domain                 string
	address                tktypes.EthAddress
	signerAddress          *tktypes.EthAddress
	numberOfEndorsers      int
	numberOfEndorsements   int
	numberOfOutputStates   int
	inputStateIDs          []tktypes.HexBytes
	readStateIDs           []tktypes.HexBytes
	endorsers              []*identityForTesting
	revertReason           *string
	predefinedDependencies []uuid.UUID
}

// useful for creating multiple transactions in a test, from the same sender
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

func (b PrivateTransactionBuilderListForTesting) Address(address tktypes.EthAddress) PrivateTransactionBuilderListForTesting {
	for _, builder := range b {
		builder.Address(address)
	}
	return b
}

// initialize sender identity locator e.g. name@node
func (b PrivateTransactionBuilderListForTesting) Sender(sender string) PrivateTransactionBuilderListForTesting {
	for _, builder := range b {
		builder.Sender(sender)
	}
	return b
}

func (b PrivateTransactionBuilderListForTesting) SenderName(senderName string) PrivateTransactionBuilderListForTesting {
	for _, builder := range b {
		builder.SenderName(senderName)
	}
	return b
}

func (b PrivateTransactionBuilderListForTesting) SenderNode(senderNode string) PrivateTransactionBuilderListForTesting {
	for _, builder := range b {
		builder.SenderNode(senderNode)
	}
	return b
}

// Function NewTransactionBuilderForTesting creates a TransactionBuilderForTesting with random values for all fields
// use the builder methods to set specific values for fields before calling Build to create a new Transaction
func NewPrivateTransactionBuilderForTesting() *PrivateTransactionBuilderForTesting {

	builder := &PrivateTransactionBuilderForTesting{
		id:                   uuid.New(),
		domain:               "defaultDomain",
		address:              *tktypes.RandAddress(),
		senderName:           "sender",
		senderNode:           "senderNode",
		signerAddress:        nil,
		numberOfEndorsers:    3,
		numberOfEndorsements: 0,
		numberOfOutputStates: 1,
	}

	return builder
}

func (b *PrivateTransactionBuilderForTesting) Address(address tktypes.EthAddress) *PrivateTransactionBuilderForTesting {
	b.address = address
	return b
}

func (b *PrivateTransactionBuilderForTesting) Sender(sender string) *PrivateTransactionBuilderForTesting {

	name, node, err := tktypes.PrivateIdentityLocator(sender).Validate(context.Background(), "", false)
	if err != nil {
		//this is only used for testing so panic is fine
		panic(err)
	}
	b.senderName = name
	b.senderName = node
	return b
}

func (b *PrivateTransactionBuilderForTesting) SenderName(senderName string) *PrivateTransactionBuilderForTesting {
	b.senderName = senderName
	return b
}

func (b *PrivateTransactionBuilderForTesting) SenderNode(senderNode string) *PrivateTransactionBuilderForTesting {
	b.senderNode = senderNode
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

func (b *PrivateTransactionBuilderForTesting) InputStateIDs(stateIDs ...tktypes.HexBytes) *PrivateTransactionBuilderForTesting {
	b.inputStateIDs = stateIDs
	return b
}

func (b *PrivateTransactionBuilderForTesting) ReadStateIDs(stateIDs ...tktypes.HexBytes) *PrivateTransactionBuilderForTesting {
	b.readStateIDs = stateIDs
	return b
}

func (b *PrivateTransactionBuilderForTesting) PredefinedDependencies(transactionIDs ...uuid.UUID) *PrivateTransactionBuilderForTesting {
	b.predefinedDependencies = transactionIDs
	return b
}

func (b *PrivateTransactionBuilderForTesting) Reverts(revertReason string) *PrivateTransactionBuilderForTesting {
	b.revertReason = &revertReason
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

func (b *PrivateTransactionBuilderForTesting) initializeSender() {

	b.sender = &identityForTesting{
		identityLocator: fmt.Sprintf("%s@%s", b.senderName, b.senderNode),
		identity:        b.senderName,
		verifier:        tktypes.RandAddress().String(),
		keyHandle:       b.senderName + "_KeyHandle",
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
			verifier:        tktypes.RandAddress().String(),
			keyHandle:       endorserName + "KeyHandle",
		}
	}
}

// Function Build creates a new complete private transaction with all fields populated as per the builder's configuration using defaults
// for any values not explicitly set by the builder
// To create a partial transaction (e.g. with no PostAssembly) use the BuildPreAssembly etc methods
func (b *PrivateTransactionBuilderForTesting) Build() *components.PrivateTransaction {

	b.initializeSender()
	b.initializeEndorsers()
	return &components.PrivateTransaction{
		ID:           b.id,
		Domain:       b.domain,
		Address:      b.address,
		PreAssembly:  b.BuildPreAssembly(),
		PostAssembly: b.BuildPostAssembly(),
	}

}

// Function BuildSparse creates a new private transaction with only the PreAssembly populated
func (b *PrivateTransactionBuilderForTesting) BuildSparse() *components.PrivateTransaction {
	b.initializeSender()
	b.initializeEndorsers()
	return &components.PrivateTransaction{
		ID:          b.id,
		Domain:      b.domain,
		Address:     b.address,
		PreAssembly: b.BuildPreAssembly(),
	}
}

// Function BuildPreAssembly creates a new PreAssembly with all fields populated as per the builder's configuration using defaults unless explicitly set
func (b *PrivateTransactionBuilderForTesting) BuildPreAssembly() *components.TransactionPreAssembly {
	preAssembly := &components.TransactionPreAssembly{
		RequiredVerifiers: make([]*prototk.ResolveVerifierRequest, b.numberOfEndorsers+1),
		Verifiers:         make([]*prototk.ResolvedVerifier, b.numberOfEndorsers+1),
	}

	preAssembly.RequiredVerifiers[0] = &prototk.ResolveVerifierRequest{
		Lookup:       b.sender.identityLocator,
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
	}

	preAssembly.Verifiers[0] = &prototk.ResolvedVerifier{
		Lookup:       b.sender.identityLocator,
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     tktypes.RandAddress().String(),
	}

	for i := 0; i < b.numberOfEndorsers; i++ {
		preAssembly.RequiredVerifiers[i+1] = &prototk.ResolveVerifierRequest{
			Lookup:       b.endorsers[i].identityLocator,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		}
		preAssembly.Verifiers[i+1] = &prototk.ResolvedVerifier{
			Lookup:       b.endorsers[i].identityLocator,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     b.endorsers[i].verifier,
		}
	}

	if b.predefinedDependencies != nil {
		preAssembly.Dependencies = append(preAssembly.Dependencies, b.predefinedDependencies...)
	}

	return preAssembly
}

// Function BuildEndorsement creates a new AttestationResult for the given endorserIndex
func (b *PrivateTransactionBuilderForTesting) BuildEndorsement(endorserIndex int) *prototk.AttestationResult {

	attReqName := b.GetEndorsementName(endorserIndex)
	return &prototk.AttestationResult{
		Name:            attReqName,
		AttestationType: prototk.AttestationType_ENDORSE,
		Payload:         tktypes.RandBytes(32),
		Verifier: &prototk.ResolvedVerifier{
			Lookup:       b.endorsers[endorserIndex].identityLocator,
			Verifier:     b.endorsers[endorserIndex].verifier,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	}
}

// Function BuildPostAssembly creates a new PostAssembly with all fields populated as per the builder's configuration using defaults unless explicitly set
func (b *PrivateTransactionBuilderForTesting) BuildPostAssemblyAndHash() (*components.TransactionPostAssembly, *tktypes.Bytes32) {
	postAssembly := b.BuildPostAssembly()
	hash := sha3.NewLegacyKeccak256()
	for _, signature := range postAssembly.Signatures {
		hash.Write(signature.Payload)
	}
	var h32 tktypes.Bytes32
	_ = hash.Sum(h32[0:0])
	return postAssembly, &h32
}

func (b *PrivateTransactionBuilderForTesting) BuildPostAssembly() *components.TransactionPostAssembly {

	if b.revertReason != nil {
		return &components.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   b.revertReason,
		}
	}
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}

	//it is normal to have one AttestationRequest for the sender to sign the pre-assembly
	postAssembly.AttestationPlan = make([]*prototk.AttestationRequest, b.numberOfEndorsers+1)
	postAssembly.AttestationPlan[0] = &prototk.AttestationRequest{
		Name:            "sign",
		AttestationType: prototk.AttestationType_SIGN,
		Algorithm:       algorithms.ECDSA_SECP256K1,
		VerifierType:    verifiers.ETH_ADDRESS,
		PayloadType:     signpayloads.OPAQUE_TO_RSV,
		Parties: []string{
			b.sender.identityLocator,
		},
	}

	postAssembly.Signatures = []*prototk.AttestationResult{
		{
			Name:            "sign",
			AttestationType: prototk.AttestationType_SIGN,
			Payload:         tktypes.RandBytes(32),
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       b.sender.identityLocator,
				Verifier:     b.sender.verifier,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
			PayloadType: ptrTo(signpayloads.OPAQUE_TO_RSV),
		},
	}

	for i := 0; i < b.numberOfEndorsers; i++ {
		postAssembly.AttestationPlan[i+1] = &prototk.AttestationRequest{
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

	for i := 0; i < b.numberOfOutputStates; i++ {
		postAssembly.OutputStates = append(postAssembly.OutputStates, &components.FullState{
			ID: tktypes.HexBytes(tktypes.RandBytes(32)),
		})
	}

	for _, inputStateID := range b.inputStateIDs {
		postAssembly.InputStates = append(postAssembly.InputStates, &components.FullState{
			ID:     inputStateID,
			Schema: tktypes.Bytes32(tktypes.RandBytes(32)),
			Data:   tktypes.JSONString("{\"data\":\"hello\"}"),
		})
	}

	for _, readStateID := range b.readStateIDs {
		postAssembly.ReadStates = append(postAssembly.ReadStates, &components.FullState{
			ID:     readStateID,
			Schema: tktypes.Bytes32(tktypes.RandBytes(32)),
			Data:   tktypes.JSONString("{\"data\":\"hello\"}"),
		})
	}

	postAssembly.Endorsements = make([]*prototk.AttestationResult, b.numberOfEndorsements)
	for i := 0; i < b.numberOfEndorsements; i++ {
		postAssembly.Endorsements[i] = b.BuildEndorsement(i)
	}
	return postAssembly

}

func ptrTo[T any](v T) *T {
	return &v
}
