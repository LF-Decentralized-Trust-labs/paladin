# Atomic Swap

The code for this tutorial can be found in [example/swap](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/swap).

This example demonstrates an **atomic swap** scenario on Paladin, showing how to perform secure exchanges between different types of privacy-preserving tokens. It illustrates the power of combining multiple Paladin domains ([Zeto](../../architecture/zeto/), [Noto](../../architecture/noto/), and [Pente](../../architecture/pente/)) to create complex privacy-preserving operations with guaranteed atomicity on a single ledger.

## Running the example

Follow the [Getting Started](../../getting-started/installation/) instructions to set up a Paladin environment, and
then follow the example [README](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/example/swap/README.md)
to run the code.

## Overview

The atomic swap demonstrates an exchange between:

- **Cash tokens** - Implemented using [Zeto](../../architecture/zeto/) with zero-knowledge privacy
- **Asset tokens** - Implemented using [Noto](../../architecture/noto/) with private hooks via [Pente](../../architecture/pente/)

### Key Features

- **Atomic execution** - Either both transfers complete successfully, or neither does
- **Cross-domain compatibility** - Seamlessly swap between Zeto and Noto tokens
- **Privacy preservation** - All token operations maintain privacy appropriate to their domains
- **Smart contract hooks** - Asset tokens use private EVM logic for enhanced control

### Participants

- **Cash Issuer** (Node 1) - Issues Zeto cash tokens and deploys infrastructure
- **Asset Issuer** (Node 1) - Issues Noto asset tokens with private tracking
- **Investor 1** (Node 2) - Holds assets and wants to trade them for cash
- **Investor 2** (Node 3) - Holds cash and wants to acquire assets

## Explanation

Below is a walkthrough of each step in the example, with an explanation of what it does.

### Scenario Setup

#### Deploy atom factory

```typescript
const atomFactory = await newAtomFactory(paladin1, cashIssuer);
```

The [Atom Factory](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/shared/Atom.sol) is a utility contract on the base ledger. It creates atomic transaction containers that can execute multiple operations as a single unit. While it is not a part of any Paladin domain, it is useful for coordinating operations from multiple domains.

#### Deploy Zeto cash token

```typescript
const zetoFactory = new ZetoFactory(paladin1, "zeto");
const zetoCash = await zetoFactory
  .newZeto(cashIssuer, {
    tokenName: "Zeto_Anon",
  })
  .waitForDeploy();
```

