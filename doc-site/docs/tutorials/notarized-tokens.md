# Notarized Tokens

In this tutorial, you’ll learn how to create and manage a basic token using the **Notarized Tokens (noto)** domain. Unlike the private storage example, these tokens can be transferred between nodes publicly, demonstrating how assets (e.g., “cash”) can be issued and tracked on the blockchain using Paladin.

## Prerequisites

Make sure you have:

1. Completed the [Private Storage Tutorial](./private-storage.md).
2. A **running Paladin network** with at least three nodes (Node1, Node2, Node3).

---

## Overview

This tutorial will guide you through:

1. **Deploying a Noto Token**: Use the `NotoFactory` to create a “cash token” with a specific notary.
2. **Minting Tokens**: Issue tokens to a particular node’s account.
3. **Transferring Tokens**: Send tokens between different nodes to simulate basic payments.

You can find the complete example code in the [Paladin example repository](https://github.com/LF-Decentralized-Trust-labs/paladin/tree/main/example/notarized-tokens).

---

## Step 1: Deploy a Noto Token

First, create a **Noto Factory** instance and deploy a new token. In this scenario, Node1 will act as both the notary (the entity allowed to mint tokens) and the initial recipient of the minted cash.

```typescript
logger.log("Step 1: Deploying a Noto cash token...");
const notoFactory = new NotoFactory(paladinClientNode1, "noto");
const cashToken = await notoFactory.newNoto(verifierNode1, {
  notary: verifierNode1,          // The notary for this token
  restrictMinting: true,          // Restrict minting to the notary only
});
if (!cashToken) {
  logger.error("Failed to deploy the Noto cash token!");
  return false;
}
logger.log("Noto cash token deployed successfully!");
```

**Key Points**:
- **`notary`**: Specifies which verifier (account) can mint new tokens.
- **`restrictMinting`**: If `true`, only the `notary` can mint additional tokens.

---

## Step 2: Mint Tokens

Now that the token contract exists, **mint** an initial supply of tokens to Node1. This step simulates creating new “cash” in the system.

```typescript
logger.log("Step 2: Minting 2000 units of cash to Node1...");
const mintReceipt = await cashToken.mint(verifierNode1, {
  to: verifierNode1,              // Mint cash to Node1
  amount: 2000,                   // Amount to mint
  data: "0x",                     // Additional data (optional)
});
if (!mintReceipt) {
  logger.error("Failed to mint cash tokens!");
  return false;
}
logger.log("Successfully minted 2000 units of cash to Node1!");
```

**Key Points**:
- **`amount`**: Number of tokens to create.
- **`data`**: Can include extra metadata or encoding, if needed.

---

## Step 3: Transfer Tokens to Node2

With tokens minted on Node1, you can **transfer** some of them to Node2. This step demonstrates a simple token transfer, much like sending money to another account.

```typescript
logger.log("Step 3: Transferring 1000 units of cash from Node1 to Node2...");
const transferToNode2 = await cashToken.transfer(verifierNode1, {
  to: verifierNode2,              // Transfer to Node2
  amount: 1000,                   // Amount to transfer
  data: "0x",                     // Optional additional data
});
if (!transferToNode2) {
  logger.error("Failed to transfer cash to Node2!");
  return false;
}
logger.log("Successfully transferred 1000 units of cash to Node2!");
```

---

## Step 4: Transfer Tokens to Node3

Now let’s see how Node2 can pass tokens to Node3. This step involves calling `.using(paladinClientNode2)` so that **Node2** signs the transaction rather than Node1.

```typescript
logger.log("Step 4: Transferring 800 units of cash from Node2 to Node3...");
const transferToNode3 = await cashToken.using(paladinClientNode2).transfer(verifierNode2, {
  to: verifierNode3,              // Transfer to Node3
  amount: 800,                    // Amount to transfer
  data: "0x",                     // Optional additional data
});
if (!transferToNode3) {
  logger.error("Failed to transfer cash to Node3!");
  return false;
}
logger.log("Successfully transferred 800 units of cash to Node3!");
```

**Key Points**:
- **`.using(paladinClientNode2)`** ensures the transaction is signed by Node2.
- If Node2 does not have sufficient tokens (e.g., tries to transfer 1200 while only having 1000), the transfer should fail and return an error.

---

## Conclusion

Congratulations! You’ve successfully:

1. **Deployed a Noto token** to represent cash within the Paladin network.  
2. **Minted tokens** from a designated notary account.  
3. **Transferred tokens** between different nodes, demonstrating how digital assets move across participants.  

At this point, you have a basic grasp of how to issue and manage tokens using the Noto domain.

---

## Next Steps

Now that you’ve explored how to create, mint, and transfer tokens using the Noto domain, you’re ready to delve into Zeto, Paladin’s zero-knowledge domain for more advanced privacy features. In the next tutorial, you’ll learn how to build a cash payment solution—for example, a wholesale CBDC or a commercial bank money rail—while leveraging powerful privacy techniques such as private minting and selective disclosure.

[Continue to the Zero-Knowledge Proof Tutorial →](./zkp-cbdc.md)

