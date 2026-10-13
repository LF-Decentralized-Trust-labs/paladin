---
name: paladin
description: Complete guide to Paladin — the privacy-preserving transaction manager for EVM. Use this skill whenever a user asks about Paladin, privacy-preserving smart contracts, Zeto ZKP tokens, Noto notarized tokens, Pente private EVM, atomic swap/interop, UTXO-based tokens, programmable privacy on EVM, deploying privacy-preserving contracts, Paladin SDK, Paladin architecture, key management/HSM integration, private transaction management, privacy groups, confidential tokens, DvP (Delivery vs Payment), PvP (Payment vs Payment), or contributing to the LFDT-Paladin project. Also trigger for questions about building on Paladin, running a Paladin node, setting up a development environment, writing Compact-style smart contracts, or integrating Paladin with Hyperledger Besu.
user-invocable: true
license: Apache-2.0
compatibility: Requires Node.js 20+, JDK 17+, Docker, Go 1.22+
metadata:
  author: LFDT-Paladin
  version: 1.0.0
---

# Paladin — Complete Agent Guide

## 1. What Paladin Is

Paladin is an open-source, modular runtime for **programmable privacy on EVM**. It is a sidecar process that runs alongside a Hyperledger Besu node (or any EVM-compliant ledger) and provides:

- **Secure private data channels** between nodes
- **Privacy-preserving smart contracts** — both ZKP-based and notary-based tokens, plus private EVM execution
- **Atomic interoperability** between all privacy contract types on a single shared EVM ledger
- **Enterprise-grade key management** with HSM/SSM integration
- **Distributed transaction coordination** across parties

Paladin is a Linux Foundation Decentralized Trust (LFDT) project, licensed under Apache 2.0.
- Repo: `https://github.com/LFDT-Paladin/paladin`
- Discord: `https://discord.com/channels/905194001349627914/1303371167020879903`
- Docs: `https://LFDT-Paladin.github.io/paladin/head`

## 2. Repository Structure

```
paladin/
├── core/go/          # Core Go runtime: TX manager, state store, transports, JSON/RPC
├── domains/          # Privacy domain implementations
│   ├── noto/         # Notarized token domain (Go)
│   ├── pente/        # Private EVM domain (Java, embeds Besu EVM)
│   └── zeto/         # ZKP token domain (Go)
├── solidity/         # EVM smart contracts (base ledger layer)
│   └── contracts/
│       ├── domains/  # Noto.sol, PentePrivacyGroup.sol, etc.
│       ├── zeto/     # Zeto ZKP contracts (Zeto_Anon, Zeto_AnonEnc, etc.)
│       └── tutorials/ # HelloWorld.sol and example contracts
├── toolkit/          # Protobuf definitions for domain plugins
│   └── proto/protos/
│       ├── to_domain.proto    # Requests from Paladin to a domain
│       └── from_domain.proto  # Callbacks from a domain to Paladin
├── transports/       # Private data transport implementations
├── sdk/              # TypeScript SDK for building DApps on Paladin
├── signingmodules/   # Key management signing modules
│   └── example/      # Reference signing module
├── examples/         # End-to-end tutorial code
│   ├── helloworld/   # Public contract deploy + call
│   ├── notarized-tokens/  # Noto token mint/transfer
│   ├── public-storage/    # Public storage contract
│   ├── private-storage/   # Pente privacy groups
│   ├── private-stablecoin/ # Zeto stablecoin with KYC
│   ├── swap/              # Atomic swap between domains
│   └── event-listener/    # Event listener example
├── operator/         # Kubernetes operator for Paladin deployment
├── doc-site/         # Documentation source (mdBook)
├── config/           # Sample configurations
└── testinfra/        # Integration test infrastructure
```

## 3. Architecture: Three-Layer Programming Model

### Layer A: Base EVM Ledger

Every privacy-preserving smart contract is backed by an EVM smart contract deployed on the base ledger (Hyperledger Besu or any EVM chain).

This layer enforces:
- State transition finalization only when validated (ZKP verification or pre-verified by notary)
- Double-spend protection
- Conformance to standard interfaces for atomic interop