This creates a **Zeto cash token** using the [Zeto_Anon](https://github.com/hyperledger-labs/zeto/tree/main?tab=readme-ov-file#zeto_anon) contract.

#### Create asset issuer privacy group

```typescript
const penteFactory = new PenteFactory(paladin1, "pente");
const issuerGroup = await penteFactory
  .newPrivacyGroup({
    members: [assetIssuer],
    evmVersion: "shanghai",
    externalCallsEnabled: true,
  })
  .waitForDeploy();
```

This creates a **single-member privacy group** for the asset issuer. Even with one member, this provides:

- **Private tracking** of Noto assets in an EVM-native representation
- **External call capabilities** to interact with base ledger contracts
- **Custom business logic** via private smart contracts

#### Deploy private asset tracker

```typescript
const tracker = await newERC20Tracker(issuerGroup, assetIssuer, {
  name: "ASSET",
  symbol: "ASSET",
});
```

The **private asset tracker** is an ERC-20 contract deployed within the privacy group. This hook logic will be invoked for every operation on the Noto token, which allows the private ERC-20 to perfectly mirror the current state and ownership of Noto balances, as well as providing a way for the asset issuer to set custom rules and policies on Noto operations.

#### Deploy Noto asset token

```typescript
const notoFactory = new NotoFactory(paladin1, "noto");
const notoAsset = await notoFactory
  .newNoto(assetIssuer, {
    notary: assetIssuer,
    notaryMode: "hooks",
    options: {
      hooks: {
        privateGroup: issuerGroup,
        publicAddress: issuerGroup.address,
        privateAddress: tracker.address,
      },
    },
  })
  .waitForDeploy();
```

This creates a **Noto token** that uses the private tracker as hooks.

### Token Issuance

#### Issue asset to investor1

```typescript
let receipt = await notoAsset
  .mint(assetIssuer, {
    to: investor1,
    amount: 1000,
    data: "0x",
  })
  .waitForReceipt();
```

The asset issuer mints asset tokens to investor1. This triggers both:
- **Noto token creation** on the base ledger
- **Private tracker updates** within the privacy group

#### Issue cash to investor2

```typescript
receipt = await zetoCash
  .mint(cashIssuer, {
    mints: [
      {
        to: investor2,
        amount: 10000,
        data: "0x",
      },
    ],
  })
  .waitForReceipt();
```

The cash issuer mints Zeto cash tokens to investor2.

### Swap Preparation

#### Lock asset for swap

```typescript
receipt = await notoAsset
  .using(paladin2)
  .lock(investor1, {
    amount: 100,
    data: "0x",
  })
  .waitForReceipt();
```

Investor1 **locks** 100 asset tokens, making them unavailable for other operations until the lock is released or the swap completes.

#### Prepare asset unlock

```typescript
receipt = await notoAsset
  .using(paladin2)
  .prepareUnlock(investor1, {
    lockId,
    from: investor1,
    recipients: [{ to: investor2, amount: 100 }],
    data: "0x",
  })
  .waitForReceipt();
```

This **prepares the asset transfer** without executing it, creating a transaction that can be delegated to the atomic swap contract.

#### Lock and prepare cash transfer

```typescript
receipt = await zetoCash
  .using(paladin3)
  .lock(investor2, {
    amount: 10,
    delegate: investor2Address,
  })
  .waitForReceipt();

const txID = await zetoCash.using(paladin3).prepareTransferLocked(investor2, {
  lockedInputs: [lockedStateId],
  delegate: investor2.lookup,
  transfers: [
    {
      to: investor1,
      amount: 10,
      data: "0x",
    },
  ],
}).id;
```

Similarly, investor2 **locks and prepares** the cash transfer. This creates the Zeto side of the swap, ready for atomic execution.

### Atomic Execution

#### Create atom

```typescript
const atom = await atomFactory.create(cashIssuer, [
  {
    contractAddress: notoAsset.address,
    callData: assetUnlockCall,
  },
  {
    contractAddress: zetoCash.address,
    callData: encodedCashTransfer,
  },
]);
```

The **Atom contract** combines both prepared transactions into a single atomic unit. This ensures that:
- **Both transfers succeed together**, or
- **Both transfers fail together**
- **No partial execution** is possible

#### Approve delegations

```typescript
// Approve asset unlock operation
receipt = await notoAsset
  .using(paladin2)
  .delegateLock(investor1, {
    lockId,
    unlock: assetUnlockParams,
    delegate: atom.address,
    data: "0x",
  })
  .waitForReceipt();

// Approve cash transfer operation
receipt = await zetoCash
  .using(paladin3)
  .delegateLock(investor2, {
    utxos: [lockedStateId],
    delegate: atom.address,
  })
  .waitForReceipt();
```

Both investors **delegate authority** to the Atom contract, allowing it to execute their prepared transactions.

#### Execute the swap

```typescript
await atom.using(paladin3).execute(investor2);
```

Finally, the **atomic swap executes**:
1. **Asset transfer**: 100 asset tokens transfer from investor1 to investor2
2. **Cash transfer**: 10 cash tokens transfer from investor2 to investor1
3. **Atomic guarantee**: Both transfers complete successfully together

## Key Concepts Demonstrated

### Atomic Transactions
The swap showcases Paladin's ability to create **atomic transactions** across different privacy domains. The Atom contract ensures that complex multi-step operations complete entirely or not at all.

### Cross-Domain Interoperability
The example demonstrates seamless integration between:
- **Zeto** for private cash transfers using zero-knowledge proofs
- **Noto** for notarized asset transfers with custom business logic
- **Pente** for private smart contract execution and state management

### Lock-and-Prepare Pattern
The **lock-and-prepare** pattern enables secure multi-party transactions:
1. **Lock** tokens to prevent double-spending
2. **Prepare** transactions without executing them
3. **Delegate** execution authority to atomic contracts
4. **Execute** all operations atomically

### Privacy Preservation
Each domain maintains its privacy characteristics:
- **Zeto transfers** remain anonymous through zero-knowledge proofs
- **Noto operations** benefit from notary oversight and private hooks
- **Pente contracts** keep business logic private within the group

## Next Steps

Explore how these atomic transaction patterns can be extended to more complex scenarios like bond issuance, where multiple parties and privacy groups coordinate sophisticated financial workflows.

[Continue to the Bond Issuance Tutorial →](./bond-issuance.md)