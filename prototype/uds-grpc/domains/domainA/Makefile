# Copyright © 2024 Kaleido, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
# the License. You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
# an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
# specific language governing permissions and limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0


# Makefile

# Variables
PROTOC_GEN_GO := $(shell which protoc-gen-go)
PROTO_FILE := Paladin.proto
PROTO_DIR := ../../paladin/proto

# Default target
all: generate run

# Generate Go files from proto
generate:
	$(shell mkdir -p internal/protos/domain)
	protoc --go_out=internal --go-grpc_out=internal  \
		--proto_path=$(PROTO_DIR) $(PROTO_FILE)

log:
	@echo "PROTOC_GEN_GO: $(PROTOC_GEN_GO)"
	@echo "PROTO_FILE: $(PROTO_FILE)"

run: 
	go run main.go

build_wasm: 
	GOOS=wasip1 GOARCH=wasm go build -o web/main.wasm main.go

run_wasm:
	wasmtime web/main.wasm

.PHONY: all generate clean run run_wasm build_wasm
