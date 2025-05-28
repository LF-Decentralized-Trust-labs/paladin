# Example: Event Listening

## Pre-requisites

Requires a local Paladin instance running on `localhost:31548`.
Requires a local Paladin instance running on `localhost:31648`.
Requires a local Paladin instance running on `localhost:31748`.

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

Build common utilities

```shell
cd ../common
npm install
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
