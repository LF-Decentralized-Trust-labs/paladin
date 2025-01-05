# Noto - Notarized Tokens

The Noto domain provides confidential UTXO tokens which are managed by a single party, referred
to as the notary. Each UTXO state (sometimes referred to as a "coin") encodes an owning address,
an amount, and a randomly-chosen salt. The states are identified on the base ledger only by a
hash of the state data, while the private state data is only exchanged via private channels.

The base ledger provides deterministic ordering, double-spend protection, and provable linkage
of state data to transactions. The private state data provides a record of ownership and value
transfer.

## Private ABI

The private ABI of Noto is implemented in [Go](https://github.com/LF-Decentralized-Trust-labs/paladin/tree/main/domains/noto),
and can be accessed by calling `ptx_sendTransaction` with `"type": "private"`.

### constructor

Creates a new Noto token, with a new address on the base ledger.

```json
{
    "name": "",
    "type": "constructor",
    "inputs": [
        {"name": "notary", "type": "string"},
        {"name": "notaryMode", "type": "string"},
        {"name": "implementation", "type": "string"},
        {"name": "options", "type": "tuple", "components": [
            {"name": "basic", "type": "tuple", "components": [
                {"name": "restrictMint", "type": "boolean"},
                {"name": "allowBurn", "type": "boolean"},
            ]},
            {"name": "hooks", "type": "tuple", "components": [
                {"name": "privateGroup", "type": "tuple", "components": [
                    {"name": "salt", "type": "bytes32"},
                    {"name": "members", "type": "string[]"}
                ]},
                {"name": "publicAddress", "type": "address"},
                {"name": "privateAddress", "type": "address"}
            ]}
        ]}
    ]
}
```

Inputs:

* **notary** - lookup string for the identity that will serve as the notary for this token instance. May be located at this node or another node
* **notaryMode** - choose the notary's mode of operation - must be "basic" or "hooks"
* **implementation** - (optional) the name of a non-default Noto implementation that has previously been registered
* **options.basic.restrictMint** - (optional - default true) only allow the notary to request mint
* **options.basic.allowBurn** - (optional - default true) allow token owners to request burn
* **options.hooks** - (optional) specify a [Pente](../pente) private smart contract that will be called for each Noto transaction, to provide custom logic and policies

### mint

Mint new value. New UTXO state(s) will automatically be created to fulfill the requested mint.

```json
{
    "name": "mint",
    "type": "function",
    "inputs": [
        {"name": "to", "type": "string"},
        {"name": "amount", "type": "uint256"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **to** - lookup string for the identity that will receive minted value
* **amount** - amount of new value to create
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

### transfer

Transfer value from the sender to another recipient. Available UTXO states will be selected for spending, and
new UTXO states will be created, in order to facilitate the requested transfer of value.

```json
{
    "name": "transfer",
    "type": "function",
    "inputs": [
        {"name": "to", "type": "string"},
        {"name": "amount", "type": "uint256"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **to** - lookup string for the identity that will receive transferred value
* **amount** - amount of value to transfer
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

### approveTransfer

Approve a transfer to be executed by another party.

When calling `ptx_prepareTransaction()` to prepare a private `transfer`, the `metadata` of the prepared transaction
will include information on how to build a proper `approveTransfer` call. This allows preparing a transfer and then
delegating it to another party for execution.

```json
{
    "name": "approveTransfer",
    "type": "function",
    "inputs": [
        {"name": "inputs", "type": "tuple[]", "components": [
            {"name": "id", "type": "bytes"},
            {"name": "schema", "type": "bytes32"},
            {"name": "data", "type": "bytes"}
        ]},
        {"name": "outputs", "type": "tuple[]", "components": [
            {"name": "id", "type": "bytes"},
            {"name": "schema", "type": "bytes32"},
            {"name": "data", "type": "bytes"}
        ]},
        {"name": "data", "type": "bytes"},
        {"name": "delegate", "type": "address"}
    ]
}
```

### burn

Burn value from the sender. Available UTXO states will be selected for burning, and new UTXO
states will be created for the remaining amount (if any).

```json
{
    "name": "burn",
    "type": "function",
    "inputs": [
        {"name": "amount", "type": "uint256"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **amount** - amount of value to burn
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

Inputs:

* **inputs** - input states that will be spent
* **outputs** - output states that will be created
* **data** - encoded Paladin and/or user data
* **delegate** - address of the delegate party that will be able to execute this transaction once approved

## Public ABI

The public ABI of Noto is implemented in Solidity by [Noto.sol](../../solidity/contracts/domains/noto/Noto.sol),
and can be accessed by calling `ptx_sendTransaction` with `"type": "public"`. However, it is not often required
to invoke the public ABI directly.

### mint

Mint new UTXO states. Generally should not be called directly.

May only be invoked by the notary address.

```json
{
    "name": "mint",
    "type": "function",
    "inputs": [
        {"name": "outputs", "type": "bytes32[]"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **outputs** - output states that will be created
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

### transfer

Spend some UTXO states and create new ones. Generally should not be called directly.

May only be invoked by the notary address.

```json
{
    "name": "transfer",
    "type": "function",
    "inputs": [
        {"name": "inputs", "type": "bytes32[]"},
        {"name": "outputs", "type": "bytes32[]"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **inputs** - input states that will be spent
* **outputs** - output states that will be created
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

### approveTransfer

Approve a specific `transfer` transaction to be executed by a specific `delegate` address.
Generally should not be called directly.

The `txhash` should be computed as the EIP-712 hash of the intended transfer, using type:
`Transfer(bytes32[] inputs,bytes32[] outputs,bytes data)`.

May only be invoked by the notary address.

```json
{
    "name": "approveTransfer",
    "type": "function",
    "inputs": [
        {"name": "delegate", "type": "address"},
        {"name": "txhash", "type": "bytes32"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
      ]
}
```

Inputs:

* **delegate** - address of the delegate party that will be able to execute this transaction once approved
* **txhash** - EIP-712 hash of the intended transfer, using type `Transfer(bytes32[] inputs,bytes32[] outputs,bytes data)`
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

### transferWithApproval

Execute a transfer that was previously approved.

The values of `inputs`, `outputs`, and `data` will be used to (re-)compute a `txhash`, which must exactly
match a `txhash` that was previously delegated to the sender via `approveTransfer`.

```json
{
    "name": "transferWithApproval",
    "type": "function",
    "inputs": [
        {"name": "inputs", "type": "bytes32[]"},
        {"name": "outputs", "type": "bytes32[]"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
      ]
}
```

Inputs:

* **inputs** - input states that will be spent
* **outputs** - output states that will be created
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

## Transaction walkthrough

Walking through a simple token transfer scenario, where Party A has some fungible tokens, transfers some to Party B, who then transfers some to Party C.

No information is leaked to Party C, that allows them to infer that Party A and Party B previously transacted.

![Noto transaction walkthrough](../images/noto_transaction_flow_example.png)

1. `Party A` has three existing private states in their wallet and proposes to the notary:
    - Spend states `S1`, `S2` & `S3`
    - Create new state `S4` to retain some of the fungible value for themselves
    - Create new state `S5` to transfer some of the fungible value to `Party B`
2. `Notary` receives the signed proposal from `Party A`
    - Validates that the rules of the token ecosystem are fully adhered to
    - Example: `sum(S1,S2,S3) == sum(S4,S5)`
    - Example: `Party B` is authorized to receive funds
    - Example: The total balance of `Party A` will be above a threshold after the transaction
    - Uses the notary account to submit `TX1` to the blockchain recording signature + hashes
3. `Party B` processes the two parts of the transaction
    - a) Receives the private data for `#5` to allow it to store `S5` in its wallet
    - b) Receives the confirmation from the blockchain that `TX1` created `#5`
    - Now `Party B` has `S5` confirmed in its wallet and ready to spend
4. `Party B` proposes to the notary:
    - Spend state `S5`
    - Create new state `S6` to retain some of the fungible value for themselves
    - Create new state `S7` to transfer some of the fungible value to `Party C`
5. `Notary` receives the signed proposal from `Party B`
    - Validates that the rules of the token ecosystem are fully adhered to
    - Uses the notary account to submit `TX2` to the blockchain recording signature + hashes
3. `Party C` processes the two parts of the transaction
    - a) Receives the private data for `#7` to allow it to store `S7` in its wallet
    - b) Receives the confirmation from the blockchain that `TX2` created `#7`
    - Now `Party C` has `S7` confirmed in its wallet and ready to spend

> TODO: Fill in significantly more detail on how Noto operates (Lead: Andrew Richardson)