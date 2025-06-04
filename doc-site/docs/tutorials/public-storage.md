# Public Smart Contract (SimpleStorage)

In the [previous tutorial](./hello-world.md), we deployed and interacted with a **HelloWorld** contract that emitted an event. Now, we will go a step further and deploy a **Storage** contract that:

1. **Stores values** on the blockchain
2. **Retrieves stored values** on demand

This tutorial will guide you through **deploying, storing, and retrieving data** using the Paladin SDK.

Like the previous tutorial this one demonstrates traditional, non-private Ethereum activity using the Paladin APIs to drive the Paladin transaction manager.

---

## Prerequisites

Before you begin, ensure that you:  

- Completed the [HelloWorld Tutorial](./hello-world.md), where you learned:  
   - How to deploy contracts using the Paladin SDK
   - How to send transactions and retrieve receipts
- Have access to a **Paladin network** to deploy and interact with smart contracts

---

## Overview

The `Storage` contract provides two primary functions:  

- **`store(uint256 num)`** – Stores a value in the contract
- **`retrieve()`** – Retrieves the last stored value

### Paladin API & Ethereum Similarities
Paladin’s API design follows **Ethereum JSON-RPC patterns**, making it familiar to developers who have used standard Ethereum APIs:  
- **Transactions (`sendTransaction`)** → Similar to `eth_sendTransaction`, used for modifying on-chain state
- **Calls (`call`)** → Similar to `eth_call`, used for reading blockchain state without modifying it

> 💡 **Numbers in Paladin are passed as strings** by default, consistent with JSON-RPC standards

---

## Where to Find the Code?

🔹 Example implementation: [Paladin example repository](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/public-storage)  
🔹 Solidity contract: [Storage.sol](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/tutorials/Storage.sol)  

---

## Step 1: Deploy the Contract

The first step is to deploy the `Storage` contract to the blockchain

```typescript
const deploymentTxID = await paladin.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: storageJson.abi,
  bytecode: storageJson.bytecode,
  from: owner.lookup,
  data: {},
});

// Wait for deployment confirmation
const deploymentReceipt = await paladin.pollForReceipt(deploymentTxID, 10000);
if (!deploymentReceipt?.contractAddress) {
  logger.error("Deployment failed!");
  return false;
}
logger.log(`Step 1: Storage contract deployed successfully at address: ${deploymentReceipt.contractAddress}`);
```

### What Happens Here?
1. The `sendTransaction` function **creates a contract deployment transaction** (similar to `eth_sendTransaction`)
2. The `pollForReceipt` function **waits for confirmation** that the contract has been deployed
3. If successful, the **contract address is returned in the receipt**

---

## Step 2: Store a Value

Now that the contract is deployed, you can **store a value** in it using the `store` function.

```typescript
const valueToStore = 125; // Example value to store
logger.log(`Step 2: Storing value "${valueToStore}" in the contract...`);

const storeTxID = await paladin.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: storageJson.abi,
  function: "store",
  from: owner.lookup,
  to: deploymentReceipt.contractAddress,
  data: { num: valueToStore },
});

// Wait for transaction confirmation
const storeReceipt = await paladin.pollForReceipt(storeTxID, 10000);
if (!storeReceipt?.transactionHash) {
  logger.error("Failed to store value in the contract!");
  return false;
}
logger.log(`Step 2: Value stored successfully in the contract.`);
```

### What Happens Here?
1. The `sendTransaction` function **calls the `store` function** with the value `125`
2. The `pollForReceipt` function **waits for confirmation** that the value has been stored

---

## Step 3: Retrieve the Stored Value

Now, retrieve the stored value using the `retrieve` function

```typescript
logger.log("Step 3: Retrieving the stored value...");

const retrieveResult = await paladin.call({
  type: TransactionType.PUBLIC,
  abi: storageJson.abi,
  function: "retrieve",
  from: owner.lookup,
  to: deploymentReceipt.contractAddress,
  data: {},
});

const retrievedValue = retrieveResult["value"];
if (retrievedValue !== valueToStore.toString()) {
  logger.error(`Retrieved value "${retrievedValue}" does not match stored value "${valueToStore}"!`);
  return false;
}
logger.log(`Step 3: Value retrieved successfully: "${retrievedValue}"`);
```

### What Happens Here?
1. The `call` function **reads the stored value** from the contract (similar to `eth_call`)
2. The retrieved value is **compared to the original stored value** to ensure correctness

💡 **Transactions (`sendTransaction`) vs. Calls (`call`)**  
- `sendTransaction`: **Writes** data to the blockchain (requires a transaction)
- `call`: **Reads** data from the blockchain (does not modify state)

💡 Why is the number returned as a string?
Paladin follows **JSON-RPC conventions**, where numbers are typically passed as strings to **avoid precision loss in JavaScript**

---

## Conclusion

🎉 Congratulations! You’ve successfully:

1. **Deployed** the `Storage` contract
2. **Stored** a value in the contract
3. **Retrieved** the stored value and validated its correctness

You now understand **how to deploy and interact with a smart contract using the Paladin SDK**, including **JSON-RPC number handling** and **Ethereum transaction conventions**

---

## Next Steps

Now that you've learned how to deploy a **public storage contract**, it's time to take things to the next level!  

🔒 In the next tutorial, you’ll explore **Storage with Privacy**, where you will **restrict access to stored values using privacy groups**

[Continue to the Private Smart Contract Tutorial →](./private-storage.md)