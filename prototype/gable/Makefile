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
