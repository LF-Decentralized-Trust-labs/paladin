#!/bin/bash

set -e

# Default namespace
NAMESPACE=${1:-paladin}

# Environment variable for image tag verification
IMAGE_TAG=${IMAGE_TAG:-""}

# Show usage if help is requested
if [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    echo "Usage: $0 [NAMESPACE]"
    echo ""
    echo "Validates Paladin installation in the specified namespace."
    echo ""
    echo "Arguments:"
    echo "  NAMESPACE    Kubernetes namespace to validate (default: paladin)"
    echo ""
    echo "Environment Variables:"
    echo "  IMAGE_TAG    Image tag to verify in all pods (optional)"
    echo ""
    echo "Examples:"
    echo "  $0                    # Validate in 'paladin' namespace"
    echo "  $0 my-paladin-ns      # Validate in 'my-paladin-ns' namespace"
    echo "  IMAGE_TAG=v1.2.3 $0   # Validate and verify image tag v1.2.3"
    echo "  $0 --help             # Show this help message"
    exit 0
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
print_status "Checking prerequisites..."

if ! command_exists kubectl; then
    print_error "kubectl is not installed"
    exit 1
fi

if ! command_exists helm; then
    print_error "helm is not installed"
    exit 1
fi

if ! command_exists curl; then
    print_error "curl is not installed"
    exit 1
fi

print_status "Prerequisites check passed"

function check_pod_status() {
    local max_wait_time=300
    local interval=10
    local elapsed_time=0

    echo "[INFO] Waiting for all pods in namespace '$NAMESPACE' to be ready..."

    while [ $elapsed_time -lt $max_wait_time ]; do
        
        # we sleep before the first check to avoid race conditions
        sleep $interval
        elapsed_time=$((elapsed_time + interval))

        local expected_pods=0
        local dep_replicas=$(kubectl --namespace "$NAMESPACE" get deployments -o jsonpath='{.items[*].spec.replicas}' 2>/dev/null)
        for i in $dep_replicas; do
            expected_pods=$((expected_pods + i))
        done
        local sts_replicas=$(kubectl --namespace "$NAMESPACE" get statefulsets -o jsonpath='{.items[*].spec.replicas}' 2>/dev/null)
        for i in $sts_replicas; do
            expected_pods=$((expected_pods + i))
        done

        if [ "$expected_pods" -eq 0 ]; then
            echo "[WARN] No deployments or statefulsets found in namespace '$NAMESPACE'. Skipping pod status check."
            continue
        fi
        local pod_list=$(kubectl --namespace "$NAMESPACE" get pods --no-headers 2>/dev/null || true)
        local total_pods=0
        local ready_pods=0

        if [ -n "$pod_list" ]; then
            total_pods=$(echo "$pod_list" | wc -l)
            local all_pods_ready_and_running=$(echo "$pod_list" | awk '
                BEGIN { ready=0 }
                {
                    split($2, ready_status, "/");
                    if ($3 == "Running" && ready_status[1] == ready_status[2]) {
                        ready++;
                    }
                }
                END { print ready }
            ')
            ready_pods=$all_pods_ready_and_running
        fi

        if [ "$total_pods" -eq "$expected_pods" ] && [ "$ready_pods" -eq "$expected_pods" ]; then
            echo "[INFO] All pods are ready! ($ready_pods/$expected_pods)"
            echo "[INFO] Pod status:"
            kubectl --namespace "$NAMESPACE" get pods
            return 0
        fi

        echo "[INFO] Waiting for pods to become ready... ($ready_pods/$expected_pods)"
    done

    echo "[ERROR] Timed out waiting for pods to be ready."
    kubectl --namespace "$NAMESPACE" get pods
    return 1
}

wait_for_image_tag() {
    local max_attempts=30
    local attempt=1
    local image_tag=$1
    
    print_status "Checking Paladin pods for image tag: $image_tag"
    
    # Get all pods and filter for Paladin-related pods (exclude besu-*)
    kubectl get pods -n $NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].image}{"\n"}{end}' | grep -v "besu-" | grep $image_tag

    while [ $attempt -le $max_attempts ]; do
        # Get all Paladin pods (excluding besu-*) and check if they have the expected image tag
        local paladin_pods=$(kubectl get pods -l app=paladin -n $NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[1].image}{"\n"}{end}')
        local pods_with_tag=$(echo "$paladin_pods" | grep $image_tag | wc -l)
        local total_paladin_pods=$(echo "$paladin_pods" | wc -l)
        
        if [ "$pods_with_tag" -eq "$total_paladin_pods" ] && [ "$total_paladin_pods" -gt 0 ]; then
            print_status "All Paladin pods have the expected image tag! ($pods_with_tag/$total_paladin_pods)"
            print_status "Paladin pods with image tag $image_tag:"
            echo "$paladin_pods" | grep $image_tag
            return 0
        fi

        print_status "Attempt $attempt/$max_attempts: $pods_with_tag/$total_paladin_pods Paladin pods have image tag $image_tag"
        print_status "Current Paladin pod images:"
        echo "$paladin_pods"
        sleep 10
        ((attempt++))
    done

    print_error "Timeout waiting for image update"
    print_error "Final Paladin pod status:"
    kubectl get pods -l app=paladin -n $NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].image}{"\n"}{end}'
    return 1
}

