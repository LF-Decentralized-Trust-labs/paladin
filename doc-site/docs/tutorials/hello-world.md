# Hello World with Paladin

This tutorial walks you through deploying and interacting with a simple `HelloWorld` smart contract using the Paladin SDK. The example demonstrates how to:
1. Deploy the contract,
2. Interact with it by calling its `sayHello` function,
3. Retrieve and verify the emitted event.

---

## Running the Example

The example code can be found in the [Paladin example repository](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/public-storage).

The storage solidity contract can be found [here](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/tutorial/Storage.sol).

Follow the [Getting Started](../../getting-started/installation/) instructions to set up a Paladin environment. Then, follow the example [README](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/helloworld/README.md) to run the code.

---

### Overview

We have a `HelloWorld` smart contract, which:
- Emits a "welcome" message as an event when its `sayHello` function is called.

### Key Artifacts
To deploy and interact with the contract, we use:
1. **ABI**: Describes the contract's interface, including its functions and events.
2. **Bytecode**: The compiled contract code.

These are pre-compiled and provided in the `helloWorldJson` object.

---

### Step 1: Deploy the Contract

```typescript
const deploymentTxID = await paladin.sendTransaction({
  type: TransactionType.PUBLIC,       // Deploy publicly
  abi: helloWorldJson.abi,            // ABI of the HelloWorld contract
  bytecode: helloWorldJson.bytecode,  // Compiled bytecode
  function: "",                       // No constructor arguments
  from: owner.lookup,                 // Account signing the transaction
  data: {},                           // No additional data
});
```

- **What happens**:
  - The `sendTransaction` method sends a deployment transaction to the Paladin network.
  - The function returns a `deploymentTxID` that uniquely identifies the transaction.

### Step 2: Confirm the Deployment

```typescript
const deploymentReceipt = await paladin.pollForReceipt(deploymentTxID, 10000, true);
if (!deploymentReceipt?.contractAddress) {
  logger.error("Deployment failed!");
  return false;
}
logger.log("Contract deployed successfully at address:", deploymentReceipt.contractAddress);
```

- **What happens**:
  - We use `pollForReceipt` to wait for the deployment transaction to be confirmed.
  - If successful, the receipt includes the new `contractAddress`.

---

### Step 3: Call the `sayHello` Function

```typescript
const name = "Blocky McChainface"; // Example name for the greeting

const sayHelloTxID = await paladin.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: helloWorldJson.abi,
  function: "sayHello",
  from: owner.lookup,
  to: deploymentReceipt.contractAddress,
  data: { name: name },
});
```

- **What happens**:
  - The `sendTransaction` method sends a transaction to call the `sayHello` function of the deployed contract.
  - The `data` object includes the function arguments—in this case, the `name` of the person being greeted.

---

### Step 4: Confirm the Function Call

```typescript
const functionReceipt = await paladin.pollForReceipt(sayHelloTxID, 10000, true);
if (!functionReceipt?.transactionHash) {
  logger.error("Receipt retrieval failed!");
  return false;
}
logger.log("sayHello function executed successfully!");
```

- **What happens**:
  - Similar to the deployment step, we wait for confirmation of the `sayHello` function call using `pollForReceipt`.

---

### Step 5: Retrieve the Emitted Event

```typescript
const events = await paladin.decodeTransactionEvents(
  functionReceipt.transactionHash,
  helloWorldJson.abi,
  "pretty=true",
);

logger.log(events[0].data["message"]);
```

- **What happens**:
  - We use `decodeTransactionEvents` to extract event data from the `sayHello` transaction.

---

## Conclusion

Congratulations! You've successfully:
1. Deployed the `HelloWorld` contract,
2. Called its `sayHello` function,
3. Retrieved and validated the emitted event.

This simple example demonstrates how to interact with smart contracts using the Paladin SDK. 

---

## Next Steps

Now that you’ve deployed and interacted with the `HelloWorld` contract, you’re ready to explore more complex interactions with smart contracts. In the next tutorial, we will introduce you to a **Storage** contract where you will write and read from from the blockchain!

[Continue to the Storage Contract Tutorial →](./public-storage.md)
