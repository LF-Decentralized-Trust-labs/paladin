package privatetxnmgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
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

	mocks.keyManager.On("ResolveKey", mock.Anything, "alice", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return("", "", fmt.Errorf("test error"))

	eg := NewEndorsementGatherer(mocks.domainSmartContract, mocks.domainContext, mocks.keyManager)
	endorsementReq := &prototk.AttestationRequest{
		Algorithm:    string(algorithms.ECDSA_SECP256K1),
		VerifierType: string(verifiers.ETH_ADDRESS),
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
		Algorithm:    string(algorithms.ECDSA_SECP256K1),
		VerifierType: string(verifiers.ETH_ADDRESS),
	}
	mocks.keyManager.On("ResolveKey", mock.Anything, "alice", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return("something", "something", nil)
	mocks.domainSmartContract.On("EndorseTransaction", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("test error"))
	eg := NewEndorsementGatherer(mocks.domainSmartContract, mocks.domainContext, mocks.keyManager)
	_, _, err := eg.GatherEndorsement(ctx, &prototk.TransactionSpecification{}, []*prototk.ResolvedVerifier{}, []*prototk.AttestationResult{}, []*prototk.EndorsableState{}, []*prototk.EndorsableState{}, []*prototk.EndorsableState{}, "alice", endorsementReq)
	require.ErrorContains(t, err, "PD011801: Unexpected error in engine failed to endorse for party alice")
}