# Function to check smart contract deployments
check_smart_contracts() {
    print_status "Checking smart contract deployments..."
    
    local max_attempts=60
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        local success_count=$(kubectl --namespace $NAMESPACE get scd --no-headers | grep -c "Success" || echo "0")
        local total_count=$(kubectl --namespace $NAMESPACE get scd --no-headers | wc -l || echo "0")
        
        if [ "$success_count" -eq "$total_count" ] && [ "$total_count" -gt 0 ]; then
            print_status "All smart contracts deployed successfully! ($success_count/$total_count)"
            return 0
        fi
        
        print_status "Attempt $attempt/$max_attempts: $success_count/$total_count contracts deployed"
        sleep 10
        ((attempt++))
    done
    
    print_error "Timeout waiting for smart contract deployments"
    kubectl --namespace $NAMESPACE get scd
    return 1
}

# Function to check registration status
check_registrations() {
    print_status "Checking Paladin node registrations..."
    
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        local published_count=$(kubectl --namespace $NAMESPACE get reg --no-headers | awk '{sum += $2} END {print sum}' || echo "0")
        
        if [ "$published_count" -ge 6 ]; then
            print_status "All nodes have published registrations! (Total: $published_count)"
            return 0
        fi
        
        print_status "Attempt $attempt/$max_attempts: Total published registrations: $published_count"
        sleep 10
        ((attempt++))
    done
    
    print_error "Timeout waiting for node registrations"
    kubectl --namespace $NAMESPACE get reg
    return 1
}

# Function to check UI responsiveness
check_ui_responsiveness() {
    print_status "Checking UI responsiveness..."
    
    local ui_ports=(31548 31648 31748)
    local failed_checks=0
    
    for port in "${ui_ports[@]}"; do
        print_status "Checking UI on port $port..."
        
        # Wait for port to be available
        local port_attempts=0
        while [ $port_attempts -lt 30 ]; do
            if curl -s --connect-timeout 5 "http://localhost:$port/ui" >/dev/null 2>&1; then
                print_status "UI on port $port is responsive"
                break
            fi
            sleep 2
            ((port_attempts++))
        done
        
        if [ $port_attempts -eq 30 ]; then
            print_error "UI on port $port is not responsive"
            ((failed_checks++))
        fi
    done
    
    if [ $failed_checks -eq 0 ]; then
        print_status "All UIs are responsive!"
        return 0
    else
        print_error "$failed_checks UI(s) failed responsiveness check"
        return 1
    fi
}

# Function to check Paladin domains
check_paladin_domains() {
    print_status "Checking Paladin domains..."
    
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        local available_count=$(kubectl --namespace $NAMESPACE get paladindomain --no-headers | grep -c "Available" || echo "0")
        local total_count=$(kubectl --namespace $NAMESPACE get paladindomain --no-headers | wc -l || echo "0")
        
        if [ "$available_count" -eq "$total_count" ] && [ "$total_count" -ge 3 ]; then
            print_status "All Paladin domains are available! ($available_count/$total_count)"
            return 0
        fi
        
        print_status "Attempt $attempt/$max_attempts: $available_count/$total_count domains available"
        sleep 10
        ((attempt++))
    done
    
    print_error "Timeout waiting for Paladin domains to be available"
    kubectl --namespace $NAMESPACE get paladindomain
    return 1
}

# Function to check registry
check_registry() {
    print_status "Checking registry..."
    
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        local available_count=$(kubectl --namespace $NAMESPACE get registry --no-headers | grep -c "Available" || echo "0")
        local total_count=$(kubectl --namespace $NAMESPACE get registry --no-headers | wc -l || echo "0")
        
        if [ "$available_count" -eq "$total_count" ] && [ "$total_count" -gt 0 ]; then
            print_status "All registries are available! ($available_count/$total_count)"
            return 0
        fi
        
        print_status "Attempt $attempt/$max_attempts: $available_count/$total_count registries available"
        sleep 10
        ((attempt++))
    done
    
    print_error "Timeout waiting for registries to be available"
    kubectl --namespace $NAMESPACE get registry
    return 1
}

# Main validation flow
main() {
    print_status "Starting Paladin installation validation (namespace: $NAMESPACE)..."
    if [ -n "$IMAGE_TAG" ]; then
        print_status "Image tag verification enabled: $IMAGE_TAG"
    fi
    
    # Check if namespace exists
    if ! kubectl get namespace $NAMESPACE >/dev/null 2>&1; then
        print_error "$NAMESPACE namespace does not exist"
        exit 1
    fi

    # Check the paladin pods are running the correct image tag (if provided)
    if [ -n "$IMAGE_TAG" ]; then
        print_status "Verifying image tag: $IMAGE_TAG"
        if ! wait_for_image_tag "$IMAGE_TAG"; then
            exit 1
        fi
    fi
    
    # Wait for pods to be ready
    if ! check_pod_status; then
        exit 1
    fi
    
    # Display pod status
    print_status "Pod status:"
    kubectl --namespace $NAMESPACE get pods
    

    
    # Display service status
    print_status "Service status:"
    kubectl --namespace $NAMESPACE get service
    
    # Check smart contract deployments
    if ! check_smart_contracts; then
        exit 1
    fi
    
    # Display smart contract status
    print_status "Smart contract deployment status:"
    kubectl --namespace $NAMESPACE get scd
    
    # Check registrations
    if ! check_registrations; then
        exit 1
    fi
    
    # Display registration status
    print_status "Registration status:"
    kubectl --namespace $NAMESPACE get reg
    
    # Check Paladin domains
    if ! check_paladin_domains; then
        exit 1
    fi
    
    # Display Paladin domain status
    print_status "Paladin domain status:"
    kubectl --namespace $NAMESPACE get paladindomain
    
    # Check registry
    if ! check_registry; then
        exit 1
    fi
    
    # Display registry status
    print_status "Registry status:"
    kubectl --namespace $NAMESPACE get registry
    
    # Check UI responsiveness
    if ! check_ui_responsiveness; then
        exit 1
    fi
    
    print_status "All validation checks passed! Paladin installation is successful."
}

# Run main function
main "$@"