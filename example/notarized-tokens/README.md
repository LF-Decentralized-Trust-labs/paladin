# Example: Notarized-Tokens

This tutorial is meant to get familiar with how to create and manage a basic token using the **Notarized Tokens (noto)** domain.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/notarized-tokens/) for a detailed explanation.

## Pre-requisites

Requires a local 3-node Paladin cluster running on `localhost:31548`, `localhost:31648`, and `localhost:31748`.

## Option 1: Use the Latest Stable Version

1. **Download and Extract Contracts**

   - [Download Solidity contracts](https://github.com/LF-Decentralized-Trust-labs/paladin/releases/latest/download/abis.tar.gz)
   - Extract `abis.tar.gz` and copy the `abis` directory to `src/`. (full path should be: `paladin/example/notarized-tokens/src/abis`)

2. **Build TypeScript SDK**

```shell
cp -rf src/abis ../../sdk/typescript/src/domains/
cd ../../sdk/typescript
npm install
npm run build
```

3. **Run the Example**

```shell
npm install
npm run start
```

---

## Option 2: Build Locally

1. **Compile Solidity Contracts**

```shell
cd ../../solidity
npm install
npm run compile
```

2. **Build TypeScript SDK**

```shell
cd ../../sdk/typescript
npm install
npm run build
```

3. **Run the Example**

```shell
npm install
npm run abi
npm run start
```

---

## Option 3: Run with Gradle

To perform all prerequisites and run the example in one go:

```shell
../../gradlew build
npm run start
```