Two approaches to finalization:
1. **ZKP-based**: A zero-knowledge proof is verified on-chain. Anyone can submit with a valid proof.
2. **Notary-based**: The transaction is pre-verified off-chain, and the smart contract records signatures from trusted verifiers. Only the authorized notary can submit.

### Layer B: Private State & Transaction Management

Off-chain code tightly coupled to the EVM contract runs in Paladin:
- Selects valid states (UTXOs) from off-chain data stores
- Gathers endorsements/signatures
- Pre-executes transaction logic against full private data
- Builds proofs or notarization certificates

Uses a **Confidential UTXO (C-UTXO) model**: state is fragmented into independent immutable records identified by salted hashes, enabling parallel transaction construction and scalable private tokens.

### Layer C: Ecosystem Programmability (Private EVM)

EVM private smart contracts (Pente) that run inside **privacy groups**:
- Each privacy group is a mini-blockchain hosted inside one base ledger smart contract
- All transitions are endorsed by 100% of group members
- Private contracts can atomically interoperate with ZKP and notary tokens

---

## 4. Privacy Domains

### 4.1 Zeto — Zero-Knowledge Proof Based Tokens

ZKP-based UTXO tokens using Circom circuits. Natively supported by Paladin as the `zeto` domain.

**Supported token contracts:**
| Contract | Description |
|----------|-------------|
| `Zeto_Anon` | Anonymous transfers, no sender/receiver disclosure |
| `Zeto_AnonEnc` | Anonymous transfers with encrypted values |
| `Zeto_AnonNullifier` | Anonymous transfers with nullifier-based double-spend protection |

**Paladin features for Zeto:**
- **Tokens indexer**: Indexes UTXOs from confirmed on-chain transactions to compute balances
- **Token selector**: Selects UTXOs for spending based on transfer intent
- **ZK proof generator**: Generates ZK proofs using secrets known only to the local Paladin runtime


#### Private ABI

Access via `ptx_sendTransaction` with `"type": "private"`.

**constructor**
```json
{
  "name": "",
  "type": "constructor",
  "inputs": [{ "name": "tokenName", "type": "string" }]
}
```
- `tokenName`: One of `Zeto_Anon`, `Zeto_AnonEnc`, `Zeto_AnonNullifier`

**mint**
```json
{
  "name": "mint",
  "type": "function",
  "inputs": [
    {
      "name": "mints",
      "type": "tuple[]",
      "components": [
        { "name": "to", "type": "string" },
        { "name": "amount", "type": "uint256" }
      ]
    }
  ]
}
```

**transfer**
```json
{
  "name": "transfer",
  "type": "function",
  "inputs": [
    {
      "name": "transfers",
      "type": "tuple[]",
      "components": [
        { "name": "to", "type": "string" },
        { "name": "amount", "type": "uint256" }
      ]
    }
  ]
}
```

**balanceOf**
```json
{
  "name": "balanceOf",
  "type": "function",
  "inputs": [{ "name": "account", "type": "string" }],
  "outputs": [
    { "name": "totalStates", "type": "uint256" },
    { "name": "totalBalance", "type": "uint256" },
    { "name": "overflow", "type": "bool" }
  ]
}
```
Note: Limited to 1000 states. Overflow=true means results may be incomplete.

**deposit** — Swap ERC20 balances for Zeto tokens (1:1 exchange rate)
```json
{
  "name": "deposit",
  "type": "function",
  "inputs": [{ "name": "amount", "type": "uint256" }]
}
```

**withdraw** — Swap Zeto tokens back to ERC20
```json
{
  "name": "withdraw",
  "type": "function",
  "inputs": [{ "name": "amount", "type": "uint256" }]
}
```

**lockProof** — Lock a ZK proof to a delegate for multi-party atomic settlement
```json
{
  "name": "lockProof",
  "type": "function",
  "inputs": [
    { "name": "delegate", "type": "address" },
    { "name": "call", "type": "bytes" }
  ]
}
```
- `delegate`: Ethereum address allowed to submit the locked proof
- `call`: ABI-encoded bytes of a `transfer()` call on the target Zeto contract

### 4.2 Noto — Notarized Tokens

Confidential UTXO tokens managed by a single trusted party (the **notary**). Each UTXO state encodes an owner address, amount, and random salt. States are hashed on-chain; private data is exchanged off-chain.

