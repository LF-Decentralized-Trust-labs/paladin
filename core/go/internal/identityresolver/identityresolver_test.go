package identityresolver

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
)

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
