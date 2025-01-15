# Public Storage Contract

In the [previous tutorial](./hello-world.md), we deployed and interacted with a **HelloWorld** contract that emitted an event. Now, we will take it a step further by deploying a **Storage** contract that:
1. Allows you to store a value on the blockchain.
2. Lets you retrieve the stored value.

---

## Prerequisites

- You’ve completed the [HelloWorld Tutorial](./hello-world.md) and are familiar with:
  - Deploying contracts with the Paladin SDK.
  - Sending transactions and retrieving their receipts.

---

## Overview

The example code can be found in the [Paladin example repository](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/public-storage).

The storage solidity contract can be found [here](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/tutorial/Storage.sol).

The `Storage` contract provides two main functions:
1. **`store(uint256 num)`**: Stores a value in the contract.
2. **`retrieve()`**: Retrieves the last stored value.


### Step 1: Deploy the Contract

The first step is to deploy the `Storage` contract to the blockchain.

```typescript
const deploymentTxID = await paladin.sendTransaction({
  type: TransactionType.PUBLIC,      // Public deployment
  abi: storageJson.abi,              // ABI of the Storage contract
  bytecode: storageJson.bytecode,    // Compiled bytecode
  function: "",                      // No constructor arguments
  from: owner.lookup,                // Account signing the transaction
  data: {},                          // No additional data
});

const deploymentReceipt = await paladin.pollForReceipt(deploymentTxID, 10000);
if (!deploymentReceipt?.contractAddress) {
  logger.error("Deployment failed!");
  return false;
}
logger.log("Step 1: Storage contract deployed successfully!");
```

- **What happens**:
  - The `sendTransaction` function creates a deployment transaction for the `Storage` contract.
  - The `pollForReceipt` function waits for the transaction to be confirmed.
  - On success, the contract address is returned in the receipt.

---

### Step 2: Store a Value

After deploying the contract, you can store a value in the contract using its `store` function.

```typescript
const valueToStore = 125; // Example value to store
logger.log(`Step 2: Storing value "${valueToStore}" in the contract...`);

const storeTxID = await paladin.sendTransaction({
  type: TransactionType.PUBLIC,       // Public transaction
  abi: storageJson.abi,               // ABI of the Storage contract
  function: "store",                  // Name of the function to call
  from: owner.lookup,                 // Account signing the transaction
  to: deploymentReceipt.contractAddress, // Address of the deployed contract
  data: { num: valueToStore },        // Function arguments
});

const storeReceipt = await paladin.pollForReceipt(storeTxID, 10000);
if (!storeReceipt?.transactionHash) {
  logger.error("Failed to store value in the contract!");
  return false;
}
logger.log("Step 2: Value stored successfully!");
```

- **What happens**:
  - The `sendTransaction` function sends a transaction to call the `store` function with the value to store (`125` in this example).
  - The `pollForReceipt` function waits for the transaction to be confirmed.

---

### Step 3: Retrieve the Stored Value

Next, retrieve the stored value using the `retrieve` function of the contract.

```typescript
logger.log("Step 3: Retrieving the stored value...");

const retrieveResult = await paladin.call({
  type: TransactionType.PUBLIC,       // Public call
  abi: storageJson.abi,               // ABI of the Storage contract
  function: "retrieve",               // Name of the function to call
  from: owner.lookup,                 // Account making the call
  to: deploymentReceipt.contractAddress, // Address of the deployed contract
  data: {},                           // No arguments required for this function
});

const retrievedValue = retrieveResult["0"];
if (retrievedValue !== valueToStore.toString()) {
  logger.error(`Retrieved value "${retrievedValue}" does not match stored value "${valueToStore}"!`);
  return false;
}
logger.log(`Step 3: Value retrieved successfully! Retrieved value: "${retrievedValue}"`);
```

- **What happens**:
  - The `call` function retrieves the stored value by interacting with the `retrieve` function of the contract.
  - The retrieved value is validated against the previously stored value to ensure correctness.

---

## Conclusion

Congratulations! You’ve successfully:
1. Deployed the `Storage` contract,
2. Stored a value in the contract, and
3. Retrieved the stored value.

---

## Next Steps

Now that you've mastered deploying and interacting with a **public storage contract**, it's time to take things to the next level. In the next tutorial, you'll learn about **Storage with Privacy**, where you will add a privacy layer to the blockchain.the blockchain!

[Continue to the Privacy Storage Contract Tutorial →](./private-storage.md)