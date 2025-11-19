# Private Stablecoin with KYC and Deposit/Withdraw

The code for this tutorial can be found in [examples/private-stablecoin](https://github.com/LFDT-Paladin/paladin/blob/main/examples/private-stablecoin).

This example demonstrates a **private stablecoin with KYC compliance** that exists as both a **public ERC20 token** and a **private Zeto token**, showcasing **deposit/withdraw functionality** using Paladin's [Zeto domain](../../architecture/zeto/). It illustrates how financial institutions can manage regulatory compliance while enabling users to seamlessly move between public and private representations of the same asset for enhanced privacy preservation.

## Running the example

Follow the [Getting Started](../../getting-started/installation/) instructions to set up a Paladin environment, and
then follow the example [README](https://github.com/LFDT-Paladin/paladin/blob/main/examples/private-stablecoin/README.md)
to run the code.

## Key Features

This tutorial demonstrates:

- **Dual Token System** – Same asset exists as both public ERC20 and private Zeto tokens
- **KYC Compliance** – Financial institution manages client registration for regulatory compliance
- **Seamless Deposit** – Convert public ERC20 tokens to private Zeto tokens
- **Private Transfers** – Zero-knowledge proof-based transfers with KYC verification and complete privacy
- **Seamless Withdraw** – Convert private Zeto tokens back to public ERC20 tokens, optionally to a different account owned by the depositor
- **Flexible Liquidity** – Users can choose between public transparency and private anonymity while maintaining compliance

## Scenario Overview

- **Financial Institution** (Node 1) – Deploys contracts, manages KYC registration, and issues stablecoins
- **Client A** (Node 2) – Enterprise client receiving and transacting with stablecoins
- **Client B** (Node 3) – Enterprise client receiving private transfers and withdrawing tokens

The financial institution manages the entire lifecycle from contract deployment through KYC registration to token issuance and oversight.

## Step-by-Step Walkthrough

### 1. Deploy Contract Infrastructure

The example begins by deploying both the public ERC20 and private Zeto contracts:

```typescript
// Deploy private stablecoin using Zeto_AnonNullifierKyc
const zetoFactory = new ZetoFactory(paladin1, "zeto");
const privateStablecoin = await zetoFactory
  .newZeto(financialInstitution, {
    tokenName: "Zeto_AnonNullifierKyc",
  })
  .waitForDeploy();

// Deploy public ERC20 stablecoin
const publicStablecoinAddress = await deployERC20(paladin1, financialInstitution);

// Connect the ERC20 to the Zeto contract for deposit/withdraw
await privateStablecoin
  .setERC20(financialInstitution, {
    erc20: publicStablecoinAddress,
  })
  .waitForReceipt();
```

The `Zeto_AnonNullifierKyc` contract provides:

- **Anonymous transfers** with zero-knowledge proofs
- **Nullifier protection** against double-spending
- **KYC compliance verification** for all participants
- **Deposit/withdraw functionality** for ERC20 integration

### 2. KYC Registration Process

Before any stablecoin operations, the financial institution registers all participants for KYC compliance:

```typescript
// Financial institution registers itself
const bankPublicKey = await getBabyjubPublicKey(financialInstitution);
let kycTxId = await paladin1.ptx.sendTransaction({
  type: TransactionType.PUBLIC,
  from: financialInstitution.lookup,
  to: privateStablecoin.address,
  data: {
    publicKey: bankPublicKey,
    data: "0x", // KYC compliance data could go here
  },
  function: "register",
  abi: kycAbi.abi,
});

// Register Client A and Client B similarly...
```

**KYC Benefits:**

- **Regulatory compliance** – All participants verified before token operations
- **Privacy-preserving verification** – Uses BabyJubJub keys compatible with zero-knowledge proofs
- **Financial institution control** – Centralized KYC management by the issuing institution
- **Cryptographic identity binding** – Links identity verification to cryptographic keys

### 3. Mint Public Stablecoins

The financial institution mints public ERC20 stablecoins to clients:

```typescript
// Mint public stablecoins to Client A
await mintERC20(paladin1, financialInstitution, clientA, publicStablecoinAddress, 100000);

// Mint public stablecoins to Client B  
await mintERC20(paladin1, financialInstitution, clientB, publicStablecoinAddress, 50000);
```

**Public Token Benefits:**

- **Transparent balances** – All ERC20 balances are publicly visible
- **Standard compatibility** – Works with existing DeFi protocols
- **Regulatory compliance** – Full transaction transparency for oversight
- **Familiar interface** – Standard ERC20 functionality

### 4. Deposit: Public to Private

Client A decides to deposit some public tokens for privacy:

```typescript
// Client A approves Zeto contract to spend their ERC20 tokens
await approveERC20(
  paladin2,
  clientA,
  privateStablecoin.address,
  publicStablecoinAddress,
  75000
);

// Client A deposits ERC20 tokens to get private Zeto tokens
const depositReceipt = await privateStablecoin
  .using(paladin2)
  .deposit(clientA, {
    amount: 75000,
  })
  .waitForReceipt();
```

**Privacy Benefits:**

- **Token amounts become private** – No longer visible on public ledger
- **Identity protection** – Client's balance and activity become anonymous
- **Zero-knowledge security** – Cryptographic proofs ensure validity with KYC compliance
- **Reversible process** – Can withdraw back to public tokens anytime

### 5. Private Transfers with KYC Verification

Client A makes a private transfer to Client B, with automatic KYC verification:

```typescript
const transferReceipt = await privateStablecoin
  .using(paladin2)
  .transfer(clientA, {
    transfers: [
      {
        to: clientB,
        amount: 25000, // Transfer 25,000 private tokens
        data: "0x",
      },
    ],
  })
  .waitForReceipt();
```

**KYC-Verified Privacy Features:**

- **Automated compliance** – Transfer only succeeds if both parties are KYC-verified
- **Zero-knowledge verification** – KYC status verified without revealing identity details
- **Private amounts** – Transfer amounts remain completely private
- **Regulatory confidence** – Financial institution maintains oversight capability

### 6. Withdraw: Private to Public

Client B withdraws some private tokens back to public ERC20:

```typescript
const withdrawReceipt = await privateStablecoin
  .using(paladin3)
  .withdraw(clientB, {
    amount: 15000, // Withdraw 15,000 tokens
  })
  .waitForReceipt();
```

**Withdrawal Benefits:**

- **Seamless conversion** – Private tokens automatically converted to public ERC20
- **Preserved compliance** – KYC verification carries through to public domain
- **Full interoperability** – Withdrawn tokens work with any ERC20-compatible system
- **Transparent balances** – Public tokens provide transparency when needed

## Privacy and Compliance Integration

### How KYC Works with Zero-Knowledge Proofs
This example demonstrates a powerful combination of privacy and compliance:

- **Registration Phase** – Financial institution registers client identities with their cryptographic keys
- **Proof Generation** – During transfers, zero-knowledge proofs verify KYC status without revealing identity
- **Compliance Assurance** – All private transactions are automatically verified against the KYC registry
- **Privacy Preservation** – Transaction amounts and participant identities remain private to external observers

## Use Cases

This private stablecoin with KYC pattern is suitable for:

- **Regulated DeFi protocols** – Privacy-preserving DeFi with built-in compliance
- **Corporate treasury management** – Enterprise payments with regulatory oversight
- **Cross-border remittances** – Private transfers with KYC compliance
- **Wholesale CBDC implementations** – Central bank digital currencies with privacy features
- **Supply chain finance** – Private B2B payments with verified participants

## Conclusion

The Private Stablecoin with KYC example demonstrates how Paladin enables:

- **Regulatory compliance** through automated KYC verification using zero-knowledge proofs
- **Transaction privacy** while maintaining compliance assurance
- **Seamless public-private interoperability** through deposit/withdraw functionality
- **Financial institution control** over customer onboarding and compliance
- **Enterprise-grade security** through nullifier-based double-spend protection

This showcases Paladin's unique capability to balance **privacy preservation** with **regulatory compliance**, enabling financial institutions to offer innovative privacy-preserving services while meeting their compliance obligations.

## Next Steps

Explore how **Notarized Tokens** and **ZKP Tokens** can be used together in an atomic swap scenario.

[Continue to the Atomic Swap Tutorial →](./atomic-swap.md)