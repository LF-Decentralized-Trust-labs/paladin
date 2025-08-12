# Besu Standalone Setup for Paladin E2E Tests

This directory contains the configuration files and scripts needed to deploy a standalone Besu node for Paladin e2e testing in customnet mode.

## Overview

The setup includes:
- A standalone Besu node with QBFT consensus
- Authentication enabled (username/password)
- NodePort service for external access
- Persistent storage for blockchain data
- Proper configuration for Paladin integration

## Files

### Core Configuration
- `01-besu-genesis.yaml` - Genesis block configuration
- `02-besu-node.yaml` - Besu node CR definition
- `03-besu-identity-secret.yaml` - Node identity and keys
- `04-besu-genesis-configmap.yaml` - Genesis JSON configuration
- `05-besu-config-configmap.yaml` - Besu runtime configuration
- `06-besu-pvc.yaml` - Persistent volume claim for data
- `07-besu-statefulset.yaml` - Besu pod deployment
- `08-besu-service.yaml` - Service configuration (NodePort)
- `09-besu-poddisruptionbudget.yaml` - Pod disruption budget
- `10-besu-auth-credentials.yaml` - Authentication credentials file
- `11-besu-auth-secret.yaml` - Kubernetes secret for auth

### Scripts
- `deploy-besu-for-e2e.sh` - Deployment script
- `cleanup-besu-e2e.sh` - Cleanup script

## Quick Start

### 1. Deploy Besu Infrastructure

```bash
# Make scripts executable
chmod +x *.sh

# Deploy Besu
./deploy-besu-for-e2e.sh
```

### 2. Deploy Paladin with Customnet Mode

```bash
# Using the updated values-customnet.yaml
helm install paladin-operator ./charts/paladin-operator \
  -f values-customnet.yaml \
  -n ci-customnet
```

### 3. Run E2E Tests

The Besu node will be accessible at:
- **HTTP RPC**: `localhost:32545` (username: `paladin`, password: `paladin123`)
- **WebSocket**: `localhost:32546` (username: `paladin`, password: `paladin123`)
- **GraphQL**: `localhost:32547`

### 4. Cleanup

```bash
# Clean up after tests
./cleanup-besu-e2e.sh
```

## Configuration Details

### Authentication
- **Username**: `paladin`
- **Password**: `paladin123`
- **Secret Name**: `besu-auth-credentials`

### Ports
- **RPC HTTP**: 8545 (internal) / 32545 (NodePort)
- **RPC WebSocket**: 8546 (internal) / 32546 (NodePort)
- **GraphQL**: 8547 (internal) / 32547 (NodePort)
- **P2P**: 30303 (internal) / 32303-32304 (NodePort)

### Consensus
- **Algorithm**: QBFT
- **Chain ID**: 1337
- **Block Period**: 100ms
- **Gas Limit**: 700,000,000

## Integration with Paladin

The `values-customnet.yaml` file has been updated to:
1. Reference the correct Besu node (`standalone-besu-node`)
2. Enable authentication with the provided credentials
3. Use the correct ports for Besu communication
4. Configure the local Besu endpoint type

## Troubleshooting

### Check Besu Status
```bash
kubectl get pods -n ci-customnet
kubectl logs besu-standalone-besu-node-0 -n ci-customnet
```

### Test Authentication
```bash
# Test RPC access
curl -u paladin:paladin123 \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
  http://localhost:32545
```

### Check Service Ports
```bash
kubectl get svc besu-standalone-besu-node -n ci-customnet
```

## Notes

- The Besu node uses persistent storage (10Gi) to maintain blockchain state
- Authentication is enabled for both HTTP and WebSocket RPC endpoints
- The setup is configured for local development and testing
- All resources are created in the `ci-customnet` namespace
