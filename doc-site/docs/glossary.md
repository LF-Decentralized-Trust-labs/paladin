### Atomic Programmability

**Atomic Programmability** is a core Paladin feature that allows developers to define and execute complex, multi-step workflows as a single, indivisible transaction. This is true even when those steps span multiple, independent systems or ledgers—a capability known as **atomic interoperability**.

This guarantees the entire workflow is treated as one "atomic" unit: either all operations succeed together, or if any single part fails, the entire transaction is rolled back. For a developer, this all-or-nothing principle is crucial for building reliable applications that involve multiple assets or services, ensuring data remains consistent across all environments without complex manual reconciliation.

***

### Besu

**LFDT Besu** is an open-source **[Ethereum client](#evm-ethereum-virtual-machine)** developed under the Linux Foundation's Decentralized Trust and written in Java. An Ethereum client is the software that implements the Ethereum protocol, allowing a computer to act as a node on the network. Besu is notable for its versatility, as it's designed to run on both the public Ethereum Mainnet and private, permissioned enterprise networks, making it a popular choice for projects that require customized privacy and permissioning features.

***

### Circom

**Circom** is a specialized programming language used to create the **aritharithmetic circuits** required for **[Zero-Knowledge Proofs (ZKPs)](#zkp-zero-knowledge-proof)**. For developers using a ZKP-based system like Paladin's [zeto](#zeto) domain, Circom is the tool used to translate the logic of a confidential transaction (e.g., "I am transferring an asset I own") into a mathematical format that can be proven and verified without revealing the underlying secret data.

***

### Cloud-Native

Paladin is designed to be **cloud-native**, meaning it is architected to take full advantage of modern cloud computing environments. For developers and operators, this translates to an architecture built on containerization (e.g., Docker) and orchestration (e.g., Kubernetes), providing high scalability, resilience, and deployment flexibility.

***

### Delivery versus Payment (DvP)

**Delivery versus Payment (DvP)** is a financial settlement method ensuring that the transfer of an asset occurs only if and when the corresponding payment is made. It's a "trade-for-trade" settlement that eliminates the risk that one party could receive a payment without delivering the asset, or vice-versa. In Paladin, this concept is programmatically enforced through **[Atomic Programmability](#atomic-programmability)**, allowing for the trustless exchange of different digital assets in a single, all-or-nothing transaction.

***

### Domain

A **domain** in Paladin represents a logical boundary for a set of services, policies, and data. It acts as an isolated execution environment tailored for a specific purpose, such as managing a particular type of token or enforcing a unique set of rules. For a developer, it's like having a separate, secured workspace within a larger office; a domain provides isolated resources and its own set of rules, ensuring that operations within that domain don't interfere with others and adhere to a specific governance model.

***

### EVM (Ethereum Virtual Machine)

The **EVM** is the runtime environment—or "world computer"—for [smart contracts](#smart-contract) on the Ethereum blockchain. It executes the code of decentralized applications and updates the state of the network. **EVM-compatibility** is a critical feature because it allows a platform like [Paladin](#paladin) to run smart contracts written in established languages like Solidity and Vyper. For developers, this significantly lowers the barrier to entry, enabling them to use familiar tools and easily port existing applications to a new, privacy-preserving environment.

***

### Full-Stack

Paladin is a **full-stack** platform, meaning it provides developers with all the necessary components to build and deploy decentralized applications out of the box. This includes the underlying ledger interaction, confidential execution environments ([domains](#domain)), interoperability protocols, and developer tools, reducing the need to integrate multiple, disparate systems.

***

### Identity

Paladin distinguishes between two fundamental types of identity. First, the **account signing identity**, which is a long-lived identity (like a private key) used for signing transactions and proving ownership of assets. Second, the **runtime routing identity**, which is specific to the infrastructure and used for securely routing data to a specific [Paladin](#paladin) runtime instance.

For a developer, this separation is key for security and operations. Think of the **account identity** as someone's permanent home address, while the **routing identity** is the specific courier and tracking number used for a single, secure delivery.

***

### Key Management and Signing Architecture

Paladin's **Key Management and Signing Architecture** provides high security and operational flexibility by abstracting all cryptographic operations away from the core application logic.

This is achieved through **Signing Modules**, which act as a standardized interface for requesting digital signatures. Instead of handling private keys directly, an application service sends a request to the appropriate Signing Module. The module then retrieves the key from its configured secure backend—such as a cloud-based Key Management System (KMS) or a Hardware Security Module (HSM)—performs the signing operation, and returns the signature. For a developer, this architecture means they can build applications without ever exposing or managing sensitive key material, greatly reducing the attack surface.

***

### Ledger Layers

Paladin's architecture uses a layered approach to separate private computation from public settlement. While it relies on a standard **base ledger** (like an [EVM-compatible](#evm-ethereum-virtual-machine) blockchain) to provide final consensus and an immutable record of transactions, Paladin's unique value comes from its **private execution layers**. These layers, implemented as **[Domains](#domain)**, handle the confidential aspects of a transaction off-chain. Only a cryptographic commitment or proof is sent to the base ledger for validation. This design allows developers to leverage the security of a public blockchain while executing complex, private logic in a scalable and confidential manner.

***

### noto

The **noto** domain is a reference domain provided with Paladin for managing **confidential [UTXO](#utxo-unspent-transaction-output) tokens under the control of a [Notary](#notary)**. It enables the creation and transfer of tokens where ownership and value are kept private from the public, while a trusted notary service enforces the rules of the token. For a developer, `noto` provides a straightforward way to implement asset privacy where governance by a designated arbiter is acceptable or desired.

***

### Notary

In the context of Paladin, a **Notary** is a trusted entity or automated service responsible for validating transactions and enforcing rules within a specific privacy [domain](#domain) (like `noto`). The Notary acts as an arbiter for confidential transactions. Critically, it does not need to see the underlying private data (e.g., who is transacting or how much). Instead, it validates cryptographic commitments to that data, ensuring that all predefined rules are followed before approving a transaction.

***

### Paladin

A decentralized trust platform designed for building and deploying secure, interoperable, and scalable applications. It provides a full-stack environment with advanced cryptographic features, [atomic programmability](#atomic-programmability), and a [cloud-native](#cloud-native) architecture, enabling developers to build applications that require privacy and complex workflows.

***

### pente

The **pente** domain is a reference domain that provides a powerful environment for executing **private [EVM-compatible](#evm-ethereum-virtual-machine) [smart contracts](#smart-contract)**. It creates what is effectively a "mini-blockchain within a single smart contract" on the base ledger.

When a transaction is sent to a `pente` domain, it loads an ephemeral instance of an [EVM](#evm-ethereum-virtual-machine) (specifically, the **[Besu](#besu)** EVM) to run the smart contract logic privately. This allows for confidential execution, custom gas economics, and controlled access, separate from the public base ledger. For a developer, `pente` is the key to building complex, privacy-preserving applications (e.g., **[Delivery versus Payment (DvP)](#delivery-versus-payment-dvp)**) using familiar Ethereum tools while ensuring sensitive business logic remains confidential.

***

### Registry

The **registry** acts as a lookup service within Paladin's communication architecture. When a [Paladin](#paladin) instance needs to send data, it consults the registry with the recipient's **[routing identity](#identity)**. The registry's job is to return the necessary details for communication, such as the correct [transport](#transport) protocol to use, the physical network address, and the specific encryption keys needed for that secure channel.

***

### Smart Contract

A **Smart Contract** is self-executing code that runs on a blockchain and automatically enforces the terms of an agreement. When specific conditions are met, the code executes, and the results are recorded immutably on the ledger. In [Paladin](#paladin), smart contracts are the core logic of an application and can be executed within different **[Domains](#domain)** to provide specific features, such as the confidentiality offered by the `pente` and `zeto` domains.

***

### State Store

The Paladin **State Store** is a pluggable key/value database responsible for persisting the private data associated with transactions. It is specifically designed around a **[UTXO](#utxo-unspent-transaction-output)** model, which is fundamental to how Paladin's privacy-preserving [domains](#domain) manage state. In this model, the State Store holds the confidential details of a transaction (e.g., ownership, value) as an encrypted data blob. The cryptographic hash of this private data is used as the key in the store and is also what gets committed to the public base ledger. This architecture cleanly separates private state (held securely off-chain) from public proof (recorded on-chain).

***

### Tokens

In Paladin, **tokens** represent digital assets, from financial instruments to identities. The platform provides developers with the tools to create and manage tokens with different, specific privacy models by using different **[Domains](#domain)**. For example, a developer can create a token governed by a trusted **[Notary](#notary)** using the [noto](#noto) domain, or a token with stronger, cryptographically-guaranteed privacy using the **[ZKP](#zkp-zero-knowledge-proof)**-based [zeto](#zeto) domain. This allows for tailoring the privacy and governance of an asset to its specific use case.

***

### Transport

The **transport** is the underlying mechanism that handles the actual asynchronous transfer of messages and events between [Paladin](#paladin) runtimes. The architecture requires that all communication is end-to-end encrypted between the communicating runtimes, ensuring that even on a multi-hop journey, the data can only be decrypted by the final recipient.

***

### UTXO (Unspent Transaction Output)

The **UTXO** model is a method for tracking cryptocurrency ownership. Instead of storing funds as a balance in an account, a user's wallet holds a collection of discrete, **unspent transaction outputs** (UTXOs). This can be analogized to physical currency, where an individual's total funds consist of a collection of discrete bills and coins rather than a single numerical balance in a bank account. When a user wishes to make a payment, their wallet selects one or more of these UTXOs as inputs. The transaction consumes these inputs and creates new UTXOs as outputs: one for the recipient and, if necessary, one for the original user as "change."

***

### zeto

The **zeto** domain is a reference domain that uses **[UTXO](#utxo-unspent-transaction-output)**-based privacy-preserving token toolkit for **[EVM](#evm-ethereum-virtual-machine)**. Unlike [noto](#noto), which relies on a trusted [Notary](#notary) for privacy, `zeto` uses advanced cryptography. It leverages **[Zero-Knowledge Proofs (ZKPs)](#zkp-zero-knowledge-proof)** to allow users to prove ownership and transact tokens without revealing any underlying confidential information. The technical implementation of these proofs is handled using **[Circom](#circom)**, a specialized language for creating ZKP circuits. For a developer, this makes `zeto` the tool of choice for applications that require the strongest level of cryptographically-guaranteed privacy for assets.

***

### ZKP (Zero-Knowledge Proof)

A **Zero-Knowledge Proof** is a cryptographic protocol that allows one party (the prover) to prove to another party (the verifier) that a specific statement is true, without revealing any information beyond the validity of the statement itself. For example, a prover could convince a verifier that they know the solution to a complex puzzle without revealing the solution. The prover can achieve this by presenting a cryptographically transformed version of the puzzle's solution to the verifier. This transformation is designed to be easily verifiable with the original puzzle, yet computationally infeasible to reverse-engineer, thereby proving knowledge without disclosure.
