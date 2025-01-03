# Example: Bond Issuance

This example demonstrates a bond issuance scenario on Paladin. It leverages a combination of Noto tokens and
Pente private smart contracts in order to control visibility of various aspects of the bond issuance process.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/bond-issuance/) for a detailed explanation.

## Pre-requisites

Requires a local 3-node Paladin cluster running on `localhost:31548`, `localhost:31648`, and `localhost:31748`.

## Run standalone

Compile [Solidity contracts](../../solidity):

```shell
cd ../../solidity
npm install
npm run compile
```

Build [TypeScript SDK](../../sdk/typescript):

```shell
cd ../../sdk/typescript
npm install
npm run abi
npm run build
```

Run example:

```shell
npm install
npm run abi
npm run start
```

## Run with Gradle

The following will perform all pre-requisites and then run the example:

```shell
../../gradlew build
npm run start
```