**Notary validation:**
- Request authenticity (EIP-712 signatures)
- State validity (UTXO inputs/outputs match the operation)
- Conservation of value (sum of inputs = sum of outputs, except mint/burn)

**Notary modes:**
| Mode | Description |
|------|-------------|
| `basic` | Configurable options: `restrictMint`, `allowBurn`, `allowLock`. No hooks. |
| `hooks` | Delegates policy decisions to a Pente private EVM contract implementing `INotoHooks`. None of the basic constraints are active — the hooks contract enforces all policies. |

#### Private ABI

**constructor**
```json
{
  "name": "",
  "type": "constructor",
  "inputs": [
    { "name": "notary", "type": "string" },
    { "name": "notaryMode", "type": "string" },
    { "name": "implementation", "type": "string" },
    { "name": "options", "type": "tuple", "components": [
      { "name": "basic", "type": "tuple", "components": [
        { "name": "restrictMint", "type": "boolean" },
        { "name": "allowBurn", "type": "boolean" },
        { "name": "allowLock", "type": "boolean" }
      ]},
      { "name": "hooks", "type": "tuple", "components": [
        { "name": "privateGroup", "type": "tuple", "components": [
          { "name": "salt", "type": "bytes32" },
          { "name": "members", "type": "string[]" }
        ]},
        { "name": "publicAddress", "type": "address" },
        { "name": "privateAddress", "type": "address" }
      ]}
    ]}
  ]
}
```

**mint**
```json
{
  "name": "mint",
  "type": "function",
  "inputs": [
    { "name": "to", "type": "string" },
    { "name": "amount", "type": "uint256" },
    { "name": "data", "type": "bytes" }
  ]
}
```

**transfer**
```json
{
  "name": "transfer",
  "type": "function",
  "inputs": [
    { "name": "to", "type": "string" },
    { "name": "amount", "type": "uint256" },
    { "name": "data", "type": "bytes" }
  ]
}
```

**transferFrom** — hooks mode only
```json
{
  "name": "transferFrom",
  "type": "function",
  "inputs": [
    { "name": "from", "type": "string" },
    { "name": "to", "type": "string" },
    { "name": "amount", "type": "uint256" },
    { "name": "data", "type": "bytes" }
  ]
}
```

**burn**
```json
{
  "name": "burn",
  "type": "function",
  "inputs": [
    { "name": "amount", "type": "uint256" },
    { "name": "data", "type": "bytes" }
  ]
}
```

**burnFrom** — hooks mode only
```json
{
  "name": "burnFrom",
  "type": "function",
  "inputs": [
    { "name": "from", "type": "string" },
    { "name": "amount", "type": "uint256" },
    { "name": "data", "type": "bytes" }
  ]
}
```

**lock** — Lock value for atomic settlement
```json
{
  "name": "lock",
  "type": "function",
  "inputs": [
    { "name": "amount", "type": "uint256" },
    { "name": "data", "type": "bytes" }
  ]
}
```

**unlock**
```json
{
  "name": "unlock",
  "type": "function",
  "inputs": [
    { "name": "lockId", "type": "bytes32" },
    { "name": "from", "type": "string" },
    {
      "name": "recipients", "type": "tuple[]", "components": [
        { "name": "to", "type": "string" },
        { "name": "amount", "type": "uint256" }
      ]
    },
    { "name": "data", "type": "bytes" }
  ]
}
```

**prepareUnlock** — Validate + record hash of an unlock, for later delegation
```json
{
  "name": "prepareUnlock",
  "type": "function",
  "inputs": [
    { "name": "lockId", "type": "bytes32" },
    { "name": "from", "type": "string" },
    {
      "name": "recipients", "type": "tuple[]", "components": [
        { "name": "to", "type": "string" },
        { "name": "amount", "type": "uint256" }
      ]
    },
    { "name": "unlockData", "type": "bytes" },
    { "name": "data", "type": "bytes" }
  ]
}
```

**delegateLock** — Appoint a delegate to execute the prepared unlock
```json
{
  "name": "delegateLock",
  "type": "function",
  "inputs": [
    { "name": "lockId", "type": "bytes32" },
    { "name": "unlock", "type": "tuple", "components": [
      { "name": "lockedInputs", "type": "bytes32[]" },
      { "name": "lockedOutputs", "type": "bytes32[]" },
      { "name": "outputs", "type": "bytes32[]" },
      { "name": "signature", "type": "bytes" },
      { "name": "data", "type": "bytes" }
    ]},
    { "name": "delegate", "type": "address" },
    { "name": "data", "type": "bytes" }
  ]
}
```

