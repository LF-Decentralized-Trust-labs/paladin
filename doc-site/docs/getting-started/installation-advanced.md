# Paladin Advanced Installation Guide

This guide covers advanced installation options for deploying Paladin using Helm, providing detailed control over various configuration options for different deployment scenarios.

## Prerequisites

* [Helm v3](https://helm.sh/docs/intro/install/) installed
* [kubectl](https://kubernetes.io/docs/tasks/tools/) installed

## Installation Modes

Paladin supports the following advanced installation modes:
1. [devnet](#1-devnet-default)
2. [customnet](#2-customnet)
3. [operator-only](#3-operator-only-none)

### 1. **devnet (default)**

Deploys a complete, ready-to-use Paladin network including domains and smart contract resources with default settings (3 nodes).

Default installation:

```bash
helm install paladin paladin/paladin-operator
```

You can customize various parameters such as:

* **Number of nodes**
* **Node name prefixes** (for both Paladin and Besu nodes)

Example custom installation:

```bash
helm install paladin paladin/paladin-operator \
  --set nodeCount=5 \
  --set paladin.nodeNamePrefix=worker \
  --set besu.nodeNamePrefix=evm
```

Refer to the provided [`values.yaml`](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/operator/charts/paladin-operator/values.yaml) for additional configurable options, including:

* Docker image repositories and tags
* Resource limits and requests
* Custom environment variables
* Service configurations (ports, node ports)

### 2. **customnet**

The `customnet` mode offers maximum flexibility, allowing detailed customization of `Paladin`, `Registry`, and `PaladinDomain` CRs. It is ideal for advanced use cases, such as integration with external blockchain nodes or deployments across multiple Kubernetes clusters.

Example usage:

```bash
helm install paladin paladin/paladin-operator \
  --set mode=customnet \
  --values values-customnet.yaml
```

The [`values-customnet.yaml`](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/operator/charts/paladin-operator/values-customnet.yaml) file provided within the Helm chart allows you to explicitly configure:

* **Paladin node names** (set individually for each node)
* **Blockchain endpoints** (local or remote)

  * Options:

    * Local Besu node
    * External blockchain JSON-RPC and WebSocket endpoints (with optional basic authentication)
* [**Smart Contract references**](#smart-contracts-references)

  * Deploy new smart contracts or reuse existing ones by setting either `deployment` (new) or `address` (existing)
* **Secret-backed signers**

  * Automatically generated HD wallets or pre-configured HD wallets stored in Kubernetes Secrets
* **Detailed service configurations** (including NodePort settings)
* **Database modes and migration settings**

### 3. **operator-only (none)**

Deploys only the Paladin operator without additional nodes or domains, useful for advanced scenarios or incremental setup:

```bash
helm install paladin paladin/paladin-operator --set mode=operator-only
```

## Smart Contracts References

You have the flexibility to manage smart contracts by either deploying new instances or referencing existing contract addresses.

**Default Scenario:**
During a fresh installation, smart contracts for domains and the registry are deployed automatically and referenced by deployment names:
```yaml
smartContractsReferences:
  notoFactory:
    address: ""                # Leave blank to deploy a new instance
    deployment: noto-factory
  zetoFactory:
    address: ""
    deployment: zeto-factory
  penteFactory:
    address: ""
    deployment: pente-factory
  registry:
    address: ""
    deployment: registry
```

**Existing Deployment Scenario:**
If you already have smart contracts deployed (e.g., deploying Paladin on multiple namespaces/clusters referencing a shared blockchain), first retrieve the existing smart contract addresses:

```bash
% kubectl get registry,paladindomain
NAME           TYPE   STATUS      CONTRACT
evm-registry   evm    Available   0x4456307ef3f119dac17a5e974d2640f714e6edb0
```

Example output:
```bash
NAME    STATUS      DOMAIN_REGISTRY                              DEPLOYMENT      LIBRARY
noto    Available   0x2681852a96b053746ff2f2f0bb94c3fbe1d63e7e   noto-factory    /app/domains/libnoto.so
pente   Available   0x4ea4549eca420802f1d74606775370063b7bcc70   pente-factory   /app/domains/pente.jar
zeto    Available   0x6ff1c15409614ad89b67baa6018d3499ef779e0b   zeto-factory    /app/domains/libzeto.so
```

Then update your values file accordingly to ensure Paladin uses the existing contracts:
```yaml
smartContractsReferences:
  notoFactory:
    address: "0x2681852a96b053746ff2f2f0bb94c3fbe1d63e7e"
  zetoFactory:
    address: "0x4ea4549eca420802f1d74606775370063b7bcc70"
  penteFactory:
    address: "0x6ff1c15409614ad89b67baa6018d3499ef779e0b"
  registry:
    address: "0x4456307ef3f119dac17a5e974d2640f714e6edb0"
```
This configuration ensures Paladin aligns with existing network contracts.

## Advanced Customization

For users requiring direct application and full manual control, including complex multi-cluster setups, refer to the previous [detailed manual installation documentation](#). This approach involves manually configuring Custom Resources (CRs), applying individual artifacts, and managing explicit node and domain settings.
 