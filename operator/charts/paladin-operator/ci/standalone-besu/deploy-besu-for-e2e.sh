#!/bin/bash

# Deploy Besu infrastructure for Paladin e2e tests
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="ci-besu"

echo "🚀 Deploying Besu infrastructure for Paladin e2e tests..."

# Create namespace if it doesn't exist
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

echo "📦 Applying Besu CRDs and resources..."

# Apply Besu resources in order
kubectl apply -f $SCRIPT_DIR -n $NAMESPACE

echo "⏳ Waiting for Besu node to be ready..."

# Wait for the Besu StatefulSet to be ready
kubectl wait --for=condition=ready pod/besu-standalone-besu-node-0 -n $NAMESPACE --timeout=120s

echo "✅ Besu node is ready!"

# Get the NodePort for RPC HTTP access
NODE_PORT=$(kubectl get svc besu-standalone-besu-node -n $NAMESPACE -o jsonpath='{.spec.ports[?(@.name=="rpc-http")].port}')
echo "🌐 Besu RPC HTTP accessible on Port: $NODE_PORT"

# Test the connection (optional)
echo "🧪 Testing Besu connection..."
if kubectl get pod besu-standalone-besu-node-0 -n $NAMESPACE -o jsonpath='{.status.containerStatuses[0].ready}' | grep -q "true"; then
    echo "✅ Besu node is running and ready!"
else
    echo "❌ Besu node is not ready yet"
    echo "   This might be expected during initial startup"
fi

echo ""
echo "🎯 Besu infrastructure is ready for Paladin e2e tests!"
echo "📋 Next steps:"
echo "   1. Deploy Paladin using: helm install paladin-operator ./charts/paladin-operator -f values-customnet.yaml -n $NAMESPACE"
echo "   2. The Besu node is accessible at:"
echo "      - HTTP RPC: localhost:$NODE_PORT (username: paladin, password: paladin123)"
echo "      - WebSocket: localhost:$(kubectl get svc besu-standalone-besu-node -n $NAMESPACE -o jsonpath='{.spec.ports[?(@.name=="rpc-ws")].nodePort}')"
echo "      - GraphQL: localhost:$(kubectl get svc besu-standalone-besu-node -n $NAMESPACE -o jsonpath='{.spec.ports[?(@.name=="graphql-http")].nodePort}')"
echo ""
echo "🔐 Authentication credentials:"
echo "   Username: paladin123"
echo "   Password: paladin"
echo ""
echo "📝 Note: New NodePorts used to avoid conflicts:"
echo "   - HTTP RPC: 32645"
echo "   - WebSocket: 32646"
echo "   - GraphQL: 32647"