Lock state machine: `AVAILABLE → LOCK → DELEGATED_LOCK`. The delegate can either complete the spend or re-delegate back to the owner (cancel).

### 4.3 Pente — Private EVM Smart Contracts

EVM smart contracts running within **privacy groups**. Each privacy group is a unique EVM smart contract on the base ledger containing the state of one or more private contracts.

**Key design principles:**
- No modification to the base EVM — uses standard EVM transactions
- Each privacy group = one base ledger smart contract
- On-chain verification via EIP-712 threshold signatures from group members
- Ephemeral Besu EVMs execute in-Paladin (no local Besu node required for private execution)
- UTXO accounts for state/code/nonce management
- Atomically interoperable with other Paladin domains

**How it differs from Tessera/Constellation:**
- Full pre-submission endorsement of transaction execution
- On-chain signature verification of commitments to input/output states
- Double-spend protection enforced on-chain
- No state divergence problem — the base ledger is the source of truth

#### Privacy Group Lifecycle

1. **Create group**: `pgroup_createGroup` distributes group info off-chain to all members
2. **Confirm receipt**: Wait for receipt containing the `PrivacyGroup` with on-chain address
3. **Submit transactions**: `pgroup_sendTransaction` with `PrivacyGroupEVMTXInput` (similar to `eth_sendTransaction`)
4. **Listen for receipts**: WebSocket-based `TransactionReceiptListener` with `incompleteStateReceiptBehavior: "block_contract"`
5. **Decode receipts**: `ptx_getDomainReceipt` decodes full EVM receipt with logs/events
6. **Query state**: `pgroup_call` with `PrivacyGroupEVMCall` (similar to `eth_call`)

#### Private ABI

The `to`, `inputs`, `gas`, `value`, and `bytecode` are nested inside:
```json
{
  "name": "<function name or 'deploy'>",
  "type": "function",
  "inputs": [
    { "name": "group", "type": "tuple", "components": [
      { "name": "salt", "type": "bytes32" },
      { "name": "members", "type": "string[]" }
    ]},
    { "name": "to", "type": "address" },
    { "name": "inputs", "type": "tuple", "components": [
      // contract-specific inputs
    ]}
  ],
  "outputs": "..."
}
```

#### Private Messaging

Privacy groups support off-chain messages via `pgroup_sendMessage` — no blockchain transaction overhead. No ordering or non-repudiation guarantees.

---

## 5. Atomic Interop

Different privacy-preserving smart contracts can interoperate atomically on a single EVM ledger.

**Supported combinations:**
- Zeto (ZKP cash token) + Noto (notarized bond token) + Pente (private EVM DvP contract)
- All in one atomic EVM transaction

### Approval-Based Atomic Transaction Flow

1. **Pre-approval/Setup**: Parties construct their individual transactions
2. **Approval/Prepare**: Each domain approves the swap contract address to finalize its sub-transaction
   - Pente: endorsed by privacy group members
   - Noto: notarized by the issuer
   - Zeto: ZK proof submitted
3. **Execution/Commit**: A single `execute()` call on the swap contract spends UTXOs across all domains
4. **Post-execution**: Each party can spend their new tokens independently, without revealing source

---

## 6. Transaction Manager

The core engine receives API requests and coordinates transaction assembly/submission/confirmation.

**Event-driven, async stage-based processing:**
- Initiated by: user instructions, peer messages, or base ledger events
- Idle transactions stored in DB; only actively processed transactions stay in memory
- Critical changes persisted between stage tasks for crash recovery
- Stage tasks designed with idempotency for safe retries

**Key components:**
- **TX assembly & submission**: Selects UTXO states optimistically, coordinates with other Paladins to mitigate contention
- **Smart contract API**: Validates and processes domain-specific instructions
- **TX confirmation & events**: Processes confirmed blockchain transactions against private state, handles out-of-order data arrival
- **Domain plug points**: Standardized gRPC interfaces for ZKP proofs, private EVM, notarization (see `toolkit/proto/protos/`)

