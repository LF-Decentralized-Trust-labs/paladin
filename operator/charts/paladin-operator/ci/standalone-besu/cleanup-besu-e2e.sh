#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="ci-besu"

echo "Cleaning up Besu infrastructure for Paladin e2e tests..."

# Delete all Besu resources
echo "Deleting Besu resources..."
kubectl delete -f $SCRIPT_DIR -n $NAMESPACE
# Wait for resources to be cleaned up
echo "Waiting for resources to be cleaned up..."
kubectl wait --for=delete pod/besu-standalone-besu-node-0 -n $NAMESPACE --timeout=60s 2>/dev/null || true

# Delete the namespace
echo "Deleting namespace..."
kubectl delete namespace $NAMESPACE --ignore-not-found=true

echo "Besu infrastructure cleanup completed!"
