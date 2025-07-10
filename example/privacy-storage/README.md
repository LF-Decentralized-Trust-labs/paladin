# Example: Storage with Privacy Groups

This example demonstrates private smart contract operations using Pente privacy groups. Each operation is validated on the base ledger, but the private EVM details are known only to members of the privacy group.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/private-storage/) for a detailed explanation.

## Pre-requisites

Run the common [setup steps](../README.md) before running the example.

## Running the example

### Option 1 - `npm` with downloaded solidity contracts

- **Extract contracts**

  - Extract the `abis.tar.gz` file that was downloaded in the [setup steps](../README.md) and copy the `abis` directory to `src/`. (full path should be: `paladin/example/privacy-storage/src/abis`)

- **Run the example**

  ```shell
  cd <paladin-root>/example/privacy-storage
  npm install
  npm run start
  ```

---

### Option 2 - `npm` with locally built solidity contracts and ABIs

- **Run the example**

  ```shell
  cd <paladin-root>/example/privacy-storage
  npm install
  npm run abi
  npm run start
  ```

5. **(Optional) Run the update example**

You can update the contract deployed in the previous step using the `update.js` sample. Take the output from the `npm run start` step above and pass the privacy group address, group ID, and storage contract address to the `update` script. E.g. if the output from the first script looks like this:

```shell
Creating a privacy group for Node1 and Node2...
Success! address: 0xb9473d084800cdf8c4526a117fa153ad2261d94e
Privacy group created, ID: 0x09be3a466f0c935f42e9aa0451574b639497f2c4d38dee41fdabf9fc2269af08
Deploying a smart contract to the privacy group...
Contract deployed successfully! Address: 0xe5ccd796c1d3e2cf9230fd51f4333208dabfdf80
```

then pass the following parameters in to the update script:

```shell
npm run update 0xb9473d084800cdf8c4526a117fa153ad2261d94e 0x09be3a466f0c935f42e9aa0451574b639497f2c4d38dee41fdabf9fc2269af08 0xe5ccd796c1d3e2cf9230fd51f4333208dabfdf80
```

The script reads the current value and sets it to a new one.

---

### Option 3 - Run with Gradle

- **Run the gradle build task, then run the sample:**

  ```shell
  cd <paladin-root>/example/privacy-storage
  ../../gradlew build
  npm run start
  ```

**(Optional) Run the update example**

You can update the contract deployed in the previous step using the `update.js` sample. Take the output from the `npm run start` step above and pass the privacy group address, group ID, and storage contract address to the `update` script. E.g. if the output from the first script looks like this:

```shell
Creating a privacy group for Node1 and Node2...
Success! address: 0xb9473d084800cdf8c4526a117fa153ad2261d94e
Privacy group created, ID: 0x09be3a466f0c935f42e9aa0451574b639497f2c4d38dee41fdabf9fc2269af08
Deploying a smart contract to the privacy group...
Contract deployed successfully! Address: 0xe5ccd796c1d3e2cf9230fd51f4333208dabfdf80
```

then pass the following parameters in to the update script:

```shell
npm run update 0xb9473d084800cdf8c4526a117fa153ad2261d94e 0x09be3a466f0c935f42e9aa0451574b639497f2c4d38dee41fdabf9fc2269af08 0xe5ccd796c1d3e2cf9230fd51f4333208dabfdf80
```

The script reads the current value and sets it to a new one.