---

## 7. Key Management

Paladin's key management handles enterprise HSM/SSM integration across multiple signing algorithms.

**Key architecture:**
- **In-memory vs in-key-store signing**: Choose per use case whether keys leave the HSM
- **Embedded vs remote signing modules**: Run signing in-process or on separate infrastructure via HTTPS+JSON or gRPC+mTLS
- **Multiple signing modules**: Mix embedded and remote modules in one node

**Key resolution:**
- **Direct mapping**: 1:1 relationship between key identifier and HSM key. Bidirectional — can resolve or discover.
- **Key derivation (BIP32)**: Many keys from one seed. BIP-39 mnemonics supported. BIP-44 derivation paths (e.g., `m / 44' / 60' / 0' / 1 / 2 / 3`).

**Key identifiers:**
- Human-friendly strings organized in folders with `/` separators
- Can reference by Ethereum address using `eth_address:` prefix (e.g., `"from": "eth_address:0xf8f3fcf26a437cac6f8bdc92257f3b03e2f5c546"`)
- Each key mapping stores attributes (`name`, `index`) automatically

**Public verifiers:**
- Paladin supports multiple verifier types per key (Ethereum address, IDEN3 Baby JubJub, etc.)
- Pluggable — Zeto domain adds its own verifier types for ZKP circuits

**Signing algorithms supported:** `secp256k1`, `Baby JubJub`, and domain-specific ZKP algorithms

---

## 8. Data Transports & Registry

**Two identity types:**
1. **Account signing identities** — Sign transactions, hold state. Long-lived, complex to change.
2. **Runtime routing identities** — Transport routing (IPs, hostnames, topics). Infrastructure-only, can rotate.

**Routing addresses format:** `account_identifier@routing_identifier`
- Account identifier: resolves to a signing key via the address book
- Routing identifier: resolves to a transport address via the registry plugin

**Transport principles:**
- Async event/message transfer (even HTTP is async — responses come in a separate request)
- Idempotent requests with automatic retry
- End-to-end encryption between Paladin runtimes (even through hub/spoke transports)

---

## 9. UTXO State Store

Private states stored in enterprise RDBMS (any SQL database). DDL migrations provided for multiple databases.

**Schemas & Labels:**
- Each domain registers a `schema` (hashed, like states) before storing states
- `labels` are dynamic indexed fields based on the schema's `indexed` parameters
- Schemas are isolated per domain

**ABI type system:**
Uses Ethereum ABI types via EIP-712 `TypedData`. Supported indexed types:

| Type | DB storage |
|------|-----------|
| `string` | Text |
| `bytes1` to `bytes32` | Hex-encoded bytes |
| `uint8` to `uint63` | 8-byte signed numbers |
| `uint64` to `uint256` | 64-char fixed-width big-endian hex |
| `address` | Same as `uint160` |
| `int8` to `int64` | 8-byte signed numbers |
| `int65` to `int256` | 65-char two's complement hex |
| `bool` | `int64(0)`/`int64(1)` |

**Hashing:** EIP-712 `hashStruct(message)` v4.

**Query language:** JSON-based with operators (`eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`, `nin`, `or`, `and`, `sort`):
```json
{
  "gte": [{ "field": "amount", "value": "12300000000000000000" }],
  "or": [
    { "in": [{ "field": "color", "values": ["red","blue"] }] },
    { "eq": [{ "field": "isSpecial", "value": true }] }
  ],
  "sort": ["amount ASC", ".created DESC"]
}
```

---

## 10. SDK: TypeScript API

Package: `@lfdecentralizedtrust/paladin-sdk`

### Client Initialization

```typescript
import PaladinClient from "@lfdecentralizedtrust/paladin-sdk";

const paladin = new PaladinClient({
  url: "http://localhost:8545",
  // or node.clientOptions from config
});

// Get verifiers (resolves key identities by lookup string)
const [owner] = paladin.getVerifiers("owner@node1");
```

### JSON/RPC API Namespaces

#### `ptx_*` — Private Transaction Manager

