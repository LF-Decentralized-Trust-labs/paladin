#!/bin/bash

# Environment variables for configuration
# RUN_COMMANDS: comma-separated list of npm scripts to run (default: "start")
#   - "start": runs npm run start (deploy/run examples)
#   - "verify": runs npm run verify (verify historical data)
#   - "start,verify": runs both start and verify commands in sequence
#
# Examples:
#   ./scripts/run-examples.sh # this will run all examples with the latest paladin SDK and solidity contracts
#   BUILD_PALADIN_SDK=true BUILD_PALADIN_ABI=true ./scripts/run-examples.sh
#   PALADIN_SDK_VERSION=0.10.0 ./scripts/run-examples.sh # use a specific paladin SDK version
#   PALADIN_ABI_VERSION=v0.10.0 ./scripts/run-examples.sh # use a specific paladin solidity version
#   ZETO_ABI_VERSION=v0.2.0 ./scripts/run-examples.sh # use a specific zeto solidity version
#   RUN_COMMANDS=start ./scripts/run-examples.sh
#   RUN_COMMANDS=verify ./scripts/run-examples.sh
#   RUN_COMMANDS=start,verify ./scripts/run-examples.sh
#   IGNORE_EXAMPLES=event-listener,private-stablecoin,bond,helloworld,swap,zeto ./scripts/run-examples.sh # ignore examples
#
# Cache directory arguments:
#   ./scripts/run-examples.sh [base_cache_dir] [version_tag]
#   ./scripts/run-examples.sh /tmp/cache v0.10.0 # specify cache directory and version
RUN_COMMANDS=${RUN_COMMANDS:-"start"}
BUILD_PALADIN_SDK=${BUILD_PALADIN_SDK:-"false"} # build the paladin SDK locally
BUILD_PALADIN_ABI=${BUILD_PALADIN_ABI:-"false"} # build the paladin solidity contracts locally

PALADIN_SDK_VERSION=${PALADIN_SDK_VERSION:-""} # download the paladin SDK from npm (default is latest)
PALADIN_ABI_VERSION=${PALADIN_ABI_VERSION:-""} # download the paladin solidity contracts from npm (default is latest)   
ZETO_ABI_VERSION=${ZETO_ABI_VERSION:-"v0.2.0"} # download the zeto solidity contracts from npm (default is v0.2.0)

IGNORE_EXAMPLES=${IGNORE_EXAMPLES:-""} # ignore examples (; separated list of example names)

# Command line arguments for cache directory
BASE_CACHE_DIR=${1:-""} # first argument: base cache directory
VERSION_TAG_ARG=${2:-""} # second argument: version tag

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

EXAMPLES_DIR=examples

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


# if build locally is set, make sure the tag is not set
if [ "$BUILD_PALADIN_SDK" = "true" ] && [ "$PALADIN_SDK_VERSION" != "" ]; then
    print_error "You cannot set PALADIN_SDK_VERSION when BUILD_PALADIN_SDK is true"
    exit 1
fi
if [ "$BUILD_PALADIN_ABI" = "true" ] && [ "$PALADIN_ABI_VERSION" != "" ]; then  
    print_error "You cannot set PALADIN_ABI_VERSION when BUILD_PALADIN_ABI is true"
    exit 1
fi

# set paladin SDK version to latest if not set
if [ "$PALADIN_SDK_VERSION" = "" ] && [ "$BUILD_PALADIN_SDK" = "false" ]; then
    PALADIN_SDK_VERSION=latest
    # PALADIN_SDK_VERSION=$(npm view @lfdecentralizedtrust/paladin-sdk version)
    print_status "PALADIN_SDK_VERSION not set, using latest version: $PALADIN_SDK_VERSION"
fi

if [ "$PALADIN_ABI_VERSION" = "" ] && [ "$BUILD_PALADIN_ABI" = "false" ]; then
    PALADIN_ABI_VERSION=latest
    print_status "PALADIN_ABI_VERSION not set, using latest version: $PALADIN_ABI_VERSION"
