//go:build tools
// +build tools

// This package only exists to record tool dependencies, so they can be managed via go.mod
package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
