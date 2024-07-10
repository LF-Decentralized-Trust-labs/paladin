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

# Variables
PROTOC_GEN_GO := $(shell which protoc-gen-go)
PROTO_FILE := Paladin.proto
PROTO_PATH := ./proto
BINARY_NAME := ./bin/main
# Default target
all: generate build run

build: 
	go build -o $(BINARY_NAME) main.go

run: 
	rm /tmp/grpc.sock || true
	$(BINARY_NAME) 

clean:
	rm $(BINARY_NAME)
	rm -rf internal/protos/domain

# Generate Go files from proto
generate:
	$(shell mkdir -p internal/protos/domain)
	protoc --go_out=internal --go-grpc_out=internal  \
        --proto_path=$(PROTO_PATH) $(PROTO_FILE)

log:
	@echo "PROTOC_GEN_GO: $(PROTOC_GEN_GO)"
	@echo "PROTO_FILE: $(PROTO_FILE)"

.PHONY: all generate clean build

