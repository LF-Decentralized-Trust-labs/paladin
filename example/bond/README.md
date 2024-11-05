# Example: Bond Issuance

This example demonstrates a bond issuance scenario on Paladin. It leverages a combination of Noto tokens and
Pente private smart contracts in order to control visibility of various aspects of the bond issuance process.

## Running

Requires a local Paladin cluster running on `localhost:31548` and `localhost:31648`.

Compile [Solidity contracts](../../solidity):

```shell
cd ../../solidity
npm install
npm run compile
```

Build the [typescript sdk](../../sdk/typescript) which is a dependency:

```shell
cd ../../sdk/typescript
npm install
npm run build
```

Run example:

```shell
npm install
npm run abi
npm run start
```
