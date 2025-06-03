n# Example: Storage with Privacy Groups

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/private-storage/) for a detailed explanation.

## Pre-requisites

Requires a local Paladin instance running on `localhost:31548`.
Requires a local Paladin instance running on `localhost:31648`.
Requires a local Paladin instance running on `localhost:31748`.

## Option 1: Use the Latest Stable Version

1. **Download and extract contracts**

   - [Download Solidity contracts](https://github.com/LF-Decentralized-Trust-labs/paladin/releases/latest/download/abis.tar.gz)
   - Extract `abis.tar.gz` and copy the `abis` directory to `src/`. (full path should be: `paladin/example/private-storage/src/abis`)

2. **Build TypeScript SDK**

```shell
cp -rf src/abis ../../sdk/typescript/src/domains/
cd ../../sdk/typescript
npm install
npm run abi
npm run build
```

3. **Build common utilities**

```shell
cd ../common
npm install
npm run build
```

4. **Run the example**

```shell
npm install
npm run abi
npm run start
```

---

## Option 2: Build Locally

1. **Compile Solidity contracts**

```shell
cd ../../solidity
npm install
npm run compile
```

2. **Build TypeScript SDK**

```shell
cd ../../sdk/typescript
npm install
npm run abi
npm run build
```

3. **Build common utilities**

```shell
cd ../common
npm install
npm run build
```

4. **Run the example**

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
