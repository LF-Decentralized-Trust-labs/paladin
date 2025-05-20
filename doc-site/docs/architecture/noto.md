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
                {"name": "allowLock", "type": "boolean"},
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
* **notaryMode** - choose the notary's mode of operation - must be "basic" or "hooks" (see [Notary logic](#notary-logic) section below)
* **implementation** - (optional) the name of a non-default Noto implementation that has previously been registered
* **options** - options specific to the chosen notary mode (see [Notary logic](#notary-logic) section below)

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

### lock

Lock value from the sender and assign it a new lock ID. Available UTXO states will be selected for spending, and new locked UTXO states will be created.

```json
{
    "name": "lock",
    "type": "function",
    "inputs": [
        {"name": "amount", "type": "uint256"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **amount** - amount of value to lock
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

### unlock

Unlock value that was previously locked, and send it to one or more recipients. Available UTXO states will be selected from the specified lock, and new unlocked UTXO states will be created for the recipients.

```json
{
    "name": "unlock",
    "type": "function",
    "inputs": [
        {"name": "lockId", "type": "bytes32"},
        {"name": "from", "type": "string"},
        {"name": "recipients", "type": "tuple[]", "components": [
            {"name": "to", "type": "string"},
            {"name": "amount", "type": "uint256"}
        ]},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockId** - the lock ID assigned when the value was locked (available from the domain receipt for the `lock` transaction)
* **from** - the lookup string for the owner of the locked value
* **recipients** - array of recipients to receive some of the value (the sum of the amounts must be less than or equal to the total locked amount)
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

### prepareUnlock

Prepare to unlock value that was previously locked. This method is identical to `unlock` except that it will not actually perform the unlock - it will only check that the unlock is valid, and will record a hash of the prepared unlock operation against the lock.

When used in combination with `delegateLock`, this can allow any base ledger address (including other smart contracts) to finalize and execute an unlock that was already approved by the notary.

```json
{
    "name": "prepareUnlock",
    "type": "function",
    "inputs": [
        {"name": "lockId", "type": "bytes32"},
        {"name": "from", "type": "string"},
        {"name": "recipients", "type": "tuple[]", "components": [
            {"name": "to", "type": "string"},
            {"name": "amount", "type": "uint256"}
        ]},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockId** - the lock ID assigned when the value was locked (available from the domain receipt)
* **from** - the lookup string for the owner of the locked value
* **recipients** - array of recipients to receive some of the value (the sum of the amounts must be less than or equal to the total locked amount)
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

### delegateLock

Appoint another address as the delegate that can execute a prepared unlock operation.

Once the lock has been delegated, the notary and the lock creator can no longer interact with the locked states, until the delegate invokes the public ABI to either 1) trigger the unlock _or_ 2) re-delegate the lock to a different address. Delegation can be cancelled if the current delegate re-delegates to the zero address.

```json
{
    "name": "delegateLock",
    "type": "function",
    "inputs": [
        {"name": "lockId", "type": "bytes32"},
        {"name": "unlock", "type": "tuple", "components": [
            {"name": "lockedInputs", "type": "bytes32[]"},
            {"name": "lockedOutputs", "type": "bytes32[]"},
            {"name": "outputs", "type": "bytes32[]"},
            {"name": "signature", "type": "bytes"},
            {"name": "data", "type": "bytes"}
        ]},
        {"name": "delegate", "type": "address"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockId** - the lock ID assigned when the value was locked (available from the domain receipt)
* **unlock** - the parameters for the public `unlock` transaction that was prepared and is now being delegated (available from the domain receipt for the `prepareUnlock` transaction)
* **delegate** - the address that will be allowed to trigger the prepared unlock
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

## Public ABI

The public ABI of Noto is implemented in Solidity by [Noto.sol](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/domains/noto/Noto.sol),
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

### lock

Lock some UTXO states. Generally should not be called directly.

May only be invoked by the notary address.

```json
{
    "name": "lock",
    "type": "function",
    "inputs": [
        {"name": "inputs", "type": "bytes32[]"},
        {"name": "outputs", "type": "bytes32[]"},
        {"name": "lockedOutputs", "type": "bytes32[]"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **inputs** - input states that will be spent
* **outputs** - unlocked output states that will be created
* **lockedOutputs** - locked output states that will be created
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

### unlock

Unlock some UTXO states. May be invoked by the notary in response to a private `unlock` transaction, but may also be called directly on the public ABI when an unlock operation has been prepared and delegated via `prepareUnlock` and `delegateLock`.

```json
{
    "name": "unlock",
    "type": "function",
    "inputs": [
        {"name": "lockedInputs", "type": "bytes32[]"},
        {"name": "lockedOutputs", "type": "bytes32[]"},
        {"name": "outputs", "type": "bytes32[]"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockedInputs** - locked input states that will be spent
* **lockedOutputs** - locked output states that will be created
* **outputs** - unlocked output states that will be created
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

### prepareUnlock

Record the hash of a prepared unlock operation. Generally should not be called directly.

May only be invoked by the notary address.

```json
{
    "name": "prepareUnlock",
    "type": "function",
    "inputs": [
        {"name": "lockedInputs", "type": "bytes32[]"},
        {"name": "unlockHash", "type": "bytes32"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockedInputs** - locked input states that will be spent
* **unlockHash** - EIP-712 hash of the intended unlock, using type `Unlock(bytes32[] lockedInputs,bytes32[] lockedOutputs,bytes32[] outputs,bytes data)`
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

### delegateLock

Appoint another address as the delegate that can execute a prepared unlock operation.

May be invoked by the notary in response to a private `delegateLock` transaction for a lock that is not yet delegated. May also be called directly on the public ABI by the current delegate, to re-delegate to a new address. Delegation can be cancelled if the current delegate re-delegates to the zero address.

```json
{
    "name": "delegateLock",
    "type": "function",
    "inputs": [
        {"name": "unlockHash", "type": "bytes32"},
        {"name": "delegate", "type": "address"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **unlockHash** - EIP-712 hash of the prepared unlock, using type `Unlock(bytes32[] lockedInputs,bytes32[] lockedOutputs,bytes32[] outputs,bytes data)`
* **delegate** - address of the delegate party that will be able to execute the unlock
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

## Notary logic

The notary logic (implemented in the domain [Go library](../../../domains/noto)) is responsible for validating and
submitting all transactions to the base shared ledger.

The notary will validate the following:

- **Request Authenticity:** Each request to the notary will be accompanied by an EIP-712 signature from the sender,
  which is validated by the notary. This prevents any identity from impersonating another when submitting requests.
- **State Validity:** Each request will be accompanied by a proposed set of input and output UTXO states assembled
  by the sending node. The notary checks that these states would be a valid expression of the requested operation -
  for example, a "transfer" must be accompanied by inputs owned only by the "from" address, outputs owned by the
  "to" address matching the desired transfer amount, and optionally some remainder outputs owned by the "from" address.
  Note the distinction here of states that _would be_ valid - as final validation of spent/unspent state IDs will
  be provided by the base ledger.
- **Conservation of Value:** For most operations (other than mint and burn), the notary will ensure that the sum
  of the inputs and of the outputs is equal.

The above constraints cannot be altered without changing the library code. However, many other aspects of the
notary logic can be easily configured as described below.

### Notary mode: basic

When a Noto contract is constructed with notary mode `basic`, the following notary behaviors can be configured:

| Option         | Default | Description |
| -------------- | ------- | ----------- |
| restrictMint   | true    | _True:_ only the notary may mint<br>_False:_ any party may mint |
| allowBurn      | true    | _True:_ token owners may burn their tokens<br>_False:_ tokens cannot be burned |
| allowLock      | true    | _True:_ token owners may lock tokens (for purposes such as preparing or delegating transfers)<br>_False:_ tokens cannot be locked (not recommended, as it restricts the ability to incorporate tokens into swaps and other workflows) |

In addition, the following restrictions will always be enforced, and cannot be disabled in `basic` mode:

- **Unlock:** Only the creator of a lock may unlock it.

### Notary mode: hooks

When a Noto contract is constructed with notary mode `hooks`, the address of a private Pente contract implementing
[INotoHooks](https://github.com/LF-Decentralized-Trust-labs/paladin/blob/main/solidity/contracts/domains/interfaces/INotoHooks.sol)
must be provided. This contract may be deployed into a privacy group only visible to the notary, or into a group
that includes other parties for observability.

The relevant hook will be invoked for each Noto operation, allowing the contract to determine if the operation is
allowed, and to trigger any additional custom policies and side-effects. Hooks can even be used to track Noto token
movements in an alternate manner, such as representing them as a private ERC-20 or other Ethereum token.

Each hook should have one of two outcomes:

- If the operation is allowed, the hook should emit `PenteExternalCall` with the prepared Noto transaction details,
  to allow the Noto transaction to be confirmed.
- If the operation is not allowed, the hook should revert.

Failure to trigger one of these two outcomes will result in undefined behavior.

The `msg.sender` for each hook transaction will always be the resolved notary address, but each hook will also
receive a `sender` parameter representing the resolved and verified party that sent the request to the notary.

!!! important
    Note that none of the `basic` notary constraints described in the previous section will be active when hooks are
    configured. It is the responsibility of the hooks to enforce policies, such as which senders are allowed to mint,
    burn, lock, unlock, etc.

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
