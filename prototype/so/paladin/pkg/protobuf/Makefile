# Makefile

# Variables
PROTOC_GEN_GO := $(shell which protoc-gen-go)
PROTO_FILE := Domain.proto
GO_OUT_DIR := ./go
OUT_DIRS := $(GO_OUT_DIR) # Ensure the output directory exists
$(shell mkdir -p $(OUT_DIRS))

# Default target
all: generate

# Generate Go files from proto
generate:
	protoc --go_out=$(GO_OUT_DIR) --go_opt=paths=source_relative \
        --proto_path=. $(PROTO_FILE)

# Clean generated files
clean:
	rm -rf $(OUT_DIR)/*

log:
	@echo "PROTOC_GEN_GO: $(PROTOC_GEN_GO)"
	@echo "PROTO_FILE: $(PROTO_FILE)"
	@echo "GO_OUT_DIR: $(GO_OUT_DIR)"
	@echo "OUT_DIRS: $(OUT_DIRS)"

.PHONY: all generate clean
