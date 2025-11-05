# Frequently Asked Questions

### What is Paladin?

**Paladin is a framework for building private, programmable applications on top of standard EVM blockchains.** It provides a full stack of technologies to manage the complex, off-chain execution of private transactions while using a public or private blockchain for final settlement, ensuring that confidential data remains private without sacrificing interoperability.

### How does Paladin compare to other privacy solutions?

**Paladin's unique value is its role as an integrator and a standards-setter, not a single, monolithic privacy technology.**

* Unlike privacy coins (e.g., Zcash) which focus only on anonymous payments, Paladin is a full-stack framework for building complex, private applications.
* Unlike previous private EVM implementations, Paladin runs on top of **un-modified** EVM chains and offers a more scalable and robust model for private transactions.
* Instead of being a single solution, Paladin is designed to host many different privacy technologies (`noto`, `zeto`, etc.) in a single runtime, with the goal of creating common standards for private assets, similar to what ERC-20 did for public tokens.

### What is the relationship between Paladin, permissioned chains, L1s and L2s?

Paladin is a client for enabling programmable privacy using any EVM compatible chain as the base ledger. The base ledger could be a permissioned chain, a public L1 or a public L2 chain. Paladin manages what state needs submitting to the base ledger and handles gas calculations where the base ledger requires gas tokens to fund transactions.



Why does Paladin use a base ledger instead of being its own standalone blockchain?

**Paladin uses a base ledger to leverage its security and enable interoperability, rather than creating another isolated ecosystem.** This additive approach provides three key advantages:

* **Security:** It inherits the security, decentralization, and immutability of the underlying base ledger.
* **Interoperability:** All private transactions settle on the same shared EVM ledger, allowing them to be composed with each other and with existing public tokens in a single atomic transaction.
* **Standardization:** It avoids creating another siloed network and instead focuses on providing a common privacy standard for the entire, vast EVM ecosystem.

### Where is the private data actually stored?

**Private transaction data is never written to the public blockchain; it is exchanged directly between participants and stored locally.** The private details are sent securely between the Paladin runtimes involved in a transaction and kept in each participant's local **State Store**. Only a cryptographic proof (a hash) is sent to the base ledger. This proof acts as a public witness, preventing double-spends and ensuring transactional integrity without revealing any of the confidential data.

### What is a Domain and what is its role in the architecture?

**A Domain is an isolated, pluggable execution engine that implements a specific set of privacy-preserving features.** Like a secured workspace within a larger office, a Domain provides a specialized environment tailored for a specific task. This model allows developers to choose the right privacy tool for the job—whether it's for tokens or general smart contracts—without being locked into a single technology.

### What is the difference between the `noto`, `pente`, and `zeto` reference domains?

**These domains provide different tools and trust models for different privacy use cases.**

* **`noto`**: Uses a **Trusted Notary** to manage confidential assets. This is ideal for ecosystems where a central rule-keeper is required to enforce specific rules.
* **`pente`**: Provides **Private EVM Execution** for running complex, confidential business logic and workflows, such as a Delivery versus Payment (DvP) agreement.
* **`zeto`**: Leverages **Zero-Knowledge Proofs** for maximum asset privacy, enabling transactions where no underlying data is revealed to any party.

These domains are _reference_ domains and have been implemented using Paladin's pluggable architecture. Other domains can be implemented to the same plugin interface, for example a zero-knowledge token that uses a different ZK standard to the one used by **zeto**.
### How does Paladin handle interoperability between different privacy domains?

**Paladin achieves interoperability through atomic settlement on the shared base ledger.** This allows a single, seamless transaction to involve multiple steps across different domains. For example, you can atomically swap a `noto` token for an asset managed by a `pente` smart contract. Because all domains ultimately commit a proof to the same underlying EVM ledger, the entire multi-step workflow can be guaranteed to succeed or fail as a single unit, a concept known as **Atomic Programmability**.

### Is Paladin limited to EVM-compatible blockchains?

**Yes, the current implementation of Paladin is designed specifically for the EVM ecosystem.** This focus is intentional, as it allows Paladin to serve the largest existing smart contract community. By targeting the EVM, Paladin can immediately leverage its immense security, established tooling, developer talent, and powerful network effects.

### How does running a private transaction in Paladin affect performance and cost?

**Paladin generally improves both cost and performance by moving heavy computation off-chain.**

* **Cost:** Running a private transaction involves off-chain computation and settles as a smaller, simpler transaction on the base ledger. This typically leads to significantly lower and more predictable gas fees compared to executing complex logic entirely on-chain.
* **Performance:** Paladin's architecture is a generational leap forward. Instead of running a full, persistent private blockchain, its `pente` domain spins up an ephemeral **Besu EVM** in memory just to execute a single transaction. This is a far more scalable and efficient model.

### What programming languages do I use to build on Paladin?

**Developers use Solidity for on-chain smart contracts and any standard language for off-chain client applications.**

* For the **smart contracts** that define your assets and logic within a Domain, you use languages that compile to the **EVM**, primarily **Solidity** or Vyper.
* For **client-side applications** that interact with the Paladin runtime, you can use any language that can communicate via a standard JSON-RPC API to send transactions and query state.

### How do I get started with a local setup?

**You can get a local Paladin network running quickly using the official quick start guide.** This process uses a Kubernetes operator to automatically deploy all the necessary components, including a private Besu blockchain and a three-node Paladin network, right on your laptop.

You can find the complete, step-by-step instructions in the **[Installation Guide](https://LFDT-Paladin.github.io/paladin/head/getting-started/installation/)**.

### Where is the best place to ask questions or get help?

**The best place to connect with the Paladin community and development team is on the Hyperledger Discord server.** This is the primary forum for real-time discussions, questions, and sharing ideas.

* Join the conversation here: **[Hyperledger Discord](https://discord.com/channels/905194001349627914/1303371167020879903)**.

### How should I report a bug?

**Bugs should be reported by creating a new issue in the official GitHub repository.** When reporting a bug, please provide as much detail as possible, including your operating system, project version, and clear steps to reproduce the issue.

* Report a bug here: **[Paladin GitHub Issues](https://github.com/LFDT-Paladin/paladin/issues)**

### What are the guidelines for contributing to Paladin?

**Contributions from the community are highly encouraged and welcome.** The general process involves forking the repository, creating a feature branch for your changes, and submitting a pull request for review. All code contributions must include tests and a "Signed-off-by" line in the commit message to comply with the Developer Certificate of Origin (DCO).

* Explore the code and contribute here: **[Paladin GitHub Repository](https://github.com/LFDT-Paladin/paladin)**

### Who maintains Paladin and what is its governance model?

**Paladin is a lab hosted by the Linux Foundation's LF Decentralized Trust and follows its open-source governance model.** This ensures that the project is managed transparently and collaboratively by its community of maintainers and contributors.