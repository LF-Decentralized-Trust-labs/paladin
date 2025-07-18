#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${BLUE}[EXAMPLE]${NC} $1"
}

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
print_status "Checking prerequisites..."

if ! command_exists npm; then
    print_error "npm is not installed"
    exit 1
fi

if ! command_exists node; then
    print_error "node is not installed"
    exit 1
fi

print_status "Prerequisites check passed"

# Function to run a single example
run_example() {
    local example_dir="$1"
    local example_name=$(basename "$example_dir")
    local exit_code=0
    
    print_header "Running example: $example_name"
    echo "=========================================="
    
    cd "$example_dir"
    
    # Copy contracts to example directory
    print_status "Setting up contracts for $example_name..."
    mkdir -p src/abis
    if ! cp -r ../../contracts/* src/abis/ 2>/dev/null; then
        print_error "No contracts found to copy for $example_name"
        cd ../..
        return 1
    fi
    
    # Switch to published SDK if not using local SDK
    if [ "$USE_PUBLISHED_SDK" = "true" ]; then
        print_status "Switching to published SDK for $example_name..."
        npm uninstall @lfdecentralizedtrust-labs/paladin-sdk 2>/dev/null || true
        npm install @lfdecentralizedtrust-labs/paladin-sdk@latest
    fi
    
    # Install dependencies
    print_status "Installing dependencies for $example_name..."
    if ! npm install; then
        print_error "Failed to install dependencies for $example_name"
        cd ../..
        return 1
    fi
    
    # Run the example
    print_status "Running $example_name..."
    if ! npm run start; then
        print_error "Example $example_name failed to run"
        exit_code=1
    else
        print_status "Completed example: $example_name"
    fi
    
    echo ""
    cd ../..
    return $exit_code
}

# Main execution
main() {
    print_status "Starting Paladin examples execution..."
    
    # Check if we're in the right directory
    if [ ! -d "example" ]; then
        print_error "example directory not found. Please run this script from the paladin root directory."
        exit 1
    fi
    
    # List all available examples
    print_status "Available examples:"
    for dir in example/*/; do
        if [ -f "$dir/package.json" ] && [ "$(basename "$dir")" != "common" ]; then
            echo "- $(basename "$dir")"
        fi
    done
    echo ""
    
    # Get list of all example directories (excluding common)
    examples=$(find example -maxdepth 1 -type d -name "*" | grep -v "example$" | grep -v "example/common" | sort)
    
    print_status "Running examples in order:"
    echo "$examples"
    echo ""
    
    local failed_examples=()
    local successful_examples=()
    
    for example_dir in $examples; do
        example_name=$(basename "$example_dir")
        
        # Check if it's a valid example (has package.json)
        if [ -f "$example_dir/package.json" ]; then
            if run_example "$example_dir"; then
                successful_examples+=("$example_name")
            else
                print_error "Example $example_name failed"
                failed_examples+=("$example_name")
            fi
        else
            print_warning "Skipping $example_name (no package.json found)"
        fi
    done
    
    # Summary
    echo "=========================================="
    print_status "Examples execution summary:"
    echo "=========================================="
    
    if [ ${#successful_examples[@]} -gt 0 ]; then
        print_status "Successful examples (${#successful_examples[@]}):"
        for example in "${successful_examples[@]}"; do
            echo "  ✅ $example"
        done
    fi
    
    if [ ${#failed_examples[@]} -gt 0 ]; then
        print_error "Failed examples (${#failed_examples[@]}):"
        for example in "${failed_examples[@]}"; do
            echo "  ❌ $example"
        done
        exit 1
    else
        print_status "All examples completed successfully! 🎉"
    fi
}

# Run main function
main "$@" 