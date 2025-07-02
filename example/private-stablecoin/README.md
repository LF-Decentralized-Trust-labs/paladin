# Example: private stablecoin with KYC and Nullifiers

This example demonstrates an enterprise-grade privacy-preserving stablecoin using [Zeto](https://github.com/hyperledger-labs/zeto) on Paladin with KYC compliance and nullifiers for enhanced security and regulatory oversight.

## Overview

The private stablecoin example showcases:

- **Privacy-preserving transactions** using zero-knowledge proofs
- **KYC compliance** through Zeto's KYC-enabled circuits
- **Nullifiers** to prevent double-spending and enhance security
- **Regulatory oversight** with appropriate authority controls
- **Enterprise-grade financial operations** suitable for institutional use

## Use Case Scenario

This example simulates a typical private stablecoin workflow:

1. **Regulatory Authority** deploys an private stablecoin with KYC capabilities
2. **Financial Institution** receives an initial allocation of stablecoins
3. **Enterprise Client** receives stablecoins for business operations
4. All transactions maintain privacy while ensuring compliance

## Key Features

### Zeto_AnonNullifierKyc Token

This example uses the `Zeto_AnonNullifierKyc` token implementation, which provides:

- **Anonymous transactions** - Transaction amounts and parties are private
- **Nullifiers** - Prevents double-spending attacks and enhances security
- **KYC compliance** - Built-in support for Know Your Customer requirements
- **Merkle tree proofs** - Efficient zero-knowledge proofs for state transitions
- **Regulatory transparency** - Authorities can verify compliance without seeing transaction details

### Privacy and Compliance Balance

The implementation strikes a balance between:
- **User privacy** through zero-knowledge proofs
- **Regulatory compliance** through KYC mechanisms
- **Transaction integrity** through nullifier-based security
- **Scalability** through efficient cryptographic circuits

### 🔐 **KYC Compliance with Privacy**
- **Regulatory Oversight**: All participants must be KYC-approved by regulatory authorities
- **Zero-Knowledge Verification**: KYC compliance is verified through cryptographic proofs without revealing sensitive information
- **Babyjubjub Cryptography**: Uses proper elliptic curve cryptography via [maci-crypto](https://github.com/privacy-scaling-explorations/maci) for secure key management
- **Nullifier Support**: Prevents double-spending while maintaining transaction privacy

### 🏛️ **Enterprise-Grade Operations**
- **Institutional Minting**: Only authorized financial institutions can mint stablecoin
- **Compliant Transfers**: All transfers require KYC verification of both sender and recipient
- **Audit Trail**: Regulatory authorities can monitor compliance without seeing transaction details
- **Scalable Privacy**: Supports high-volume enterprise transactions with constant proof sizes

### 🔧 **Technical Implementation**
- **Smart Contract Integration**: Uses the Zeto KYC registry for participant approval
- **Cryptographic Security**: Proper babyjubjub key decompression using maci-crypto library
- **Privacy-Preserving Proofs**: ZK-SNARK circuits verify compliance without revealing amounts or parties
- **Enterprise Workflow**: Demonstrates real-world financial institution operations

## Pre-requisites

Run the common [setup steps](../README.md) before running the example.

## Running the example

### ABI resources for Zeto

The sample uses the following ABIs to accomplish the end-to-end flow:

- `IZetoFungible.json`: Private transaction ABI for the Zeto domain, to conduct private transactions like `mint`, `transfer`, etc.
- `Zeto_AnonNullifierKyc.json`: Public transaction ABI for the Zeto KYC token implementation
- `SampleERC20.json`: Public transaction ABI for ERC20 tokens (if needed for deposit/withdraw operations)

These dependency resources can be obtained by one of the following ways.

### Option 1 - `npm` with downloaded solidity contracts

- **Extract contracts**

  - Extract the `abis.tar.gz` file that was downloaded in the [setup steps](../README.md) and copy the `abis` directory to `src/`. (full path should be: `paladin/example/private-stablecoin/src/abis`)

- **Run the example**

  ```shell
  cd <paladin-root>/example/private-stablecoin
  npm install
  npm run start
  ```

---

### Option 2 - `npm` with locally built solidity contracts and ABIs

- **Run the example**

  ```shell
  cd <paladin-root>/example/private-stablecoin
  npm install
  npm run abi
  npm run start
  ```

---

### Option 3 - Run with Gradle

- **Run the gradle build task, then run the sample:**

  ```shell
  cd <paladin-root>/example/private-stablecoin
  ../../gradlew build
  npm run start
  ```

### Verify Dependent Zeto Resources

You can verify that the dependent ABIs have been successfully put in place by the build:

```shell
$ ls <paladin-root>/sdk/typescript/build/domains/abis/
INoto.json		INotoPrivate.json	IZetoFungible.json	PentePrivacyGroup.json	Zeto_AnonNullifierKyc.json
$ ls <paladin-root>/example/private-stablecoin/src/abis/
SampleERC20.json
```

## Run the example

Run the example with the following command from inside the `example/private-stablecoin` folder:

```shell
cd <paladin-root>/example/private-stablecoin
npm run start

=== private stablecoin with KYC and Nullifiers ===
This example demonstrates a privacy-preserving private stablecoin
using Zeto with nullifiers and KYC compliance features.

1. Deploying private stablecoin with KYC capabilities...
   ✓ private stablecoin deployed at: 0x1234567890abcdef...

2. Regulatory authority issuing stablecoin to financial institution...
   ✓ Financial institution balance: 1000000 units (1 states)
   ✓ Overflow protection: false

3. Financial institution transferring stablecoin to enterprise client...
   ✓ Financial institution balance: 950000 units (1 states)
   ✓ Enterprise client balance: 50000 units (1 states)
   ✓ Transfer completed with privacy and KYC compliance

=== private stablecoin Example Complete ===
✓ Successfully demonstrated:
  - Privacy-preserving stablecoin issuance
  - KYC-compliant transfers with nullifiers
  - Enterprise-grade financial operations
  - Regulatory oversight capabilities

🎉 private stablecoin example completed successfully!
```

## Technical Details

### Zero-Knowledge Circuits

The `Zeto_AnonNullifierKyc` implementation uses the following ZK circuits:

- **Transfer circuit**: `anon_nullifier_kyc_transfer` - Handles private transfers with KYC
- **Deposit circuit**: `deposit` - Handles deposits from ERC20 tokens
- **Withdraw circuit**: `withdraw_nullifier` - Handles withdrawals with nullifier protection
- **Lock circuit**: `anon_nullifier_kyc_transferLocked` - Handles locked transfers

### Security Features

- **Nullifiers**: Prevent double-spending by creating unique nullifiers for each spent coin
- **Merkle trees**: Efficient membership proofs for UTXO sets and KYC identities
- **Zero-knowledge proofs**: Hide transaction amounts and participant identities
- **KYC integration**: Compliance checks without revealing sensitive information

### Enterprise Benefits

- **Regulatory compliance**: Built-in KYC support for regulatory requirements
- **Privacy protection**: Transaction details remain confidential
- **Audit trail**: Nullifiers provide verifiable transaction history
- **Scalability**: Efficient ZK circuits for high-throughput operations
- **Interoperability**: Compatible with existing ERC20 infrastructure

## Further Reading

- [Zeto Documentation](https://github.com/hyperledger-labs/zeto)
- [Paladin Privacy Architecture](https://lf-decentralized-trust-labs.github.io/paladin/head/architecture/)
- [Zero-Knowledge Proof Tutorials](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/zkp-cbdc/)

## Dependencies

### Core Dependencies
- **@lfdecentralizedtrust-labs/paladin-sdk**: Paladin blockchain SDK for private transactions
- **paladin-example-common**: Shared utilities for Paladin examples

### Cryptographic Dependencies
- **circomlib**: Provides iden3-compatible babyjubjub elliptic curve cryptography
  - Used for decompressing babyjubjub public keys in the same format as Zeto's iden3 implementation
  - Ensures cryptographically correct key formatting for zero-knowledge proofs
  - Compatible with the compressed key format returned by Paladin's Zeto domain
- **ffjavascript**: JavaScript finite field arithmetic library required by circomlib

## Usage

### Running the Example
```bash
# Build the example
npm run build

# Run the example
npm start
```

### Expected Workflow
1. **Deployment**: Regulatory authority deploys the KYC-compliant stablecoin
2. **KYC Registration**: All participants register their babyjubjub public keys
3. **Minting**: Financial institution mints stablecoin tokens
4. **Private Transfers**: KYC-verified transfers with nullifiers for privacy
5. **Balance Verification**: Check final balances while maintaining privacy

## Architecture

### Participants
- **Regulatory Authority**: Manages KYC approvals and contract deployment
- **Financial Institution**: Authorized stablecoin issuer and liquidity provider
- **Enterprise Clients**: KYC-verified businesses conducting private transactions

### Privacy Technology
- **Zeto Protocol**: Privacy-preserving token protocol with ZK-SNARK proofs
- **Nullifiers**: Prevent double-spending without revealing transaction history
- **KYC Registry**: On-chain compliance verification without exposing identity details
- **Babyjubjub Curves**: Efficient elliptic curve cryptography optimized for zero-knowledge proofs

### Compliance Features
- **Regulatory Transparency**: Authorities can verify compliance without seeing transaction details
- **Audit Capabilities**: Transaction validity and compliance can be verified independently
- **Identity Privacy**: Participant identities are protected while maintaining regulatory oversight
- **Selective Disclosure**: Compliance information can be revealed when required by regulation

## Integration

This example integrates with:
- **Paladin Operator**: Kubernetes deployment and management
- **Kind Clusters**: Local development and testing environment
- **Gradle Build System**: Enterprise build and dependency management
- **ZK Circuit Infrastructure**: Automated proof generation and verification

## Security Considerations

- **Key Management**: Babyjubjub private keys must be securely managed and never shared
- **Proof Verification**: All ZK-SNARK proofs are verified on-chain for security
- **Regulatory Compliance**: KYC requirements are enforced cryptographically
- **Privacy Guarantees**: Transaction amounts and participants remain private to unauthorized parties

---

*This example demonstrates the future of compliant digital finance: maintaining regulatory oversight while preserving individual privacy through advanced cryptographic techniques.* 