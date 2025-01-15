# Private Storage Contract

In this tutorial, you'll learn how to deploy and interact with a **private storage contract** using Paladin's privacy groups. Unlike the public storage example, here only authorized members of a privacy group can interact with the contract, ensuring secure and private data handling.

---

## Prerequisites

Before starting, make sure you have:

1. Completed the [Public Storage Tutorial](./public-storage.md) and are familiar with:
   - Deploying and interacting with contracts.
   - Using Paladin SDK for blockchain transactions.
2. A running Paladin network with multiple nodes (at least 3 for this tutorial).

---

## Overview

The `PrivateStorage` tutorial demonstrates how to:

1. Create a **privacy group** with selected members.
2. Deploy a **private Storage contract** within the group.
3. Interact with the contract securely within the group.
4. Test privacy by attempting access from a non-member node.

The example code can be found in the [Paladin example repository](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/private-storage).

The Solidity contract remains the same as in the [Public Storage Tutorial](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/tutorial/Storage.sol). However, the interaction is scoped to the privacy group.

---

## Step 1: Create a Privacy Group

To enable private interactions, start by creating a privacy group with selected members.

```typescript
// Create a privacy group with Node1 and Node2
logger.log("Creating a privacy group for Node1 and Node2...");
const penteFactory = new PenteFactory(paladinNode1, "pente");
const memberPrivacyGroup = await penteFactory.newPrivacyGroup(verifierNode1, {
  group: {
    salt: newGroupSalt(), // Generate a unique salt for the group
    members: [verifierNode1, verifierNode2], // Include Node1 and Node2 as members
  },
  evmVersion: "shanghai",
  endorsementType: "group_scoped_identities",
  externalCallsEnabled: true,
});

if (!checkDeploy(memberPrivacyGroup)) {
  logger.error("Failed to create the privacy group.");
  return false;
}
logger.log("Privacy group created successfully!");
```

---

## Step 2: Deploy the Contract in the Privacy Group

Deploy the `Storage` contract within the created privacy group.

```typescript
logger.log("Deploying a private Storage contract...");
const contractAddress = await memberPrivacyGroup.deploy(
  storageJson.abi, // ABI of the Storage contract
  storageJson.bytecode, // Bytecode of the Storage contract
  verifierNode1 // Deploying as Node1
);

if (!contractAddress) {
  logger.error("Failed to deploy the private Storage contract.");
  return false;
}
logger.log(`Private Storage contract deployed! Address: ${contractAddress}`);
```

---

## Step 3: Store and Retrieve Values as Group Members

### Store a Value

Group members can store values securely in the private contract.

```typescript
const privateStorage = new PrivateStorage(memberPrivacyGroup, contractAddress);

logger.log("Storing value (125) in the private Storage contract...");
const storeTx = await privateStorage.invoke(verifierNode1, "store", { num: 125 });
logger.log("Value stored successfully! Transaction hash:", storeTx?.transactionHash);
```

---

### Retrieve the Value as a Member

Authorized group members can retrieve the stored value.

```typescript
logger.log("Node1 retrieving the stored value...");
const retrievedValueNode1 = await privateStorage.call(verifierNode1, "retrieve", []);
logger.log("Node1 retrieved the value successfully:", retrievedValueNode1["0"]);

logger.log("Node2 retrieving the stored value...");
const retrievedValueNode2 = await privateStorage
  .using(paladinNode2)
  .call(verifierNode2, "retrieve", []);
logger.log("Node2 retrieved the value successfully:", retrievedValueNode2["0"]);
```

---

## Step 4: Verify Privacy by Testing Unauthorized Access

When an outsider (Node3) tries to access the private contract, the attempt should fail.

```typescript
try {
  logger.log("Node3 (outsider) attempting to retrieve the value...");
  await privateStorage.using(paladinNode3).call(verifierNode3, "retrieve", []);
  logger.error("Node3 (outsider) should not have access to the private Storage contract!");
  return false;
} catch (error) {
  logger.info("Node3 (outsider) cannot retrieve data. Access denied.");
}
```

---

## Conclusion

Congratulations! You’ve successfully:

1. Created a privacy group with selected members.
2. Deployed a `Storage` contract in the privacy group.
3. Ensured secure interactions with the contract for group members.
4. Verified that unauthorized access is blocked.

---

## Next Steps

After exploring Private Storage and learning how to keep contract data confidential within a privacy group, you’re ready to explore other Paladin domains. In the next tutorial, you’ll learn about **Notarized Tokens (Noto)** - a way to create, mint, and transfer tokens on the Paladin network. This will introduce concepts like notaries, restricted minting, and token transfers among multiple nodes.

[Continue to the Notarized Tokens Tutorial →](./notarized-tokens.md)
