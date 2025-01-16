# Example: Atomic Swap

This example demonstrates an atomic swap scenario on Paladin. It performs a swap between:

- a cash token implemented with Zeto
- an asset token implemented with Noto, with private hooks implemented on Pente

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
