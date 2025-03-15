package nonfungible

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetAlgoZetoSnarkBJJ verifies that getAlgoZetoSnarkBJJ returns the correct
// algorithm string for a given input name and that it matches the expected regexp.
func TestGetAlgoZetoSnarkBJJ(t *testing.T) {
	h := &baseHandler{name: "bla"}
	result := h.getAlgoZetoSnarkBJJ()
	assert.Equal(t, "domain:bla:snark:babyjubjub", result)
}

var (
	orgEncodeTransactionDataFunc = encodeTransactionDataFunc
	orgEncodeProofFunc           = encodeProofFunc
	orgFindVerifierFunc          = findVerifierFunc
	orgFindAttestationFunc       = findAttestationFunc
)

func defaultHelpers() {
	// Set the functions to the public functions for testing.
	encodeTransactionDataFunc = orgEncodeTransactionDataFunc
	encodeProofFunc = orgEncodeProofFunc
	findVerifierFunc = orgFindVerifierFunc
	findAttestationFunc = orgFindAttestationFunc
}