fi

print_status "Prerequisites check passed"

# switch paladin sdk version
switch_paladin_sdk_version() {
    local name="$1"
    if [ "$BUILD_PALADIN_SDK" = "true" ]; then
        print_status "Running $name with local paladin SDK..."
        npm uninstall @lfdecentralizedtrust/paladin-sdk 2>/dev/null || true
        if ! npm install file:../../sdk/typescript; then
            print_error "Failed to install local SDK for $name"
            exit 1
        fi
    fi

    if [ "$PALADIN_SDK_VERSION" != "" ]; then
        print_status "Running $name with paladin SDK version $PALADIN_SDK_VERSION..."
        npm uninstall @lfdecentralizedtrust/paladin-sdk 2>/dev/null || true
        if ! npm install @lfdecentralizedtrust/paladin-sdk@$PALADIN_SDK_VERSION; then
            print_error "Failed to install SDK version $PALADIN_SDK_VERSION for $name"
            exit 1
        fi
    fi
}

install_prerequisites() {
    # there are three prerequisites:
    # 1. build solidity contracts
    # 2. build paladin sdk
    # 3. build common
    # now, you can chose not to build the solidity or the SDK and use the published ones

    DOWNLOAD_CONTRACTS_CMD=download-abi
    COPY_CONTRACTS_CMD=copy-abi

    # build paladin solidity contracts
    if [ "$BUILD_PALADIN_ABI" = "true" ]; then
        print_status "Building paladin solidity contracts..."
        cd solidity
        if ! npm install; then
            print_error "Failed to install dependencies for solidity"
            exit 1
        fi
        if ! npm run compile; then
            print_error "Failed to compile solidity contracts"
            exit 1
        fi
        DOWNLOAD_CONTRACTS_CMD=abi
        COPY_CONTRACTS_CMD=abi
        cd ..
    fi

    # build paladin SDK
    if [ "$BUILD_PALADIN_SDK" = "true" ]; then
        print_status "Building paladin SDK..."
        cd sdk/typescript
        if ! npm install; then
            print_error "Failed to install dependencies for paladin SDK"
            exit 1
        fi
        if ! npm run $DOWNLOAD_CONTRACTS_CMD; then
            print_error "Failed to run abi for paladin SDK"
            exit 1
        fi
        if ! npm run build; then
            print_error "Failed to build paladin SDK"
            exit 1
        fi
        cd ../..
    fi

    # build common
    cd $EXAMPLES_DIR/common

    switch_paladin_sdk_version "common"

    if ! npm install; then
        print_error "Failed to install dependencies for common"
        exit 1
    fi

    if ! npm run $DOWNLOAD_CONTRACTS_CMD; then
        print_error "Failed to copy contracts for common using `$DOWNLOAD_CONTRACTS_CMD`"
        exit 1
    fi

    if ! npm run build; then
        print_error "Failed to build common"
        exit 1
    fi
    cd ../..
}


# Function to run a single example
run_example() {
    local examples_dir="$1"
    local example_name=$(basename "$examples_dir")
    local exit_code=0
    
    print_header "Running example: $example_name"
    echo "=========================================="

    config_file="../common/config.json"
    
    cd "$examples_dir"
    
    # Install dependencies
    print_status "Installing dependencies for $example_name..."

    # switch to the correct paladin sdk version
    switch_paladin_sdk_version "$example_name"

    if ! npm install; then
        print_error "Failed to install dependencies for $example_name"
        cd ../..
        return 1
    fi

    if ! npm run $COPY_CONTRACTS_CMD; then
        print_error "Failed to run 'npm run $COPY_CONTRACTS_CMD' for $example_name"
        cd ../..
        return 1
    fi  
    
    mkdir -p logs
    
    local cmd_cache_path=""
    # Construct cache directory path if provided
    # default is <example_dir>/data
    if [ -n "$BASE_CACHE_DIR" ] && [ -n "$VERSION_TAG_ARG" ]; then
        example_cache_path="$BASE_CACHE_DIR/$VERSION_TAG_ARG/$example_name"
        print_status "Using cache path: $example_cache_path"
        mkdir -p "$example_cache_path"
        cmd_cache_path="--cache $example_cache_path"
    fi
    
    # Split RUN_COMMANDS by comma and run each command
    IFS=',' read -ra COMMANDS <<< "$RUN_COMMANDS"
    
    for command in "${COMMANDS[@]}"; do
        command=$(echo "$command" | xargs) # trim whitespace
        
        # Run the example command, passing the cache path as an argument.
        cmd="$command -- $cmd_cache_path --config $config_file"
        print_status "Running command: $cmd"
        if ! npm run $cmd; then
            print_error "Example $example_name failed to run command '$command'"
            exit_code=1
            break
        else
            print_status "Completed $example_name command: $command"
        fi
    done
    
    echo ""
    cd ../..
    return $exit_code
}

