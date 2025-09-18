#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="ci-besu"

echo "Deploying Besu infrastructure for Paladin e2e tests..."

# Create namespace if it doesn't exist
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

echo "Applying Besu resources..."

# Apply Besu resources
kubectl apply -f $SCRIPT_DIR -n $NAMESPACE

echo "Waiting for Besu node to be ready..."

# Wait for the Besu StatefulSet to be ready
kubectl wait --for=condition=ready pod/besu-standalone-besu-node-0 -n $NAMESPACE --timeout=120s

echo "Besu node is ready!"

# Get the Port for RPC HTTP access
BESU_PORT=$(kubectl get svc besu-standalone-besu-node -n $NAMESPACE -o jsonpath='{.spec.ports[?(@.name=="rpc-http")].port}')
echo "Besu RPC HTTP accessible on Port: $BESU_PORT"
