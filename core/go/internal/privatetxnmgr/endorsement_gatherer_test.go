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

package privatetxnmgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGatherEndorsementFailResolveKey(t *testing.T) {
	ctx := context.Background()
	mocks := &dependencyMocks{
		domainSmartContract: componentmocks.NewDomainSmartContract(t),
		domainContext:       componentmocks.NewDomainContext(t),
		keyManager:          componentmocks.NewKeyManager(t),
	}

	mocks.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, "alice", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(nil, fmt.Errorf("test error"))

	eg := NewEndorsementGatherer(mocks.domainSmartContract, mocks.domainContext, mocks.keyManager)
	endorsementReq := &prototk.AttestationRequest{
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
	}
	_, _, err := eg.GatherEndorsement(ctx, &prototk.TransactionSpecification{}, []*prototk.ResolvedVerifier{}, []*prototk.AttestationResult{}, []*prototk.EndorsableState{}, []*prototk.EndorsableState{}, []*prototk.EndorsableState{}, "alice", endorsementReq)
	require.ErrorContains(t, err, "PD011801: Unexpected error in engine failed to resolve key for party alice")
}

func TestGatherEndorsementFailEndorseTransaction(t *testing.T) {
	ctx := context.Background()
	mocks := &dependencyMocks{
		domainSmartContract: componentmocks.NewDomainSmartContract(t),
		keyManager:          componentmocks.NewKeyManager(t),
	}
	endorsementReq := &prototk.AttestationRequest{
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
	}
	mocks.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, "alice", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
		Return(&pldapi.KeyMappingAndVerifier{
			KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{Identifier: "alice"}},
			Verifier:           &pldapi.KeyVerifier{Verifier: "something"},
		}, nil)
	mocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("test error"))
	eg := NewEndorsementGatherer(mocks.domainSmartContract, mocks.domainContext, mocks.keyManager)
	_, _, err := eg.GatherEndorsement(ctx, &prototk.TransactionSpecification{}, []*prototk.ResolvedVerifier{}, []*prototk.AttestationResult{}, []*prototk.EndorsableState{}, []*prototk.EndorsableState{}, []*prototk.EndorsableState{}, "alice", endorsementReq)
	require.ErrorContains(t, err, "PD011801: Unexpected error in engine failed to endorse for party alice")
}