# Main execution
main() {
    print_status "Starting Paladin examples execution..."

    # Check if we're in the right directory
    if [ ! -d "$EXAMPLES_DIR" ]; then
        print_error "$EXAMPLES_DIR directory not found. Please run this script from the paladin root directory."
        exit 1
    fi
    
    # List all available examples
    print_status "Available examples:"
    for dir in $EXAMPLES_DIR/*/; do
        if [ -f "$dir/package.json" ] && [ "$(basename "$dir")" != "common" ]; then
            echo "- $(basename "$dir")"
        fi
    done
    echo ""

    # install prerequisites
    print_status "Installing prerequisites..."
    install_prerequisites
    print_status "Prerequisites installed"
    
    # Get list of all example directories (excluding common)
    examples=$(find $EXAMPLES_DIR -maxdepth 1 -type d -name "*" | grep -v "examples$" | grep -v "$EXAMPLES_DIR/common" | sort)
    
    print_status "Running examples in order:"
    echo "$examples"
    echo ""
    
    local failed_examples=()
    local successful_examples=()
    local skipped_examples=()
    
    for examples_dir in $examples; do
        example_name=$(basename "$examples_dir")

        # Check if example should run based on metadata and current version
        if [ -f "$examples_dir/package.json" ]; then
            # ignore examples if IGNORE_EXAMPLES is set
            if [ "$IGNORE_EXAMPLES" != "" ]; then
                if [[ "$IGNORE_EXAMPLES" == *$example_name* ]]; then
                    print_status "Skipping example $example_name (IGNORE_EXAMPLES)"
                    skipped_examples+=("$example_name")
                    continue
                fi
            fi
 
            run_example "$examples_dir"
            exit_code=$?
            if [ $exit_code -eq 0 ]; then
                successful_examples+=("$example_name")
            elif [ $exit_code -eq 2 ]; then
                skipped_examples+=("$example_name")
            else
                print_error "Example $example_name failed"
                failed_examples+=("$example_name")
            fi
        else
            print_warning "Skipping example $example_name (no package.json found)"
        fi
    done
    
    print_status "BUILD_PALADIN_SDK: $BUILD_PALADIN_SDK"
    print_status "BUILD_PALADIN_ABI: $BUILD_PALADIN_ABI"
    print_status "PALADIN_SDK_VERSION: $PALADIN_SDK_VERSION"
    print_status "RUN_COMMANDS: $RUN_COMMANDS"
    
    # Display cache configuration in summary
    local version_tag_to_use="${VERSION_TAG_ARG:-$VERSION_TAG}"
    if [ "$BASE_CACHE_DIR" != "" ] && [ "$version_tag_to_use" != "" ]; then
        print_status "BASE_CACHE_DIR: $BASE_CACHE_DIR"
        print_status "VERSION_TAG: $version_tag_to_use"
    fi

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

    if [ ${#skipped_examples[@]} -gt 0 ]; then
        print_status "Skipped examples (${#skipped_examples[@]}):"
        for example in "${skipped_examples[@]}"; do
            echo " 🚫 $example"
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