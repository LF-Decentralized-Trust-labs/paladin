# Tutorials

Welcome to the Paladin tutorials! These tutorials provide a comprehensive introduction to running Paladin, starting with basic concepts and progressing to advanced privacy-preserving  applications.

## Prerequisites

- **Git**  
- **Node.js 20.x or newer**  
  Follow the [official Node.js documentation](https://nodejs.org/en/download/package-manager) for your platform to install the appropriate version.

⚠️ To ensure you are using a stable version, **always clone the most recent release tag**:

```shell
REPO=https://github.com/LFDT-Paladin/paladin.git
TAG=$(git ls-remote --tags $REPO | cut -d/ -f3 | grep -v '\-rc' | sort -V | tail -n1)
git clone $REPO -b $TAG
```

## Next Steps

The tutorials on this page provide an introduction to building on the Paladin platform. If you haven't already, visit the [Getting Started guide](../getting-started/installation.md) to familiarize yourself with running Paladin before proceeding with any of the tutorials below.

<div class="grid cards" markdown>

-   **[Hello World](hello-world.md)**  
    
    ---  
    
    Begin with a simple "Hello World" example to get familiar with deploying and interacting with smart contracts using the Paladin SDK.

-   **[Public Smart Contract](public-storage.md)**  

    ---  

    Explore fundamental SDK functionality for deploying and interacting with a publicly visible contract that stores and retrieves data.

-   **[Private Smart Contract](private-storage.md)**  

    ---  

    Discover how to use **Privacy Groups** and keep contract data confidential among authorized members using Paladin's Pente domain.

-   **[Notarized Tokens](notarized-tokens.md)**  

    ---  

    Learn how to issue, mint, and transfer tokens using Paladin's **Notarized Tokens** domain with notary-controlled oversight.

-   **[Wholesale CBDC](zkp-cbdc.md)**  

    ---  

    Implement a wholesale CBDC with **zero-knowledge proof** features for enhanced privacy and regulatory compliance using the Zeto domain.

-   **[Private Stablecoin with KYC](private-stablecoin.md)**  

    ---  

    Deploy a **private stablecoin with KYC compliance** combining **deposit/withdraw functionality** with zero-knowledge proof privacy and nullifier protection.

-   **[Atomic Swap](atomic-swap.md)**  

    ---  

    Learn how to perform **atomic swaps** between different types of privacy-preserving tokens across multiple Paladin domains.

-   **[Bond Issuance](bond-issuance.md)**  

    ---  

    Understand how **Notarized Tokens** and **Privacy Groups** work together to model and manage a sophisticated bond issuance process.

</div>

## Learning Path

The tutorials are designed to be completed in sequence, with each building upon the concepts introduced in previous tutorials:

1. **Foundation** - Start with Hello World and Public Storage to understand basic Paladin operations
2. **Privacy** - Move to Private Storage to learn about privacy groups and confidential contracts
3. **Tokens** - Explore Notarized Tokens for controlled, auditable token operations
4. **Advanced Privacy** - Dive into ZKP-based privacy with the CBDC and Stablecoin tutorials
5. **Integration** - Learn atomic operations and complex workflows with Atomic Swap and Bond Issuance

## Getting Help

If you encounter issues while working through the tutorials:
- Check the [Troubleshooting guide](../../getting-started/troubleshooting.md).
- Review the [Architecture documentation](../../architecture/).
- Explore the [Examples repository](https://github.com/LFDT-Paladin/paladin/tree/main/examples).
- Join the [Paladin community on Discord](https://discord.com/channels/905194001349627914/1303371167020879903) for support and updates.