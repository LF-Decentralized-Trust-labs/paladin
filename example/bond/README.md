# Example: Bond Issuance

This example demonstrates a bond issuance scenario on Paladin. It leverages a combination of Noto tokens and
Pente private smart contracts in order to control visibility of various aspects of the bond issuance process.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/bond-issuance/) for a detailed explanation.

## Pre-requisites

Run the common [setup steps](../README.md) before running the example.

## Running the example

### Option 1 - `npm` with downloaded solidity contracts

- **Extract contracts**

  - Extract the `abis.tar.gz` file that was downloaded in the [setup steps](../README.md) and copy the `abis` directory to `src/`. (full path should be: `paladin/example/bond/src/abis`)

- **Run the example**

  ```shell
  cd <paladin-root>/example/bond
  npm install
  npm run start
  ```

---

### Option 2 - `npm` with locally built solidity contracts and ABIs

- **Run the example**

  ```shell
  cd <paladin-root>/example/bond
  npm install
  npm run abi
  npm run start
  ```

---

### Option 3 - Run with Gradle

- **Run the gradle build task, then run the sample:**

  ```shell
  cd <paladin-root>/example/bond
  ../../gradlew build
  npm run start
  ```
