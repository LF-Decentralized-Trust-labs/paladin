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
GO_FILES := $(shell find * -name '*.go' -print)
PROTO_FILES := $(shell find * -name '*.proto' -print)
PATH := $(shell pwd)/bin:$(PATH)

build: protobufs gable.so

gable.so: ${GO_FILES}
	go build -o gable.so -buildmode=c-shared gable.go

protobufs:
    # Use go.mod for dependency version management (tools/tools.go tracks deps for go mod tidy)
	GOBIN=${PWD}/bin go install google.golang.org/protobuf/cmd/protoc-gen-go
	GOBIN=${PWD}/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
	protoc --go_out=${PWD} --go-grpc_out=${PWD} --proto_path=${PWD} $(PROTO_FILES)