| Method | Purpose |
|--------|---------|
| `ptx.sendTransaction(input)` | Submit a transaction (public or private). Returns `txID`. |
| `ptx.call(input)` | Simulate a transaction (read-only). |
| `ptx.getTransaction(txId)` | Get full transaction details. |
| `ptx.getDomainReceipt(domain, txId)` | Decode domain-specific receipt (EVM logs for Pente, state info for Noto/Zeto). |
| `ptx.getStateReceipt(txId)` | Get state receipt with locked/confirmed/available/spent states. |
| `ptx.getPreparedTransaction(txId)` | Get prepared transaction details. |
| `ptx.getDispatch(id)` | Get dispatch details. |
| `ptx.createReceiptListener(listener)` | Create WebSocket listener for transaction receipts. |
| `ptx.createBlockchainEventListener(listener)` | Listen for blockchain events. |
| `ptx.decodeCall(callData, format)` | Decode ABI call data. |
| `ptx.decodeEvent(topics, data, format)` | Decode ABI event data. |
| `ptx.decodeError(revertData, format)` | Decode revert reason. |
| `ptx.getTransactionByIdempotencyKey(key)` | Get transaction by idempotency key. |
| `ptx.getStoredABI(hashRef)` | Get stored ABI by hash. |

**Transaction input** (`TransactionInput`):
```typescript
{
  type: TransactionType.PUBLIC | TransactionType.PRIVATE,
  abi: ABIEntry[],
  function?: string,       // omit for deployment
  bytecode?: string,       // only for deployment
  from: string,            // lookup string
  to?: string,             // omit for deployment
  data: Record<string, any> // function arguments
}
```

#### `pgroup_*` — Privacy Group Management

| Method | Purpose |
|--------|---------|
| `pgroup_createGroup(spec)` | Create a privacy group. Returns `PrivacyGroup`. |
| `pgroup_sendTransaction(input)` | Send transaction within a privacy group. Returns `txID`. |
| `pgroup_call(call)` | Call (read) within a privacy group. |
| `pgroup_sendMessage(input)` | Send off-chain message to group members. |
| `pgroup_getGroupByAddress(address)` | Get group by contract address. |
| `pgroup_getGroupById(domainName, id)` | Get group by domain + ID. |
| `pgroup_queryGroups(query)` | Query groups by properties. |
| `pgroup_queryGroupsWithMember(member, query)` | Query groups a member belongs to. |
| `pgroup_createMessageListener(listener)` | WebSocket listener for private messages. |

**PrivacyGroupInput:**
```typescript
{
  name: string;
  members: string[];  // lookup strings of members
  properties?: Record<string, string>; // searchable metadata
}
```

#### `bidx_*` — Blockchain Indexer

| Method | Purpose |
|--------|---------|
| `bidx.getConfirmedBlockHeight()` | Get latest confirmed block number. |
| `bidx.getBlockByNumber(number)` | Get indexed block. |
| `bidx.getBlockTransactionsByNumber(number)` | Get all transactions in a block. |
| `bidx.getTransactionByHash(hash)` | Get transaction by hash. |
| `bidx.getTransactionByNonce(from, nonce)` | Get transaction by sender + nonce. |
| `bidx.decodeTransactionEvents(txHash, abi, format)` | Decode events from a confirmed transaction. |
| `bidx.getTransactionEventsByHash(hash)` | Get raw indexed events. |
| `bidx.queryIndexedBlocks(query)` | Query blocks. |
| `bidx.queryIndexedEvents(query)` | Query events. |
| `bidx.queryIndexedTransactions(query)` | Query transactions. |

#### `keymgr_*` — Key Manager

| Method | Purpose |
|--------|---------|
| `keymgr.resolveKey(identifier)` | Resolve a key identifier. |
| `keymgr.getVerifier(identifier, algorithm, verifierType)` | Get a specific verifier for a key. |
| `keymgr.listKeys()` | List all managed keys. |
| `keymgr.listWallets()` | List all HD wallets. |

#### `pstate_*` — Private State Store

| Method | Purpose |
|--------|---------|
| `pstate.queryStates(query)` | Query private states across domains. |
| `pstate.getState(domain, stateId)` | Get a specific state by ID. |

#### `transport_*` — Transport Operations

| Method | Purpose |
|--------|---------|
| `transport.getTransports()` | List available transports. |
| `transport.getLocalDetails()` | Get local transport details. |

