# Hello World

The code for this tutorial can be found in [example/helloworld](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/helloworld).


## Running the example

Follow the [Getting Started](../../getting-started/installation/) instructions to set up a Paladin environment, and
then follow the example [README](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/helloworld/README.md) to run the code.

## Explanation

Below is a walkthrough of each step in the example, with an explanation of what it does.

## Overview

We have a `HelloWorld` contract compiled to obtain:
- **ABI** (in `helloWorldJson.abi`)
- **Bytecode** (in `helloWorldJson.bytecode`)

We will use Paladin to:
1. **Send a transaction** to deploy the contract,
2. **Confirm** that it was deployed successfully,
3. **Retrieve** the transaction details from the ledger.

### 1. Send the Deployment Transaction

```typescript
let txID = await paladin.sendTransaction({
    type: TransactionType.PUBLIC,      // Deploy publicly
    abi: helloWorldJson.abi,           // ABI of our HelloWorld contract
    bytecode: helloWorldJson.bytecode, // Compiled bytecode
    function: "",                      // No constructor args in this example
    from: "owner",                     // Account that signs and endorses the transaction
    data: {},                          // Additional data if needed (none here)
});
```

- **What happens**: Paladin encodes the contract ABI, bytecode, and any constructor parameters into a public transaction. It returns a `txID` which we’ll use to track the transaction status.

### 2. Confirm the Deployment

```typescript
let receipt = await paladin.pollForReceipt(txID, 10000);
if (!receipt?.contractAddress) {
  logger.error("Deployment failed!");
  return false;
}
logger.log(`Contract deployed successfully! Address: ${receipt.contractAddress}`);
```

- **What happens**: We poll for a receipt for up to 10 seconds. If the transaction is confirmed on the base ledger, it will include a new `contractAddress`. This confirms the contract is deployed.

### 3. Retrieve and Inspect the Transaction

```typescript
let result = paladin.getTransaction(txID, true);
logger.log("Result of getTransaction:", result);
```

- **What happens**: We fetch the full transaction details using the transaction ID. The second argument (`true`) tells Paladin to include any additional debugging or internal info, if available.

## Conclusion

That’s it! You’ve successfully:
- Deployed the `HelloWorld` contract
- Confirmed its deployment via receipt
- Inspected the final transaction record
 
## What's next
