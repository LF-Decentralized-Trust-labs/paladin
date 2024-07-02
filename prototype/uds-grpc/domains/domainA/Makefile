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

.PHONY: all generate clean run
