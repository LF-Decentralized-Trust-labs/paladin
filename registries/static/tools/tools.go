//go:build tools
// +build tools

// This package only exists to record tool dependencies, so they can be managed via go.mod
package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/vektra/mockery/v2"
)