#### `reg_*` — Registry

| Method | Purpose |
|--------|---------|
| `reg.queryEntries(query)` | Query registry entries. |

### Polling for Receipts

```typescript
const receipt = await paladin.pollForReceipt(txId, timeoutMs, includeDetails);
if (receipt?.success) {
  // transaction confirmed
  console.log(receipt.transactionHash, receipt.contractAddress);
}
```

### Domain Receipt Helpers

```typescript
// For Pente (private EVM): decode EVM logs
const domainReceipt = await paladin.ptx.getDomainReceipt("pente", txId);

// For Noto/Zeto: get state info
const stateReceipt = await paladin.ptx.getStateReceipt(txId);
// stateReceipt.spent, stateReceipt.confirmed, stateReceipt.available
```

### NotoFactory (Notarized Token Helper)

```typescript
import { NotoFactory } from "@lfdecentralizedtrust/paladin-sdk";

const notoFactory = new NotoFactory(paladinClient, "noto");
const cashToken = await notoFactory
  .newNoto(verifier, {
    name: "CASH_TOKEN",
    symbol: "CASH",
    notary: verifier,
    notaryMode: "basic",
  })
  .waitForDeploy(timeout);

// Mint
const mintReceipt = await cashToken
  .mint(verifier, { to: verifier, amount: 2000, data: "0x" })
  .waitForReceipt(timeout);

// Transfer
const transferReceipt = await cashToken
  .transfer(verifier, { to: verifier2, amount: 1000, data: "0x" })
  .waitForReceipt(timeout);

// Transfer from another node
const transferReceipt2 = await cashToken
  .using(paladinClient2)
  .transfer(verifier2, { to: verifier3, amount: 500, data: "0x" })
  .waitForReceipt(timeout);

// Balance check
const balance = await cashToken.balanceOf(verifier, { account: verifier.lookup });
// balance.totalStates, balance.totalBalance, balance.overflow
```

---

## 11. Code Patterns (from Real Examples)

### Pattern 1: Public Smart Contract (Hello World)

```typescript
import PaladinClient, { TransactionType } from "@lfdecentralizedtrust/paladin-sdk";

const paladin = new PaladinClient(connection.clientOptions);
const [owner] = paladin.getVerifiers(`owner@${connection.id}`);

// Deploy
const deployTxId = await paladin.ptx.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: contractJson.abi,
  bytecode: contractJson.bytecode,
  from: owner.lookup,
  data: {},
});
const receipt = await paladin.pollForReceipt(deployTxId, 10000, true);
const contractAddress = receipt.contractAddress;

// Call function
const callTxId = await paladin.ptx.sendTransaction({
  type: TransactionType.PUBLIC,
  abi: contractJson.abi,
  function: "sayHello",
  from: owner.lookup,
  to: contractAddress,
  data: { name: "Paladin User" },
});
const callReceipt = await paladin.pollForReceipt(callTxId, 10000, true);

// Decode events
const events = await paladin.bidx.decodeTransactionEvents(
  callReceipt.transactionHash,
  contractJson.abi,
  "pretty=true"
);
console.log(events[0].data["message"]);
```

### Pattern 2: Private Storage (Privacy Group)

```typescript
// Create privacy group
const createGroupTxId = await paladin.ptx.sendTransaction({
  type: TransactionType.PRIVATE,
  domain: "pente",
  abi: penteFactoryAbi,
  function: "createGroup",
  from: owner.lookup,
  data: {
    group: {
      salt: generateSalt(),
      members: [member1.lookup, member2.lookup],
    },
  },
});
const groupReceipt = await paladin.pollForReceipt(createGroupTxId, 10000, true);
const groupAddress = groupReceipt.contractAddress;

// Deploy contract in the group
const deployTxId = await paladin.pgroup.sendTransaction({
  group: { address: groupAddress },
  abi: contractJson.abi,
  bytecode: contractJson.bytecode,
  from: owner.lookup,
  data: {},
});
```

### Pattern 3: Multi-Node Noto Token Transfer

