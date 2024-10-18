# Paladin Kubernetes Operator Design Document

## Overview

The **Paladin Kubernetes Operator** is a custom controller for managing Paladin nodes and smart contracts in a Kubernetes environment. The operator interacts with a blockchain (initially, a Besu network) and manages the lifecycle of Paladin nodes and smart contracts through Custom Resource Definitions (CRDs).

The operator will perform the following tasks:
1. **Manage Paladin CRs**: Create and manage Paladin nodes as StatefulSets in Kubernetes.
2. **Manage Besu CRs**: Create and manage Besu nodes as StatefulSets in Kubernetes.
3. **Manage SmartContractDeployment CRs**: Submit smart contracts to the blockchain and track their status (via paladin or directly to the blockchain).

## Architecture

- **Custom Resources (CRs)**: 
  - `Paladin`: Represents a Paladin node.
  - `Besu`: Represents a Besu node for simple dev blockchain networks
  - `SmartContractDeployment`: Represents a smart contract that will be submitted to the blockchain.
- **Operator**: Manages the CR lifecycle and interacts with the blockchain via the Paladin/Besu API.

## High-Level Workflow

### 1. Helm Chart Installation

1. User installs the operator via a Helm chart.
2. The Helm chart creates a ConfigMap and Secret containing the Besu blockchain connection details (e.g., node URLs, authentication details).
3. The operator starts and loads the blockchain configuration from the ConfigMap.
4. The operator will continuously attempt to connect to the Besu blockchain and set its status as "Ready" only when the connection is successful.

### 2. Paladin CR Lifecycle

1. User creates a `Paladin` custom resource with the necessary configuration (matching `config.paladin.yaml`).
2. The operator generates a ConfigMap from the CR's configuration.
3. The operator creates a StatefulSet that runs an instance of Paladin.
4. The StatefulSet mounts the ConfigMap as a volume, and the Paladin instance uses the configuration file in its command line.

### 3. Besu CR Lifecycle - optional

1. User creates a `Besu` custom resource with the necessary configuration (matching `config.paladin.yaml`).
2. The operator generates a ConfigMap from the CR's configuration.
3. The operator creates a StatefulSet that runs an instance of Besu.
4. The StatefulSet mounts the ConfigMap as a volume, and the Besu instance uses the configuration file in its command line.

### 4. SmartContractDeployment CR Lifecycle

1. User creates a `SmartContractDeployment` custom resource with the Solidity code and target (Paladin or blockchain directly).
2. The operator compiles and submits the smart contract to the blockchain via Besu's API.
3. The operator updates the `SmartContractDeployment` CR's status with the transaction ID and tracks the contract deployment status (e.g., **Pending**, **Success**, **Failed**, **Rejected**).

## Custom Resource Definitions (CRDs)

### Paladin CRD

```yaml
apiVersion: core.paladin.io/v1alpha1
kind: Paladin
metadata:
  name: <node-name>
spec:
  config: |
    # Any YAML can go here that is valid in the configuration of Paladin.
    # This can be combined with the below utilities that provide K8s specific utilities for
    # generation of parts of the configuration that use Kubernetes secrets, volumes etc.
```

- **Spec**:
  - `paladinConfig`: The configuration used by the Paladin instance. It reflects the structure of `config.paladin.yaml`.
  
- **Status**:
    ???? Do we need a status? (maybe pod name) ????

### SmartContractDeployment CRD

```yaml
apiVersion: core.paladin.io/v1alpha1
kind: SmartContractDeployment
metadata:
  name: <contract-name>
spec:
  solidityCode: |
    contract SimpleStorage {
        uint storedData;
        function set(uint x) public {
            storedData = x;
        }
        function get() public view returns (uint) {
            return storedData;
        }
    }
  target: <paladin|besu>
status:
  state: <Pending|Success|Failed|Rejected>
  transactionID: <transaction-id>
  message: <status-message>
``` 

- **Status**:
  - `state`: The current state of the contract submission (e.g., Pending, Submitted, Completed, Failed).
  - `transactionID`: The transaction ID after the contract is submitted.
  - `message`: Status or error messages.

## Operator Flow Diagrams

### Node CR Workflow

```mermaid

graph TD
    A[User creates Node CR] --> B[Operator fetches CR]
    B --> C[Generate ConfigMap from Node spec]
    C --> D[Create StatefulSet with ConfigMap as volume]
    D --> E[Start StatefulSet with Paladin instance]

```

### SmartContractDeployment CR Workflow

```mermaid

graph TD
    A[User creates SmartContractDeployment CR] --> B[Operator fetches CR]
    B --> C[Compile and submit contract via Besu/Paladin API]
    C --> D[Set SmartContractDeployment status to Pending]
    D --> E[Set SmartContractDeployment status with transaction ID]
    E --> F[Wait for transaction confirmation]
    F --> G{Transaction successful?}
    G -->|Yes| H[Set SmartContractDeployment status to Success]
    G -->|No| I[Set SmartContractDeployment status to Failed]

```

## Status Handling
 

 