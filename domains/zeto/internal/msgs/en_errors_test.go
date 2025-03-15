package msgs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFfeError(t *testing.T) {
	assert.Panics(t, func() {
		pde("notvalid", "")
	})
}