```typescript
// Deploy on node1
const notoFactory = new NotoFactory(client1, "noto");
const token = await notoFactory
  .newNoto(verifier1, {
    name: "TOKEN",
    symbol: "TKN",
    notary: verifier1,
    notaryMode: "basic",
  })
  .waitForDeploy(timeout);

// Mint
await token.mint(verifier1, { to: verifier1, amount: 1000, data: "0x" })
  .waitForReceipt(timeout);

// Transfer to node2 (from node1)
await token.transfer(verifier1, { to: verifier2, amount: 400, data: "0x" })
  .waitForReceipt(timeout);

// Transfer from node2 to node3 (using node2's client)
await token
  .using(client2)
  .transfer(verifier2, { to: verifier3, amount: 200, data: "0x" })
  .waitForReceipt(timeout);
```

### Pattern 4: Receipt Listener (WebSocket)

```typescript
const listener = await paladin.ptx.createReceiptListener({
  name: "my-listener",
  filters: {
    type: TransactionType.PRIVATE,
    domain: "noto",
  },
  incompleteStateReceiptBehavior: "block_contract",
});

// In production, use WebSocket events — for simple cases use pollForReceipt
```

---

## 12. Tutorials (Learning Path)

Follow in sequence:

1. **Hello World** — Public contract deploy + call + event decode
2. **Public Storage** — Write/read public contract state
3. **Private Storage** — Pente privacy groups, confidential contract data
4. **Notarized Tokens** — Noto token mint, transfer, balance checks, multi-node
5. **Wholesale CBDC** — Zeto ZKP tokens with deposit/withdraw
6. **Private Stablecoin** — Zeto stablecoin with KYC and nullifier protection
7. **Atomic Swap** — Cross-domain atomic settlement (Noto + Zeto + Pente)
8. **Bond Issuance** — Noto tokens + Pente groups for bond lifecycle

All tutorial code: `examples/*/src/index.ts`

---

## 13. Building & Development

### Prerequisites
- JDK 17+
- Protoc (gRPC protocol buffers compiler)
- Docker + Docker Compose
- Node.js 20+
- Go 1.22+

### Build
```shell
./gradlew build
```

### Run full dev environment
See [operator/README.md](https://github.com/LFDT-Paladin/paladin/tree/main/operator) for running a complete multi-node Paladin+Besu stack.

### Project language breakdown
| Component | Language | Build |
|-----------|----------|-------|
| Core runtime | Go | `go build` via Gradle |
| Noto domain | Go | Integrated into core |
| Zeto domain | Go | Integrated into core |
| Pente domain | Java | Gradle (embeds Besu EVM) |
| Solidity contracts | Solidity | Hardhat/Truffle |
| SDK | TypeScript | npm |
| Signing modules | Java | Gradle |
| Toolkit (protos) | Protobuf | protoc |

---

## 14. Contributing

1. Review [LFDT contribution guidelines](https://www.lfdecentralizedtrust.org/how-to-contribute)
2. Browse [open issues](https://github.com/LFDT-Paladin/paladin/issues)
3. All contributions under Apache 2.0 license with DCO sign-off
4. Join [Discord](https://discord.com/channels/905194001349627914/1303371167020879903)
5. Maintainers channel: `#paladin-maintainers` on Discord
6. TSC governs technical direction (see `Paladin_Technical_Charter.md`)

### Adding a new domain
1. Define Protobuf interfaces in `toolkit/proto/protos/` (`to_domain.proto`, `from_domain.proto`)
2. Implement the domain in Go or Java under `domains/`
3. Implement base ledger EVM contract in `solidity/contracts/domains/`
4. Add configuration in `config/`
5. Add integration tests in `testinfra/`

### Release streams
- **`main`**: Stable releases, at least monthly
- **`v1-develop`**: Upcoming V1.0 with M-of-N endorsement and token interface standardization
- Track V1.0 progress: https://github.com/orgs/LFDT-Paladin/projects/3

---

## 15. External References

- **Zeto** (ZKP circuits and Solidity): https://github.com/LFDT-Paladin/zeto
- **Paladin Docs**: https://LFDT-Paladin.github.io/paladin/head
- **Examples**: https://github.com/LFDT-Paladin/paladin/tree/main/examples
- **Hyperledger Besu**: https://github.com/hyperledger/besu
- **Circom** (ZKP circuit language): https://iden3.io/circom
- **Anonymous Zether**: https://github.com/kaleido-io/anonymous-zether-client
