
# Installation: custom

This guide describes how to install a Paladin Network by directly configures the Custom Resources that the Paladin operator provides. This allows for flexibility in how many nodes are in the network, which EVM chain is used, and whether those nodes all run in a single Kubernetes cluster or are distributed across multiple clusters. It assumes that you are using a HD Wallet signer, the `selfsigned-issuer` from `cert-manager`, and that the contracts/plugins that the Paladin project provides for the EVM registry, GRPC transport, and Noto, Pente and Zeto domains. 

In the case that nodes are distributed across multiple Kubernetes clusters, the same instructions need to be followed on all of them. However, one cluster needs to be designated to deploy the smart contracts that the Paladin Network requires, and this cluster needs to be installed first. Resources created in the other clusters will reference the addresses of the smart contracts deployed by this first cluster.

## Pre-requisites

* [helm](https://helm.sh/) `v3` installed
* [kubectl](https://kubernetes.io/docs/reference/kubectl/) installed

## Step 1: Install the CRD Chart
   
Install the CRD chart that contains the necessary Custom Resource Definitions (CRDs) for the Paladin operator:

```bash
helm repo add paladin https://LF-Decentralized-Trust-labs.github.io/paladin --force-update
helm upgrade --install paladin-crds paladin/paladin-operator-crd
```

## Step 2: Install cert-manager CRDs

Install the [cert-manager](https://artifacthub.io/packages/helm/cert-manager/cert-manager) CRDs:

```bash
helm repo add jetstack https://charts.jetstack.io --force-update
helm install cert-manager --namespace cert-manager --version v1.16.1 jetstack/cert-manager --create-namespace --set crds.enabled=true
```

## Step 3: Install operator in `none` mode

Install the Paladin operator chart:

```bash
helm upgrade --install paladin paladin/paladin-operator -n paladin --create-namespace --set mode=none
```

## Step 4: Unpack and apply release artifacts

Download and extract the artifacts for the latest release from https://github.com/LF-Decentralized-Trust-labs/paladin/releases/download/latest/artifacts.tar.gz.

In all clusters apply the selfsigned cert issuer.
```bash
kubectl apply -f cert_issuer_selfsigned.yaml
```

If this is the Kubernetes cluster that will be used to deploy smart contracts, also apply the registry and domain smart contract deployments as well as the transaction invokes required for configuring zeto.
```bash
kubectl apply -f core_v1alpha1_paladinregistry.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_noto_factory.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_pente_factory.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_registry.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_factory.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_batch.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_enc_batch.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_enc.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_nullifier_transfer_batch.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon_nullifier_transfer.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_anon.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_deposit.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_withdraw_batch.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_withdraw_nullifier_batch.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_withdraw_nullifier.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_g16_withdraw.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_impl_anon_enc.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_impl_anon_nullifier.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_impl_anon.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_poseidon_unit2l.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_poseidon_unit3l.yaml
kubectl apply -f core_v1alpha1_smartcontractdeployment_zeto_smt_lib.yaml
kubectl apply -f core_v1alpha1_transactioninvoke_zeto_register_anon_enc.yaml
kubectl apply -f core_v1alpha1_transactioninvoke_zeto_register_anon_nullifier.yaml
kubectl apply -f core_v1alpha1_transactioninvoke_zeto_register_anon.yaml
```

Note that the smart contracts will not actually be deployed until step 7 is complete, as a Paladin node is required to submit the deployment transactions.

The contents of these CRs should not be modified, with the exception of

* `spec.node`- the name of the node that will be used to submit the transactions. Change this if you wish to name your first node something other than `node1`.
* `spec.from`- the identifier of the key that will be used to sign the transactions. Change this if you wish to use a specific key from your signer for this purpose.

## Step 5: Create an EVM Registry

Create an EVM registry that uses the `registry` smart contract deployment created in step 3. The examples below are suitable for the first Kubernetes cluster that is used to deploy the smart contracts. For subsequent clusters, `spec.evm.smartContractDeployment` should be replaced with `spec.evm.contractAddress`. The value of the contract address can found by running `kubectl get paladinregistry` on the first cluster once all steps in this guide have been completed for the first cluster.

The value of `metadata.lavels.paladin.io/registry-name` is significant as this what Paladin nodes will use to reference the registry.


```yaml
apiVersion: core.paladin.io/v1alpha1
kind: PaladinRegistry
metadata:
  labels:
    paladin.io/registry-name: evm-registry
  name: evm-registry
  namespace: paladin
spec:
  type: evm
  evm:
    smartContractDeployment: registry
  plugin:
    library: /app/registries/libevm.so
    type: c-shared
```

## Step 6: Create Domains

Create Noto, Pente, and Zeto domains, which reference the `noto-factory`, `pente-factory`, and `zeto-factory` smart contract deployments created in step 1.

The examples below are suitable for the first Kubernetes cluster that is used to deploy the smart contracts. For subsequent clusters, `spec.smartContractDeployment` should be replaced with `spec.registryAddress`. The value of the contract address can found by running `kubectl get paladindomain` on the first cluster once all steps in this guide have been completed for the first cluster.

```bash
kubectl get paladindomain
NAME    STATUS      DOMAIN_REGISTRY                              DEPLOYMENT      LIBRARY
noto    Available   0x2681852a96b053746ff2f2f0bb94c3fbe1d63e7e   noto-factory    /app/domains/libnoto.so
pente   Available   0x4ea4549eca420802f1d74606775370063b7bcc70   pente-factory   /app/domains/pente.jar
zeto    Available   0x6ff1c15409614ad89b67baa6018d3499ef779e0b   zeto-factory    /app/domains/libzeto.so
```

The value of `metadata.lavels.paladin.io/domain-name` is significant as this what Paladin nodes will use to reference the domains.

```yaml
apiVersion: core.paladin.io/v1alpha1
kind: PaladinDomain
metadata:
  labels:
    paladin.io/domain-name: noto
  name: noto
  namespace: paladin
spec:
  plugin:
    library: /app/domains/libnoto.so
    type: c-shared
  smartContractDeployment: noto-factory
```
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

## Step 7: Create Paladin Nodes

Create as many Paladin nodes as you wish to run in this Kubernetes cluster. More nodes can be added at a later point by following this step and step 8.

Paladin node names need to be unique within the network. The Paladin node that is used to deploy the smart contracts must be called `node1`. 

The full specification for the Paladin node CRD is available [here](../reference/crds/core.paladin.io/#paladin); however, there are detailed explanations for the most relevant sections in the example below.

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

## Step 8: Create Registrations

Register all the Paladin nodes created in step 7 with the registry created in step 5. Creating a PaladinRegistration CR will cause a registration transaction to be submitted to the node being registered.

This CR references two keys

* `spec.registryAdminKey`: the identifier for the key that will be used to sign the root registration transaction
* `spec.nodeAdminKey`: the identifier for the key that is used as the owner of the registration, and used to register transports
These keys come from the signer configured on the node being registered.

```yaml
apiVersion: core.paladin.io/v1alpha1
kind: PaladinRegistration
metadata:
  name: node1
  namespace: paladin
spec:
  node: node1
  nodeAdminKey: registry.node1
  registry: evm-registry
  registryAdminKey: registry.operator
  registryAdminNode: node1
  transports:
  - grpc
```






