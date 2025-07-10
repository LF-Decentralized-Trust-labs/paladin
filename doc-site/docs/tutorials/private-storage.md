# Private Smart Contract (SimpleStorage)

In this tutorial, you'll learn how to deploy and interact with a **private storage contract** using **Paladin’s privacy groups**. Unlike the **public storage contract**, where data is visible to everyone, **private storage ensures that only authorized members** of a privacy group can interact with the contract.

If you're new to **Pente privacy groups** or want to dive deeper into their architecture, check out the [Pente documentation](https://lf-decentralized-trust-labs.github.io/paladin/head/architecture/pente) for more information.

---

## Prerequisites

Before starting, make sure you have:

1. Completed the [Public Smart Contract Tutorial](./public-storage.md) and are familiar with:
   - Deploying and interacting with smart contracts.
   - Using the Paladin SDK for blockchain transactions.
2. A **running Paladin network** with at least **three nodes** (Node1, Node2, Node3).

---

## Overview

This tutorial will guide you through:

1. **Creating a privacy group** – Define a **private transaction group** that includes selected members.
2. **Deploying a private contract** – Deploy a **Storage** contract that only privacy group members can interact with.
3. **Interacting with the contract** – Members will **store and retrieve values** securely.
4. **Testing unauthorized access** – A non-member (Node3) will attempt to retrieve data, demonstrating **privacy enforcement**.

There is a second example (`update.js`) in the privacy storage example folder which does the following:

1. **Attaches to an existing privacy group and private storage smart contract**
2. **Reads the value of the existing smart contract, then adds a new random number to it**
3. **Writes the newly update value to the smart contract**

You can use this second example to learn about using resuming use of existing privacy groups and contracts.

The **full example** code is available in the [Paladin example repository](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/privacy-storage).

---

## Step 1: Create a Privacy Group

To **restrict contract access** to specific members, you first need to create a **privacy group**.

```typescript
logger.log("Creating a privacy group for Node1 and Node2...");
const penteFactory = new PenteFactory(paladinNode1, "pente");
const memberPrivacyGroup = await penteFactory.newPrivacyGroup({
  members: [verifierNode1, verifierNode2],
  evmVersion: "shanghai",
  externalCallsEnabled: true,
}).waitForDeploy();
```

#### Key Points:

1.  The **privacy group** consists of **Node1 and Node2**.
2.  Transactions within this group will **only be visible** to these members.
3.  **Node3 is excluded**, meaning it **won’t have access** to private transactions.

---

## Step 2: Deploy the Contract in the Privacy Group

Now that the **privacy group** is established, **deploy** the `Storage` contract inside this group.

```typescript
logger.log("Deploying a smart contract to the privacy group...");
const contractAddress = await memberPrivacyGroup.deploy({
  abi: storageJson.abi,
  bytecode: storageJson.bytecode,
  from: verifierNode1.lookup,
}).waitForDeploy();
```

#### Key Points

1. The contract is deployed **inside the privacy group**, meaning **only group members** can interact with it.
2. **Transactions involving this contract are private** and only visible to **Node1 & Node2**.

---

## Step 3: Store and Retrieve Values as Group Members

### Storing a Value

Now that the contract is deployed, **Node1** can store a value.

```typescript
const privateStorageContract = new PrivateStorage(
  memberPrivacyGroup,
  contractAddress
);

const valueToStore = 125; // Example value to store
logger.log(`Storing a value "${valueToStore}" in the contract...`);
const storeReceipt = await privateStorageContract.sendTransaction({
  from: verifierNode1.lookup,
  function: "store",
  data: { num: valueToStore },
}).waitForReceipt();
logger.log(
  "Value stored successfully! Transaction hash:",
  storeReceipt?.transactionHash
);
```

---

### Retrieving the Stored Value

Group members **Node1 & Node2** can now retrieve the stored value.

```typescript
// Retrieve the value as Node1
logger.log("Node1 retrieving the value from the contract...");
const retrievedValueNode1 = await privateStorageContract.call({
  from: verifierNode1.lookup,
  function: "retrieve",
});
logger.log(
  "Node1 retrieved the value successfully:",
  retrievedValueNode1["value"]
);

// Retrieve the value as Node2
logger.log("Node2 retrieving the value from the contract...");
const retrievedValueNode2 = await privateStorageContract
  .using(paladinNode2)
  .call({
    from: verifierNode2.lookup,
    function: "retrieve",
  });
logger.log(
  "Node2 retrieved the value successfully:",
  retrievedValueNode2["value"]
);
```

---

## Step 4: Verify Privacy by Testing Unauthorized Access

Now, let’s test if **Node3 (an outsider)** can access the stored data.

### **What should happen?**

Node3 should NOT be able to retrieve the stored value because it wasn’t part of the privacy group.

```typescript
try {
  logger.log("Node3 (outsider) attempting to retrieve the value...");
  await privateStorageContract.using(paladinNode3).call({
    from: verifierNode3.lookup,
    function: "retrieve",
  });
  logger.error(
    "Node3 (outsider) should not have access to the private Storage contract!"
  );
  return false;
} catch (error) {
  logger.info(
    "Expected error - Node3 (outsider) cannot retrieve the data. Access denied."
  );
}
```

### Why Privacy Groups Work

- **Private State Isolation** – Transactions within a privacy group are only visible to its members.
- **No Global State Sharing** – Outsiders (e.g., Node3) never receive the transaction history, making it impossible for them to infer contract data.
- **Selective State Distribution** – Only group members can access and verify the shared state.

By design, **Node3 does not just “lack permission” to call the contract—it lacks any knowledge of its state, history, or data, making unauthorized access fundamentally impossible**.

## Conclusion

Congratulations! You’ve successfully:

1. Created a privacy group with selected members.
2. Deployed a `Storage` contract in the privacy group.
3. Ensured secure interactions with the contract for group members.
4. Verified that unauthorized access is blocked.

---

## Next Steps

After exploring private smart contracts and learning how to keep contract data confidential within a privacy group, you’re ready to explore other Paladin domains. In the next tutorial, you’ll learn about **Notarized Tokens (Noto)** - a way to create, mint, and transfer tokens on the Paladin network. This will introduce concepts like notaries, restricted minting, and token transfers among multiple nodes.

[Continue to the Notarized Tokens Tutorial →](./notarized-tokens.md)
