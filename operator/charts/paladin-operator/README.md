# Paladin Operator Helm Chart

This chart is part of the larger Paladin ecosystem. For comprehensive setup and usage instructions, please refer to the [main Paladin documentation](https://LFDT-Paladin.github.io/paladin/head/getting-started/installation/).

## Node Configuration

The chart supports flexible node configuration with two independent parameters:

### Paladin Nodes
Set the number of Paladin nodes to deploy:

```yaml
nodeCount: 3  # Deploys 3 Paladin nodes
```

### Besu Nodes  
Set the number of Besu nodes to deploy:

```yaml
besuNodeCount: 2  # Deploys 2 Besu nodes
```

### Node Connectivity Logic
- **Single Besu node**: All Paladin nodes connect to the first Besu node (`node1`)
- **Multiple Besu nodes**: Uses modulo arithmetic to distribute Paladin nodes across available Besu nodes
  - **Example with 3 Paladin nodes and 2 Besu nodes**:
    - Paladin node 1 → Besu node 1
    - Paladin node 2 → Besu node 2  
    - Paladin node 3 → Besu node 1
  - This creates load distribution where some Besu nodes may handle multiple Paladin nodes

## Auto-Generated CRs

**Important**: Some of the CRs deployed by this chart are **auto-generated during the chart release process** and therefore cannot be found in the `templates/` directory of this repository.

### Generation Process

The chart uses an automated build process to generate smart contract-related CRs:

1. **Smart Contract Processing**: The [`../../contractpkg/main.go`](../../contractpkg/main.go) program processes compiled smart contract artifacts and generates corresponding Kubernetes CRs.

2. **Template Generation**: During the build process (see [`../../Makefile`](../../Makefile)), the tool:
   - Reads smart contract build artifacts
   - Generates `SmartContractDeployment` CRs
   - Creates `PaladinDomain` and `PaladinRegistry` CRs
   - Applies appropriate templating for different installation modes
   - Packages everything into the final Helm chart

3. **Build Integration**: The Makefile targets like `prepare-crd-chart` and `helm-install-dependencies` orchestrate this generation process.

