# Example: ZKP Fungible Tokens with Zeto

This example demonstrates a privacy-preserving fungible tokens using [Zeto](https://github.com/hyperledger-labs/zeto) on Paladin.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/zkp-cbdc/) for a detailed explanation.

## Pre-requisites

Run the common [setup steps](../README.md) before running the example.

## Running the example

### ABI resources for Zeto

The sample uses the following ABIs to accomplish the end to end flow:

- IZetoFungible.json: private transaction ABI for the Zeto domain, to conduct private transactions like `mint`, `transfer`, `lock`, etc.
- Zeto_Anon.json: public transaction ABI for the Zeto token implementation, to conduct public transactions "delegateLock`
- SampleERC20.json: public transaction ABI for a sample ERC20 token, to conduct public transactions to mint ERC20 tokens

These dependency resources can be obtained by one of the following ways.

### Option 1 - `npm` with downloaded solidity contracts

- **Extract contracts**

  - Extract the `abis.tar.gz` file that was downloaded in the [setup steps](../README.md) and copy the `abis` directory to `src/`. (full path should be: `paladin/example/zeto/src/abis`)

- **Run the example**

  ```shell
  cd <paladin-root>/example/zeto
  npm install
  npm run start
  ```

---

### Option 2 - `npm` with locally built solidity contracts and ABIs

- **Run the example**

  ```shell
  cd <paladin-root>/example/zeto
  npm install
  npm run abi
  npm run start
  ```

---

### Option 3 - Run with Gradle

- **Run the gradle build task, then run the sample:**

  ```shell
  cd <paladin-root>/example/zeto
  ../../gradlew build
  npm run start
  ```

### Verify Dependent Zeto Resources

You can verify that the dependent ABIs have been successfully put in place by the build:

```shell
$ ls <paladin-root>/sdk/typescript/build/domains/abis/
INoto.json		INotoPrivate.json	IZetoFungible.json	PentePrivacyGroup.json	Zeto_Anon.json
$ ls <paladin-root>/example/zeto/src/abis/
SampleERC20.json
```

## Run the example

Run the example with the following command from inside the `example/zeto` folder:

```shell
cd <paladin-root>/example/zeto
npm run start
```
