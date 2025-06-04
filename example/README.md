# Paladin Examples

These examples demonstrate the various areas of function provided by Paladin.

Every example has its own readme and setup instructions. The following setup is common to most examples and sets up the SDKs and common requirements ready to run any of the examples.

See the [tutorials](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/) for more information.

### Set up a local Paladin network

All of the samples require a local 3-node Paladin network running on `localhost:31548`, `localhost:31648`, and `localhost:31748`. You can get one setup by referring to the [Getting Started](https://lf-decentralized-trust-labs.github.io/paladin/head/getting-started/installation/) guide.

This should provision a local kubernetes (based on Kind) cluster and provision a Besu and Paladin network including these pods:

```shell
$ kubectl get po
NAME                               READY   STATUS    RESTARTS   AGE
besu-node1-0                       1/1     Running   0          19m
besu-node2-0                       1/1     Running   0          19m
besu-node3-0                       1/1     Running   0          19m
paladin-node1-0                    2/2     Running   0          17m
paladin-node2-0                    2/2     Running   0          17m
paladin-node3-0                    2/2     Running   0          17m
paladin-operator-bc788db4f-mzbs7   1/1     Running   0          19m
```

### Option 1 - npm (downloading the latest stable smart contracts and ABIs)

1. **Download the contracts**

   - Download the [latest stable solidity contracts](https://github.com/LF-Decentralized-Trust-labs/paladin/releases/latest/download/abis.tar.gz) to the paladin root directory.

2. **Build TypeScript SDK**

   ```shell
   cd <paladin-root>/sdk/typescript
   npm install
   npm run abi
   npm run build
   ```

3. **Build common utilities**

   ```shell
   cd <paladin-root>/example/common
   npm install
   npm run build
   ```

### Option 2 - npm (building contracts and ABIs locally)

1. **Compile Solidity contracts**

   ```shell
   cd <paladin-root>/solidity
   npm install
   npm run compile
   ```

2. **Build TypeScript SDK**

   ```shell
   cd <paladin-root>/sdk/typescript
   npm install
   npm run abi
   npm run build
   ```

3. **Build common utilities**

   ```shell
   cd <paladin-root>/example/common
   npm install
   npm run build
   ```

### Option 3 - Gradle

If you are using gradle, every example provides a gradle task that completes the setup steps ready to run the example.
