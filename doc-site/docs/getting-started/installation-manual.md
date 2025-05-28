# Paladin Manual Installation Guide

This guide provides detailed instructions for manually installing a Paladin Network by directly configuring the Custom Resources (CRs) provided by the Paladin operator. This approach offers maximum flexibility for customizing network configuration, node distribution, blockchain integration, and smart contract management.

## Prerequisites

Ensure the following are installed:

* [Helm v3](https://helm.sh/docs/intro/install/)
* [kubectl](https://kubernetes.io/docs/tasks/tools/)

## Step 1: Install Paladin Operator CRDs

Add the Paladin Helm repository:

```bash
helm repo add paladin https://LF-Decentralized-Trust-labs.github.io/paladin --force-update
helm upgrade --install paladin-crds paladin/paladin-operator-crd
```

## Step 2: Install cert-manager CRDs

Install the [cert-manager](https://artifacthub.io/packages/helm/cert-manager/cert-manager) required by Paladin:

```bash
helm repo add jetstack https://charts.jetstack.io --force-update
helm install cert-manager --namespace cert-manager --version v1.16.1 jetstack/cert-manager --create-namespace --set crds.enabled=true
```

## Step 3: Install Paladin Operator

Install the Paladin operator in `operator-only` mode (without additional resources):

```bash
helm upgrade --install paladin paladin/paladin-operator -n paladin --create-namespace --set mode=operator-only
```

## Step 4: Deploy Smart Contract Artifacts

Download the latest release artifacts:

```bash
wget https://github.com/LF-Decentralized-Trust-labs/paladin/releases/download/latest/artifacts.tar.gz
tar -xzvf artifacts.tar.gz
```

Apply the self-signed certificate issuer in all Kubernetes clusters:

```bash
kubectl -n paladin apply -f cert_issuer_selfsigned.yaml
```


On the primary Kubernetes cluster (designated for smart contract deployment), apply the smart contract resources:

The contents of these CRs should not be modified, with the exception of
* `spec.node`- the name of the node that will be used to submit the transactions. Change this if you wish to name your first node something other than `node1`.
* `spec.from`- the identifier of the key that will be used to sign the transactions. Change this if you wish to use a specific key from your signer for this purpose.

```bash
kubectl -n paladin apply -f core_v1alpha1_paladinregistry.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_noto_factory.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_pente_factory.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_registry.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_factory.yaml
# Apply other provided Zeto-related smart contracts similarly
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_batch.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_enc_batch.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_enc.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_nullifier_transfer_batch.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_nullifier_transfer.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_deposit.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_withdraw_batch.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_withdraw_nullifier_batch.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_withdraw_nullifier.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_withdraw.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_impl_anon_enc.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_impl_anon_nullifier.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_impl_anon.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_poseidon_unit2l.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_poseidon_unit3l.yaml
kubectl -n paladin apply -f core_v1alpha1_smartcontractdeployment_zeto_smt_lib.yaml
kubectl -n paladin apply -f core_v1alpha1_transactioninvoke_zeto_register_anon_enc.yaml
kubectl -n paladin apply -f core_v1alpha1_transactioninvoke_zeto_register_anon_nullifier.yaml
kubectl -n paladin apply -f core_v1alpha1_transactioninvoke_zeto_register_anon.yaml
```

**Note:** Smart contracts won't be fully deployed until Step 7.

## Step 5: Create an EVM Registry

Create the registry CR. For the primary cluster, use a deployment name; subsequent clusters should reference the existing contract address:

> The `paladin.io/registry-name` label is significant as this what Paladin nodes will use to reference the registry.

```yaml
apiVersion: core.paladin.io/v1alpha1
kind: PaladinRegistry
metadata:
  name: evm-registry
  namespace: paladin
  labels:
    paladin.io/registry-name: evm-registry
spec:
  type: evm
  evm:
    smartContractDeployment: registry  # use 'contractAddress' if referencing existing deployment
    # contractAddress: "0x...."
  plugin:
    library: /app/registries/libevm.so
    type: c-shared
```

## Step 6: Deploy Domains

Deploy Paladin domains (`noto`, `pente`, `zeto`). Similar to the registry, reference the deployment name initially and contract address for subsequent clusters:

```yaml
apiVersion: core.paladin.io/v1alpha1
kind: PaladinDomain
metadata:
  name: noto
  namespace: paladin
  labels:
    paladin.io/domain-name: noto
spec:
  plugin:
    library: /app/domains/libnoto.so
    type: c-shared
  smartContractDeployment: noto-factory  # use 'contractAddress' for existing deployments
  # contractAddress: "0x...."

```

Repeat for `pente` and `zeto` domains.

<details><summary>pente</summary>
```yaml
apiVersion: core.paladin.io/v1alpha1
kind: PaladinDomain
metadata:
  labels:
    paladin.io/domain-name: pente
  name: pente
  namespace: paladin
spec:
  plugin:
    class: io.kaleido.paladin.pente.domain.PenteDomainFactory
    library: /app/domains/pente.jar
    type: jar
  smartContractDeployment: pente-factory
```
</details>

<details><summary>zeto</summary>
```yaml
apiVersion: core.paladin.io/v1alpha1
kind: PaladinDomain
metadata:
  labels:
    paladin.io/domain-name: zeto
  name: zeto
  namespace: paladin
spec:
  allowSigning: true
  configJSON: |
    {
      "domainContracts": {
        "implementations": [
          {
            "name": "Zeto_Anon",
            "circuits": {
              "deposit": {
                "name": "deposit"
              },
              "withdraw": {
                "name": "withdraw"
              },
              "transfer": {
                "name": "anon"
              },
              "transferLocked": {
                "name": "anon"
              }
            }
          },
          {
            "name": "Zeto_AnonEnc",
            "circuits": {
              "deposit": {
                "name": "deposit"
              },
              "withdraw": {
                "name": "withdraw"
              },
              "transfer": {
                "name": "anon_enc",
                "usesEncryption": true
              },
              "transferLocked": {
                "name": "anon_enc",
                "usesEncryption": true
              }
            }
          },
          {
            "name": "Zeto_AnonNullifier",
            "circuits": {
              "deposit": {
                "name": "deposit"
              },
              "withdraw": {
                "name": "withdraw_nullifier",
                "usesNullifiers": true
              },
              "transfer": {
                "name": "anon_nullifier_transfer",
                "usesNullifiers": true
              },
              "transferLocked": {
                "name": "anon_nullifier_transferLocked",
                "usesNullifiers": true
              }
            }
          }
        ]
      },
      "snarkProver": {
        "circuitsDir": "/app/domains/zeto/zkp",
        "provingKeysDir": "/app/domains/zeto/zkp"
      }
    }
  plugin:
    library: /app/domains/libzeto.so
    type: c-shared
  smartContractDeployment: zeto-factory
```
</details>

## Step 7: Create Paladin Nodes

Deploy Paladin nodes with unique names. The first node (`node1`) handles initial smart contract deployment transactions:

> Paladin node names need to be unique within the network. The Paladin node that is used to deploy the smart contracts must be called `node1`. 

```yaml
apiVersion: core.paladin.io/v1alpha1
kind: Paladin
metadata:
  name: node1
  namespace: paladin
spec:
  baseLedgerEndpoint:
    type: endpoint
    endpoint:
      jsonrpc: https://chain-json-rpc-endpoint
      ws: wss://chain-json-ws-endpoint
      auth:
        type: secret
        secret:
          name: node1-auth
  config: |
    log:
      level: debug
  database:
    migrationMode: auto
    mode: sidecarPostgres
  domains:
  - labelSelector:
      matchLabels:
        paladin.io/domain-name: noto
  - labelSelector:
      matchLabels:
        paladin.io/domain-name: zeto
  - labelSelector:
      matchLabels:
        paladin.io/domain-name: pente
  registries:
  - labelSelector:
      matchLabels:
        paladin.io/registry-name: evm-registry
  secretBackedSigners:
  - keySelector: .*
    name: signer-1
    secret: node1.keys
    type: autoHDWallet
  service:
    ports:
    - name: rpc-http
      nodePort: 31548
      port: 8548
      protocol: TCP
    - name: rpc-ws
      nodePort: 31549
      port: 8549
      protocol: TCP
    type: NodePort
  transports:
  - configJSON: |
      {
        "port": 9000,
        "address": "0.0.0.0"
      }
    name: grpc
    plugin:
      library: /app/transports/libgrpc.so
      type: c-shared
    ports:
    - name: transport-grpc
      port: 9000
      protocol: TCP
      targetPort: 9000
    tls:
      certName: paladin-node1-mtls
      issuer: selfsigned-issuer
      secretName: paladin-node1-mtls
```

**Adjust configuration fields as needed:**

### `baseLedgerEndpoint`
```yaml
  baseLedgerEndpoint:
    type: endpoint
    endpoint:
      jsonrpc: https://chain-json-rpc-endpoint
      ws: wss://chain-json-ws-endpoint
      auth:
        type: secret
        secret:
          name: node1-auth
```

Configure the JSON RPC and websocket endpoints for the blockchain node you wish this Paladin node to connect to.

If the blockchain node is secured with basic auth, you may specify a secret that contains the username and password.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: node1-auth
  namespace: paladin
data:
  password: ...
  username: ...
type: Opaque
```

### `config`
This section contains an inline yaml/json string which can be used to provide Paladin node configuration that isn't exposed directly by the CRD. If the same configuration value is provided in the inline config and a CR field, the CR field will take precedence.

[This code file](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/config/pkg/pldconf/config.go) defines the full set of Paladin configuration.

### `database`
```yaml
database:
  migrationMode: auto
  mode: sidecarPostgres
```
`mode` may be `sidecarPostgres` or `embeddedSQLite`

### `domains`
```yaml
domains:
- labelSelector:
    matchLabels:
      paladin.io/domain-name: noto
- labelSelector:
    matchLabels:
      paladin.io/domain-name: zeto
- labelSelector:
    matchLabels:
      paladin.io/domain-name: pente
```
The domain name labels provided here must match the labels on the domains created in step 6.

### `registries`
```yaml
registries:
- labelSelector:
    matchLabels:
      paladin.io/registry-name: evm-registry
```
The registry name label provided here must match the label on the registry created in step 5.

### `secretBackedSigners`
To use a HD wallet with a seed phrase generated by the Paladin operator, set `type: autoHDWallet` and provide the name of a secret you wish the seed phrase to be stored into, but which doesn't exist yet.

```yaml
secretBackedSigners:
- keySelector: .*
  name: signer-1
  secret: node1.keys
  type: autoHDWallet
```

If you wish to use an existing seed phrase, store a file containing this seed phrase in a secret, and reference it in the signer configuration.

```yaml
seed:
  inline: manual rabbit frost hero squeeze adjust link crystal filter purchase fruit border coin able tennis until endless crisp scout figure wage finish aisle rabbit
```

```bash
kubectl create secret generic <secret name> --from-file=keys.yaml=<file name>
```

```yaml
secretBackedSigners:
- keySelector: .*
  name: signer-1
  secret: node1.keys
```

### `service`
```yaml
service:
  ports:
  - name: rpc-http
    nodePort: 31548
    port: 8548
    protocol: TCP
  - name: rpc-ws
    nodePort: 31549
    port: 8549
    protocol: TCP
  type: NodePort
```
This section is used directly to create a Kubernetes service to expose the Paladin HTTP and WS servers. See the [Kubernetes documentation](https://kubernetes.io/docs/concepts/services-networking/service/) to understand how to configure this section.

The Paladin http and ws servers run on ports `8548` and `8549` by default. 

### `transports`
```yaml
transports:
- configJSON: |
    {
      "port": 9000,
      "address": "0.0.0.0",
      "externalHostname": ...
    }
  name: grpc
  plugin:
    library: /app/transports/libgrpc.so
    type: c-shared
  ports:
  - name: transport-grpc
    port: 9000
    protocol: TCP
    targetPort: 9000
  tls:
    certName: paladin-node1-mtls
    issuer: selfsigned-issuer
    secretName: paladin-node1-mtls
```

`configJSON` is the json/yaml configuration that is passed to the transport plugin. Specifically for the grpc transport plugin
* `port`- this must match `targetPort` in the `ports` configuration
* `externalHostname` - this can be set if the Paladin node needs to be accessible from outside the cluster. The value will depend on how ingress in configured, which is outside the scope of the Paladin project. If not set it defaults to the internal hostname, which is adequate if all Paladin nodes are running in the same cluster

The certificates generated to match the `tls` configuration section is added in by the operator to the configuration for the transport plugin.

## Step 8: Register Paladin Nodes

Create `PaladinRegistration` CRs for each Paladin node:


* `spec.registryAdminKey`: the identifier for the key that will be used to sign the root registration transaction
* `spec.nodeAdminKey`: the identifier for the key that is used as the owner of the registration, and used to register transports
These keys come from the signer configured on the node being registered.

```yaml
apiVersion: core.paladin.io/v1alpha1
kind: PaladinRegistration
metadata:
  name: node1
spec:
  node: node1 # <node name>
  nodeAdminKey: registry.node1 # registry.<node name>
  registry: evm-registry
  registryAdminKey: registry.operator
  registryAdminNode: node1 # <admin node name>
  transports:
  - grpc
```

Repeat for all nodes created in Step 7.

## Multi-cluster Considerations

When deploying across multiple clusters:

* Follow Steps 1-8 on the primary cluster.
* On additional clusters, reference the existing contract addresses obtained from the primary cluster (Steps 5-6).

