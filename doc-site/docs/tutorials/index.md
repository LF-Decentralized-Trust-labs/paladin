## Prerequisites

- **Git**  
- **Node.js 20.x or newer**  
  Follow the [official Node.js documentation](https://nodejs.org/en/download/package-manager) for your platform to install the appropriate version.

To access the tutorial code, clone the **most recent release tag** of the Paladin repository:

```shell
REPO=https://github.com/LF-Decentralized-Trust-labs/paladin.git && \
git clone $REPO --single-branch --branch \
$(git ls-remote --tags $REPO | cut -d/ -f3 | sort -V | tail -n1)
```

## Next Steps

The tutorials on this page provide an introduction to building on the Paladin platform. If you haven’t already, visit the [Getting Started guide](../getting-started/installation.md) to familiarize yourself with running Paladin before proceeding with any of the tutorials below.

<div class="grid cards" markdown>

-   **[Hello World](hello-world.md)**  
    
    ---  
    
    Begin with a simple “Hello World” example to get familiar with some of the basics.

-   **[Public Smart Contract](public-storage.md)**  

    ---  

    Explore fundamental SDK functionality for deploying and interacting with a publicly visible contract.

-   **[Private  Smart Contract](private-storage.md)**  

    ---  

    Discover how to use **Privacy Groups** and keep contract data confidential among authorized members.

-   **[Notarized Tokens](notarized-tokens.md)**  

    ---  

    Learn how to issue, mint, and transfer tokens using Paladin’s **Notarized Tokens** domain.


-   **[Wholesale CBDC](zkp-cbdc.md)**  

    ---  

    Implement a wholesale CBDC with **zero-knowledge proof** features for enhanced privacy and regulatory compliance.

-   **[Bond Issuance](bond-issuance.md)**  

    ---  

    Understand how **Notarized Tokens** and **Privacy Groups** work together to model and manage a bond issuance process.

</div>