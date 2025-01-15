# Example: Notarized-Tokens

This tutorial is meant to get familiar with how to create and manage a basic token using the **Notarized Tokens (noto)** domain.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/notarized-tokens/) for a detailed explanation.

## Pre-requisites

Requires a local 3-node Paladin cluster running on `localhost:31548`, `localhost:31648`, and `localhost:31748`.

## Run standalone

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
npm run start
```

## Run with Gradle

The following will perform all pre-requisites and then run the example:

```shell
../../gradlew build
npm run start
```
