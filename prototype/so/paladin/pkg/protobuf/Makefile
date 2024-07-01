# Makefile

# Variables
PROTOC_GEN_GO := $(shell which protoc-gen-go)
PROTO_FILE := LoadDomain.proto
GO_OUT_DIR := ./go
CPP_OUT_DIR := ./cpp
OUT_DIRS := $(GO_OUT_DIR) $(CPP_OUT_DIR)
# Ensure the output directory exists
$(shell mkdir -p $(OUT_DIRS))

# Default target
all: generate

# Generate Go files from proto
generate:
	protoc --go_out=$(GO_OUT_DIR) --go_opt=paths=source_relative \
        --proto_path=. $(PROTO_FILE)
	protoc --cpp_out=$(CPP_OUT_DIR) \
        --proto_path=. $(PROTO_FILE)

# Clean generated files
clean:
	rm -rf $(OUT_DIR)/*

.PHONY: all generate clean
