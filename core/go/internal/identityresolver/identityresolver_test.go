package identityresolver

import (
	"context"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestResolveVerifier(t *testing.T) {
	r := &identityResolver{}
	_, err := r.ResolveVerifier(context.Background(), "something$bad", "bad algorithm", "bad type")
	assert.ErrorContains(t, err, "PD020006: Locator string something$bad is invalid")

	capacity := 100
	config := &pldconf.CacheConfig{Capacity: &capacity}
	r = &identityResolver{
		nodeName:      "testnode",
		verifierCache: cache.NewCache[string, string](config, config),
		keyManager:    componentsmocks.NewKeyManager(t),
	}
	waitChan := make(chan time.Time)
	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{
			Verifier: "0x1234567890abcdef",
			Type:     "ETH_ADDRESS",
		},
	}
	r.keyManager.(*componentsmocks.KeyManager).On("ResolveKeyNewDatabaseTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).WaitUntil(waitChan).Return(resolvedKey, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer func() {
		cancel()
		close(waitChan)
	}()

	done := make(chan bool)
	go func() {
		_, err := r.ResolveVerifier(ctx, "something@testnode", algorithms.Curve_SECP256K1, verifiers.ETH_ADDRESS)
		assert.ErrorContains(t, err, "PD010301: Context canceled")
		done <- true
	}()
	<-done
}

func TestResolveVerifierAsync(t *testing.T) {
	r := &identityResolver{}
	resolved := func(ctx context.Context, verifier string) {
		t.Logf("Resolved verifier: %s", verifier)
	}
	errhandler := func(ctx context.Context, err error) {
		assert.ErrorContains(t, err, "PD020006: Locator string something$bad is invalid")
	}
	r.ResolveVerifierAsync(context.Background(), "something$bad", "bad algorithm", "bad type", resolved, errhandler)
}

func TestNewIdentityResolver(t *testing.T) {
	capacity := 100
	ctx := context.Background()
	conf := &pldconf.IdentityResolverConfig{
		VerifierCache: pldconf.CacheConfig{
			Capacity: &capacity,
		},
	}

	ir := NewIdentityResolver(ctx, conf)

	assert.NotNil(t, ir)
	assert.Equal(t, ctx, ir.(*identityResolver).bgCtx)
	assert.NotNil(t, ir.(*identityResolver).inflightRequests)
	assert.NotNil(t, ir.(*identityResolver).inflightRequestsMutex)
}
func TestCacheKey(t *testing.T) {
	tests := []struct {
		identifier   string
		node         string
		algorithm    string
		verifierType string
		expected     string
	}{
		{
			identifier:   "id1",
			node:         "node1",
			algorithm:    "alg1",
			verifierType: "type1",
			expected:     "id1@node1|alg1|type1",
		},
		{
			identifier:   "id2",
			node:         "node2",
			algorithm:    "alg2",
			verifierType: "type2",
			expected:     "id2@node2|alg2|type2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := cacheKey(tt.identifier, tt.node, tt.algorithm, tt.verifierType)
			assert.Equal(t, tt.expected, result)
		})
	}
}
