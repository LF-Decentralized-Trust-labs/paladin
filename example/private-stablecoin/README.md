# Example: Private Stablecoin with KYC and Nullifiers

This example demonstrates a privacy-preserving stablecoin using Zeto with KYC compliance and nullifiers for enhanced security and regulatory oversight.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/private-stablecoin/) for a detailed explanation.

## Pre-requisites

Run the common [setup steps](../README.md) before running the example.

## Running the example

### ABI resources for Zeto

The sample uses the following ABIs to accomplish the end-to-end flow:

- `IZetoFungible.json`: Private transaction ABI for the Zeto domain, to conduct private transactions like `mint`, `transfer`, etc.
- `Zeto_AnonNullifierKyc.json`: Public transaction ABI for the Zeto KYC token implementation
- `SampleERC20.json`: Public transaction ABI for ERC20 tokens (if needed for deposit/withdraw operations)

These dependency resources can be obtained by one of the following ways.

### Option 1 - `npm` with downloaded solidity contracts

- **Extract contracts**

  - Extract the `abis.tar.gz` file that was downloaded in the [setup steps](../README.md) and copy the `abis` directory to `src/`. (full path should be: `paladin/example/private-stablecoin/src/abis`)

- **Run the example**

  ```shell
  cd <paladin-root>/example/private-stablecoin
  npm install
  npm run start
  ```

---

### Option 2 - `npm` with locally built solidity contracts and ABIs

- **Run the example**

  ```shell
  cd <paladin-root>/example/private-stablecoin
  npm install
  npm run abi
  npm run start
  ```

---

### Option 3 - Run with Gradle

- **Run the gradle build task, then run the sample:**

  ```shell
  cd <paladin-root>/example/private-stablecoin
  ../../gradlew build
  npm run start
  ```

### Verify Dependent Zeto Resources

You can verify that the dependent ABIs have been successfully put in place by the build:

```shell
$ ls <paladin-root>/sdk/typescript/build/domains/abis/
INoto.json		INotoPrivate.json	IZetoFungible.json	PentePrivacyGroup.json	Zeto_AnonNullifierKyc.json
$ ls <paladin-root>/example/private-stablecoin/src/abis/
SampleERC20.json
```