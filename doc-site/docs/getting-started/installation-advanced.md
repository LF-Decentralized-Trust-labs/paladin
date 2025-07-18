# Paladin Advanced Installation Guide

This guide covers advanced installation options for deploying Paladin using Helm, providing detailed control over various configuration options for different deployment scenarios.

# Installation Modes

1. [**devnet** (default)](#mode-devnet-default) — Sandbox/demo network, ideal for a quick start and getting a first taste of Paladin.
2. [**customnet**](#mode-customnet) — Full control: explicitly configure each Paladin node and its settings.
3. [**attach**](#mode-attach) — Connect to an existing Paladin network and reuse deployed contracts.
4. [**operator-only**](#mode-operator-only) — Install only the operator, then apply CRs manually.


## Mode: `devnet` (default)

Deploys a complete, ready-to-use Paladin network including domains and smart contract resources with default settings (3 nodes).

Default installation:

```bash
helm install paladin paladin/paladin-operator -n paladin --create-namespace
```

Refer to the provided [`values.yaml`](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/operator/charts/paladin-operator/values.yaml) for additional configurable options, including: `Number of nodes`, `node name prefixes`, `ports`, `images`, `environment variables`, etc.

Example custom installation:

```bash
helm install paladin paladin/paladin-operator -n paladin --create-namespace \
  --set nodeCount=5 \
  --set paladin.nodeNamePrefix=worker \
  --set besu.nodeNamePrefix=evm
```

## Mode: `customnet`

The `customnet` mode offers maximum flexibility, allowing detailed customization of `Paladin`, `Registry`, and `PaladinDomain` CRs. It is ideal for advanced use cases, such as integration with external blockchain nodes.

Declare each node explicitly for setups:

**Example `values-customnet.yaml`**:

```yaml
mode: customnet
paladinNodes:
  - name: central-bank
    baseLedgerEndpoint:
      type: endpoint       # or "local" for built-in Besu
      endpoint:
        jsonrpc: https://mychain.rpc
        ws:    wss://mychain.ws
        auth:
          enabled: false    # set true + secretName if basic auth
          secretName: creds
    domains:
      - noto
      - zeto
      - pente
    registries:
      - evm-reg
    transports:
      - name: grpc
        plugin:
          type: c-shared
          library: /app/transports/libgrpc.so
        config:
          port: 9000
          address: 0.0.0.0
        ports:
          transportGrpc:
            port: 9000
            targetPort: 9000
    service:
      type: NodePort    # change service type if needed
      ports:
        rpcHttp:
          port: 8548
          nodePort: 31748   # Do not set nodePort in case the service.type is not NodePort
        rpcWs:
          port: 8549
          nodePort: 31749   # Do not set nodePort in case the service.type is not NodePort
    database:
      mode: sidecarPostgres
      migrationMode: auto
      # pvcTemplate:         # Optional - Set a custom PVC for the DB
      #   storageClassName: "my-storage"
      #   accessModes:
      #   - ReadWriteOnce
      #   capacity:
      #     storage: 10Gi
    secretBackedSigners:
      - name: signer-auto-wallet
        secret: central-bank.keys
        type: autoHDWallet  # or preConfigured. in case of preConfigured you must create the secret with the seed
        keySelector: ".*"
    paladinRegistration:
      registryAdminNode: central-bank  # The admin node that manages the registry
      registryAdminKey: registry.operator
      registry: evm-registry
    config: |
      log:
        level: debug  # Log levels: debug, info, warn, error
      publicTxManager:
        gasLimit:
          gasEstimateFactor: 2.0
```


**Run**:

```bash
helm install paladin paladin/paladin-operator -n paladin --create-namespace \
  -f values-customnet.yaml
```

This will deploy one Paladin node named `bank`. Add more entries under `paladinNodes` to scale horizontally.

Refer to the provided [`values-customnet.yaml`](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/operator/charts/paladin-operator/values-customnet.yaml) for additional configurable options.

### Options
Here are some customizations that you will most likely make when in stalling with `customnet ` mode.

#### `baseLedgerEndpoint`

Configure the JSON RPC and websocket endpoints for the blockchain node you wish this Paladin node to connect to.
If the blockchain node is secured with basic auth, you may specify a secret that contains the username and password:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: central-bank-auth
  namespace: paladin
data:
  password: ...
  username: ...
type: Opaque
```

#### `transports.config.externalHostname`

`externalHostname` - this can be set if the Paladin node needs to be accessible from outside the cluster. The value will depend on how ingress in configured, which is outside the scope of the Paladin project. If not set it defaults to the internal hostname, which is adequate if all Paladin nodes are running in the same cluster

#### `secretBackedSigners`

Set `autoHDWallet` or `preConfigured` (must supply `keys.yaml`).
To use a HD wallet with a seed phrase generated by the Paladin operator, set `type: autoHDWallet` and provide the name of a secret you wish the seed phrase to be stored into, but which doesn't exist yet.

```yaml
secretBackedSigners:
- keySelector: .*
  name: signer-1
  secret: central-bank.keys
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
    secret: central-bank.keys
  ```


## Mode: `attach`

Set `mode=attach` when you have an existing blockchain network and previously deployed smart contracts that you want Paladin to reuse.
This mode will tipacly be in use after you already installed Paladin in `customnet` mode and now you want to run another paladin node in a different namespace/cluster and attach it to the already existing blockchain and paladin network.

### Retrieving Contract Addresses

First, get the registries addresses and paladinDomain addresses:

```bash
kubectl -n paladin get paladindomains,paladinregistries
```

Example output:

```
NAME                                  STATUS      DOMAIN_REGISTRY                              DEPLOYMENT      LIBRARY
paladindomain.core.paladin.io/noto    Available   0xd93630936d854fb718b89537cce4acc97fd50463   noto-factory    /app/domains/libnoto.so
paladindomain.core.paladin.io/pente   Available   0x48c11bbb7caa77329d53b89235fec64733a24ca1   pente-factory   /app/domains/pente.jar
paladindomain.core.paladin.io/zeto    Available   0xc29ed8a902ff787445bdabee9ae5e2380089959d   zeto-factory    /app/domains/libzeto.so

NAME                                           TYPE   STATUS      CONTRACT
paladinregistry.core.paladin.io/evm-registry   evm    Available   0x07f73d1d358fe9f178b0b8a749a99bf08e4e4140
```

and past it to the values file:

```yaml
smartContractsReferences:
  notoFactory:
    address: "0xd93630936d854fb718b89537cce4acc97fd50463"
  zetoFactory:
    address: "0xc29ed8a902ff787445bdabee9ae5e2380089959d"
  penteFactory:
    address: "0x48c11bbb7caa77329d53b89235fec64733a24ca1"
  registry:
    address: "0x4456307ef3f119dac17a5e974d2640f714e6edb0"
```

Also, make sure you configure the `paladinRegistration.registryAdminNode` so it will be the admin node (the node that deployed the smart contracts), like this:
```yaml
paladinRegistration:
  registryAdminNode: central-bank # the node name that deployed the smart contracts
  registryAdminKey:  ...
  registry:         ...
```

### Example `values-attach.yaml`:

Here is a full example of the `values-attach.yaml`:

```yaml
mode: attach
smartContractsReferences:
  notoFactory:
    address: "0xd93630936d854fb718b89537cce4acc97fd50463"
  zetoFactory:
    address: "0xc29ed8a902ff787445bdabee9ae5e2380089959d"
  penteFactory:
    address: "0x48c11bbb7caa77329d53b89235fec64733a24ca1"
  registry:
    address: "0x4456307ef3f119dac17a5e974d2640f714e6edb0"

paladinNodes:
paladinNodes:
- name: bank-a
  baseLedgerEndpoint:
    type: endpoint
    endpoint:
      jsonrpc: https://mychain.rpc
      ws:    wss://mychain.ws
      auth:
        enabled: false    # set true + secretName if basic auth
        secretName: creds
  domains:
    - noto
    - zeto
    - pente
  registries:
    - evm-reg
  transports:
    - name: grpc
      plugin:
        type: c-shared
        library: /app/transports/libgrpc.so
      config:
        port: 9000
        address: 0.0.0.0
      ports:
        transportGrpc:
          port: 9000
          targetPort: 9000
  service:
    type: NodePort    # change service type if needed
    ports:
      rpcHttp:
        port: 8548
        nodePort: 31748  # Do not set nodePort in case the service.type is not NodePort
      rpcWs:
        port: 8549
        nodePort: 31749  # Do not set nodePort in case the service.type is not NodePort
  database:
    mode: sidecarPostgres
    migrationMode: auto
  secretBackedSigners:
    - name: signer-auto-wallet
      secret: central-bank.keys
      type: autoHDWallet  # or preConfigured. in case of preConfigured you must create the secret with the seed
      keySelector: ".*"
  config: |
    log:
      level: debug
    publicTxManager:
      gasLimit:
        gasEstimateFactor: 2.0
  paladinRegistration:
    registryAdminNode: central-bank
    registryAdminKey:  registry.operator
    registry:         evm-registry
```

**Run**:

```bash
helm install paladin paladin/paladin-operator -n paladin --create-namespace \
  -f values-attach.yaml
```

### Mode: `operator-only`

Deploys only the Paladin operator without additional nodes or domains, useful for advanced scenarios or incremental setup:

```bash
helm install paladin paladin/paladin-operator --set mode=operator-only
```

## Advanced Customization

For users requiring direct application and full manual control, including complex multi-cluster setups, refer to the [manual installation documentation](./installation-manual.md). This approach involves manually configuring CRs, applying individual artifacts, and managing explicit node and domain settings.
 