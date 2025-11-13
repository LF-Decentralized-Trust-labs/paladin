# Hello World with Paladin

This tutorial walks you through deploying and interacting with a simple `HelloWorld` smart contract using the Paladin SDK. The example demonstrates how to deploy a contract, interact with it by calling its `sayHello` function, and retrieve and verify the emitted event.

The code for this tutorial can be found in [examples/helloworld](https://github.com/LFDT-Paladin/paladin/blob/main/examples/helloworld).

## Prerequisites

Before starting, ensure you have:

1. **Git** and **Node.js 20.x or newer** installed
2. A **running Paladin network** to deploy and interact with smart contracts

## Running the Example

Follow the [Getting Started](../../getting-started/installation/) instructions to set up a Paladin environment.

> ⚠️ To ensure you are using a stable version, **always clone the most recent release tag** instead of `main`.

Clone the repository at the latest release:

```bash
REPO=https://github.com/LFDT-Paladin/paladin.git
TAG=$(git ls-remote --tags $REPO | cut -d/ -f3 | grep -v '\-rc' | sort -V | tail -n1)
git clone $REPO -b $TAG
```

Once cloned, navigate to the example:

```bash
cd paladin/examples/helloworld
```

The HelloWorld solidity contract is located at: [`solidity/contracts/tutorials/HelloWorld.sol`](https://github.com/LFDT-Paladin/paladin/blob/main/solidity/contracts/tutorials/HelloWorld.sol)

Follow the example [README](https://github.com/LFDT-Paladin/paladin/blob/main/examples/helloworld/README.md) to run the code.

## Overview

This tutorial demonstrates how traditional, non-private Ethereum smart contract interactions can be achieved via a Paladin node and its transaction manager. Later tutorials will demonstrate how to make use of more advanced features of the Paladin APIs.

We have a `HelloWorld` smart contract, which:

- Emits a "welcome" message as an event when its `sayHello` function is called.

### Key Artifacts

To deploy and interact with the contract, we use:

1. **ABI**: Describes the contract's interface, including its functions and events.
2. **Bytecode**: The compiled contract code.

These are pre-compiled and provided in the `helloWorldJson` object.

## Step-by-Step Walkthrough

### Step 1: Deploy the Contract

```typescript
const deploymentTxID = await paladin.ptx.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: helloWorldJson.abi,
  bytecode: helloWorldJson.bytecode,
  from: owner.lookup,
  data: {},
});
```

#### Key Differences (vs. calling a contract function)

- **Deployment requires `bytecode`**, as it is creating a new contract on the blockchain.
- **No `to` address is specified**, since a contract does not yet exist at this stage.
- **No specific function is called**, since this is an initial deployment.

#### What happens:

- The `sendTransaction` method sends a deployment transaction to the blockchain via Paladin.
- The function returns a `deploymentTxID` that uniquely identifies the transaction.

### Step 2: Confirm the Deployment

```typescript
const deploymentReceipt = await paladin.pollForReceipt(
  deploymentTxID,
  10000,
  true
);
if (!deploymentReceipt?.contractAddress) {
  logger.error("Deployment failed!");
  return false;
}
logger.log(
  "Contract deployed successfully at address:",
  deploymentReceipt.contractAddress
);
```

#### What happens:

- We use `pollForReceipt` to wait for the deployment transaction to be confirmed.
- If successful, the receipt includes the new `contractAddress`, which we will use in the next step.

### Step 3: Call the `sayHello` Function

```typescript
const name = "Paladin User"; // Example name for the greeting

const sayHelloTxID = await paladin.ptx.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: helloWorldJson.abi,
  function: "sayHello",
  from: owner.lookup,
  to: deploymentReceipt.contractAddress,
  data: { name: name },
});
```

#### Key Differences (vs. contract deployment)

- **Function calls require a `to` address**, since the contract already exists.
- **No `bytecode` is needed**, as we are invoking an existing contract, not creating one.
- **A specific function (`sayHello`) is provided**, along with its arguments in `data`.

#### What happens:

- The `sendTransaction` method sends a transaction to call the `sayHello` function of the deployed contract.
- The `data` object includes the function arguments—in this case, the `name` of the person being greeted.

### Step 4: Confirm the Function Call

```typescript
const functionReceipt = await paladin.pollForReceipt(sayHelloTxID, 10000, true);
if (!functionReceipt?.transactionHash) {
  logger.error("Receipt retrieval failed!");
  return false;
}
logger.log("sayHello function executed successfully!");
```

#### What happens:

- Similar to the deployment step, we wait for confirmation of the `sayHello` function call using `pollForReceipt`.

### Step 5: Retrieve the Emitted Event

```typescript
const events = await paladin.bidx.decodeTransactionEvents(
  functionReceipt.transactionHash,
  helloWorldJson.abi,
  "pretty=true"
);

logger.log(events[0].data["message"]);
```

#### What happens:

- We use `decodeTransactionEvents` to extract event data from the `sayHello` transaction.

## Key Concepts Demonstrated

This tutorial introduces fundamental Paladin concepts:

- **Contract Deployment** - Creating new smart contracts on the Paladin network
- **Function Invocation** - Calling existing contract functions
- **Transaction Management** - Using Paladin's transaction APIs
- **Event Handling** - Retrieving and processing contract events
- **Receipt Processing** - Confirming transaction success and extracting results

## Conclusion

Congratulations! You've successfully:

1. **Deployed** the `HelloWorld` contract
2. **Called** its `sayHello` function
3. **Retrieved** and validated the emitted event

This simple example demonstrates how to interact with smart contracts using the Paladin SDK.

## Next Steps

Now that you've deployed and interacted with the `HelloWorld` contract, you're ready to explore more complex interactions with smart contracts. In the next tutorial, we will introduce you to a **Storage** contract where you will write and read from the blockchain!

[Continue to the Storage Contract Tutorial →](./public-storage.md)
