# Example: Private Stablecoin with KYC and Nullifiers

This example demonstrates a privacy-preserving stablecoin using Zeto with KYC compliance and nullifiers for enhanced security and regulatory oversight.

See the [tutorial](https://lf-decentralized-trust-labs.github.io/paladin/head/tutorials/private-stablecoin/) for a detailed explanation.

## Pre-requisites

Run the common [setup steps](../README.md) before running the example.

## Running the Example

> **Note:** This example is not yet supported by the published Paladin SDK. You’ll need to build the SDK locally before running it.

### Step 1: Build the SDK

```bash
cd ../../sdk/typescript
npm install
npm run download-abi
npm run build
```

### Step 2: Run the Example

```bash
npm install                             # Install project dependencies
npm run copy-abi                        # Copy required ABIs
npm run start                           # Start the example
```
