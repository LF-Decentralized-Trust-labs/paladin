# Example: Lock Controller

This example demonstrates programmable lock + unlock logic on Noto. Each user that locks some of their
Noto tokens can specify a base ledger contract to handle the lock + unlock. In this example, the base
ledger contract deploys an ERC20 representing the value of the lock. The owner may transfer those
ERC20 tokens as they wish, and the recipients of the ERC20 tokens may then unlock the value to claim
it back as Noto.

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
