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

