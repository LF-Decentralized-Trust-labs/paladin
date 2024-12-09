package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBatchCircuit(t *testing.T) {
	assert.False(t, IsBatchCircuit(2))
	assert.True(t, IsBatchCircuit(5))
}
