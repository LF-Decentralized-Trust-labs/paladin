# Notarized Tokens

In this tutorial, you’ll learn how to create and manage **Notarized Tokens (Noto)** within Paladin. Unlike simple private storage, **Notarized Tokens allow secure, private exchanges while maintaining verifiability**.

## Why Use Notarized Tokens?

- **Privacy-Preserving Transfers** – Transactions remain private, visible only to relevant parties.
- **Notary-Controlled Oversight** – A designated **notary** approves and submits every transaction, ensuring compliance and control.
- **Selective Disclosure** – Owners can prove token ownership by selectively revealing transaction details when needed.

This tutorial will guide you through issuing, transferring, and verifying tokens using Paladin’s notarization model.

---

## Prerequisites

Before starting, ensure you have:

1. Completed the [Private Smart Contract Tutorial](./private-storage.md).
2. A **running Paladin network** with at least three nodes (**Node1, Node2, and Node3**).

---

## Overview

This tutorial will cover:

1. **Deploying a Noto Token** – Creating a “cash token” with a designated notary.
2. **Minting Tokens** – Issuing new tokens into circulation.
3. **Transferring Tokens** – Simulating payments by moving tokens between nodes.

💡 **The complete example code is available in the [Paladin example repository](https://github.com/LFDT-Paladin/paladin/tree/main/examples/notarized-tokens).**

---

## Step 1: Deploy a Noto Token

First, create a **Noto Factory** instance and deploy a new token. **Node1** will act as the **notary**, responsible for approving and submitting all transactions related to this token. Additionally, **Node1 will be the initial recipient of the minted tokens**.

```typescript
logger.log("Step 1: Deploying a Noto cash token...");
const notoFactory = new NotoFactory(paladinClientNode1, "noto");
const cashToken = await notoFactory
  .newNoto(verifierNode1, {
    name: "NOTO",
    symbol: "NOTO",
    notary: verifierNode1,
    notaryMode: "basic",
  })
  .waitForDeploy();
if (!cashToken) {
  logger.error("Failed to deploy the Noto cash token!");
  return false;
}
logger.log("Noto cash token deployed successfully!");
```

### Why the Notary Role Matters
The **notary** is more than just a minting authority—it plays a fundamental role in the **Noto token model**:

- **Approves and submits all token transactions** to the network.
- **Maintains full visibility** over all token movements.
- **Ensures transaction integrity and compliance** with predefined rules.

By designating a notary, every transaction must be verified and approved, ensuring controlled and auditable token transfers.

---

## Step 2: Mint Tokens

With the token contract deployed, let’s **mint** an initial supply of tokens for Node1. This simulates creating new “cash” in the system.

```typescript
logger.log("Step 2: Minting 2000 units of cash to Node1...");
const mintReceipt = await cashToken
  .mint(verifierNode1, {
    to: verifierNode1,
    amount: 2000,
    data: "0x",
  })
  .waitForReceipt();
if (!mintReceipt) {
  logger.error("Failed to mint cash tokens!");
  return false;
}
logger.log("Successfully minted 2000 units of cash to Node1!");
```

### What Happens Here?

1. **Node1 submits a minting request** to the notary (in this case, node1 is the notary so it will be receiving and validating it's own request).
2. **The notary reviews and approves** the request.
3. **Tokens are minted and assigned** to the recipient.
4. **The `data` field is recorded** in the transaction receipt for auditability.

### Key Parameters
- **`amount`** – Number of tokens to create.
- **`to`** – Recipient of the newly minted tokens.
- **`data`** – (Optional) Can include metadata or extra information about the transaction.

💡 **The data field is stored in the transaction receipt, making it useful for audits or tracking purposes.**

---

## Step 3: Transfer Tokens to Node2

Now that Node1 has tokens, let’s **transfer some to Node2**. This works similarly to a bank transfer.

```typescript
logger.log("Step 3: Transferring 1000 units of cash from Node1 to Node2...");
const transferToNode2 = await cashToken
  .transfer(verifierNode1, {
    to: verifierNode2,
    amount: 1000,
    data: "0x",
  })
  .waitForReceipt();
if (!transferToNode2) {
  logger.error("Failed to transfer cash to Node2!");
  return false;
}
logger.log("Successfully transferred 1000 units of cash to Node2!");
```

---

## Step 4: Transfer Tokens to Node3

Now let’s see how **Node2** transfers tokens to **Node3**. Since Node2 is initiating the transaction, we call `.using(paladinClientNode2)` to ensure **Node2 signs the transaction instead of Node1**.

```typescript
logger.log("Step 4: Transferring 800 units of cash from Node2 to Node3...");
const transferToNode3 = await cashToken
  .using(paladinClientNode2)
  .transfer(verifierNode2, {
    to: verifierNode3,
    amount: 800,
    data: "0x",
  })
  .waitForReceipt();
if (!transferToNode3) {
  logger.error("Failed to transfer cash to Node3!");
  return false;
}
logger.log("Successfully transferred 800 units of cash to Node3!");
```

### Transaction Privacy in Paladin

Unlike traditional blockchains, **Paladin’s notarized token model ensures that not all participants see every transaction**:

- **The notary has full visibility** over all token transfers.
- **Node2 and Node3 only see transactions they were involved in.**
- **Other nodes have no visibility into the transfer.**


---

## Conclusion

Congratulations! You have successfully:

1. **Deployed a Noto token** to represent cash within the Paladin network.
2. **Minted tokens** under a notary’s supervision.
3. **Transferred tokens** between nodes while maintaining privacy and control.

At this point, you understand how to issue, manage, and transfer notarized tokens within Paladin.

---

## Next Steps

Now that you’ve explored **Notarized Tokens**, you’re ready to delve into **Zeto**, Paladin’s **zero-knowledge domain** for enhanced privacy. In the next tutorial, you’ll learn how to build **a privacy-preserving cash payment system** using advanced techniques such as **private minting and selective disclosure**.

[Continue to the Zero-Knowledge Proof Tutorial →](./zkp-cbdc.md)

