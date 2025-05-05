# Example: ZKP Fungible Tokens with Zeto

This example demonstrates a privacy-preserving fungible tokens using [Zeto](https://github.com/hyperledger-labs/zeto) on Paladin.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/zkp-cbdc/) for a detailed explanation.

## Pre-requisites

### Local Paladin network

This sample requires a local 3-node Paladin network running on `localhost:31548`, `localhost:31648`, and `localhost:31748`. You can get one setup by referring to the [Getting Started](https://lf-decentralized-trust-labs.github.io/paladin/head/getting-started/installation/) guide.

This should provision a local kubernetes (based on Kind) cluster and provision a Besu and Paladin network including these pods:

```console
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

### ABI resources for Zeto

The sample uses the following ABIs to accomplish the end to end flow:

- IZetoFungible.json: private transaction ABI for the Zeto domain, to conduct private transactions like `mint`, `transfer`, `lock`, etc.
- Zeto_Anon.json: public transaction ABI for the Zeto token implementation, to conduct public transactions "delegateLock`
- SampleERC20.json: public transaction ABI for a sample ERC20 token, to conduct public transactions to mint ERC20 tokens

These dependency resources can be obtained by running the following builds:

```console
$ ./gradlew :sdk:typescript:build
$ ./gradlew :example:zeto:build
```

## Run the example

Run the example with the following command from inside the `example/zeto` folder:

```console
cd example/zeto
npm install
npm run start

Use case #1: Privacy-preserving CBDC token, using private minting...
- Deploying Zeto token...
Success! address: 0xc4e831f1f0d59356d6f11e84a90c0609720edd4d
- Issuing CBDC to bank1 and bank2 with private minting...
Success!
- Bank1 transferring CBDC to bank2 to pay for some asset trades ...
Success!

Use case #1 complete!

Use case #2: Privacy-preserving CBDC token, using public minting of an ERC20 token...
- Deploying Zeto token...
Success! address: 0x8eec172314970dda6d55e1613e778406781e3b36
- Deploying ERC20 token to manage the CBDC supply publicly...
Success!
  ERC20 deployed at: 0x4ac689607d88c813db9ef6ab6a453eab88291b39
- Setting ERC20 to the Zeto token contract ...
Success!
- Issuing CBDC to bank1 with public minting in ERC20...
Success!
- Bank1 approve ERC20 balance for the Zeto token contract as spender, to prepare for deposit...
Success!
- Bank1 deposit ERC20 balance to Zeto ...
Success!
- Bank1 transferring CBDC to bank2 to pay for some asset trades ...
Success!
- Bank1 withdraws Zeto back to ERC20 balance ...
Success!

Use case #2 complete!
```
