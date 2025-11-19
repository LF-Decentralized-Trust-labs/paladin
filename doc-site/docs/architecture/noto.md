# Noto - Notarized Tokens

The Noto domain provides confidential UTXO tokens which are managed by a single party, referred
to as the notary. Each UTXO state (sometimes referred to as a "coin") encodes an owning address,
an amount, and a randomly-chosen salt. The states are identified on the base ledger only by a
hash of the state data, while the private state data is only exchanged via private channels.

The base ledger provides deterministic ordering, double-spend protection, and provable linkage
of state data to transactions. The private state data provides a record of ownership and value
transfer.

## Private ABI

The private ABI of Noto is implemented in [Go](https://github.com/LFDT-Paladin/paladin/tree/main/domains/noto),
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

* **to** - lookup string for the identity that will receive transferred value
* **amount** - amount of value to transfer
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

### transferFrom

Transfer value from a specified account to another recipient. Available UTXO states will be selected for spending, and
new UTXO states will be created, in order to facilitate the requested transfer of value.

!!! important
    This method is only available in hooks mode. It is disabled in basic mode.

```json
{
    "name": "transferFrom",
    "type": "function",
    "inputs": [
        {"name": "from", "type": "string"},
        {"name": "to", "type": "string"},
        {"name": "amount", "type": "uint256"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **from** - lookup string for the identity whose tokens will be transferred
* **to** - lookup string for the identity that will receive transferred value
* **amount** - amount of value to transfer
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

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

### burnFrom

Burn value from a specified account. Available UTXO states will be selected for burning, and new UTXO
states will be created for the remaining amount (if any).

!!! important
    This method is only available in hooks mode. It is disabled in basic mode.

```json
{
    "name": "burnFrom",
    "type": "function",
    "inputs": [
        {"name": "from", "type": "string"},
        {"name": "amount", "type": "uint256"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **from** - lookup string for the identity whose tokens will be burned
* **amount** - amount of value to burn
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the state receipt)

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

Once the lock has been delegated, the notary and the lock creator can no longer interact with the locked states, until the delegate invokes the public ABI to either 1) trigger the unlock via `spendLock()` _or_ 2) cancel the lock via `cancelLock()` _or_ 3) re-delegate the lock to a different address. Delegation can be cancelled if the current delegate re-delegates to the zero address.

The lock must have been prepared (via `prepareUnlock`) before it can be delegated.

```json
{
    "name": "delegateLock",
    "type": "function",
    "inputs": [
        {"name": "lockId", "type": "bytes32"},
        {"name": "delegate", "type": "address"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockId** - the lock ID assigned when the value was locked (available from the domain receipt)
* **delegate** - the address that will be allowed to trigger the prepared unlock (or zero address to cancel delegation)
* **data** - user/application data to include with the transaction (will be accessible from an "info" state in the
state receipt)

## Public ABI

The public ABI of Noto is implemented in Solidity by [Noto.sol](https://github.com/LFDT-Paladin/paladin/blob/main/solidity/contracts/domains/noto/Noto.sol),
and can be accessed by calling `ptx_sendTransaction` with `"type": "public"`. However, it is not often required
to invoke the public ABI directly.

### LockOptions Struct

The `LockOptions` struct is used to control how a lock may be utilized. It is ABI-encoded and passed as the `options` parameter to `createLock()` or `updateLock()`.

```json
{
    "name": "LockOptions",
    "type": "tuple",
    "components": [
        {"name": "spendTxId", "type": "bytes32"},
        {"name": "spendHash", "type": "bytes32"},
        {"name": "cancelHash", "type": "bytes32"}
    ]
}
```

Fields:

* **spendTxId** - A unique transaction ID that must be used when calling `spendLock()` or `cancelLock()`. This is generated randomly during `prepareUnlock` and stored in the lock options.
* **spendHash** - EIP-712 hash of the intended spend operation, using type `Unlock(bytes32 txId,bytes32[] lockedInputs,bytes32[] outputs,bytes data)`. If set to non-zero, this is the only valid outcome for `spendLock()`. A lock may not be delegated unless both `spendHash` and `cancelHash` have been prepared.
* **cancelHash** - EIP-712 hash of the intended cancel operation, using the same type `Unlock(bytes32 txId,bytes32[] lockedInputs,bytes32[] outputs,bytes data)`. If set to non-zero, this is the only valid outcome for `cancelLock()`. A lock may not be delegated unless both `spendHash` and `cancelHash` have been prepared.

### mint

Mint new UTXO states. Generally should not be called directly.

May only be invoked by the notary address.

```json
{
    "name": "mint",
    "type": "function",
    "inputs": [
        {"name": "txId", "type": "bytes32"},
        {"name": "outputs", "type": "bytes32[]"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **txId** - unique identifier for this transaction which must not have been used before
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
        {"name": "txId", "type": "bytes32"},
        {"name": "inputs", "type": "bytes32[]"},
        {"name": "outputs", "type": "bytes32[]"},
        {"name": "signature", "type": "bytes"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **txId** - unique identifier for this transaction which must not have been used before
* **inputs** - input states that will be spent
* **outputs** - output states that will be created
* **signature** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **data** - encoded Paladin and/or user data

### createLock

Create a new lock by spending states and creating new locked states. Generally should not be called directly.

May only be invoked by the notary address.

```json
{
    "name": "createLock",
    "type": "function",
    "inputs": [
        {"name": "params", "type": "tuple", "components": [
            {"name": "txId", "type": "bytes32"},
            {"name": "inputs", "type": "bytes32[]"},
            {"name": "outputs", "type": "bytes32[]"},
            {"name": "lockedOutputs", "type": "bytes32[]"},
            {"name": "proof", "type": "bytes"},
            {"name": "options", "type": "bytes"}
        ]},
        {"name": "data", "type": "bytes"}
    ],
    "outputs": [
        {"name": "lockId", "type": "bytes32"}
    ]
}
```

Inputs:

* **params.txId** - unique identifier for this transaction which must not have been used before
* **params.inputs** - input states that will be spent (must be only unlocked states)
* **params.outputs** - unlocked output states that will be created
* **params.lockedOutputs** - locked output states that will be created and tied to the lockId
* **params.proof** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **params.options** - ABI-encoded LockOptions struct (optional, may be empty)
* **data** - encoded Paladin and/or user data

Outputs:

* **lockId** - the generated unique identifier for the lock

### updateLock

Update the current options for a lock. Only allowed if the lock has not been prepared already (i.e., both spendHash and cancelHash are zero).

May only be invoked by the notary address.

```json
{
    "name": "updateLock",
    "type": "function",
    "inputs": [
        {"name": "lockId", "type": "bytes32"},
        {"name": "params", "type": "tuple", "components": [
            {"name": "txId", "type": "bytes32"},
            {"name": "lockedInputs", "type": "bytes32[]"},
            {"name": "proof", "type": "bytes"},
            {"name": "options", "type": "bytes"}
        ]},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockId** - unique identifier for the lock
* **params.txId** - unique identifier for this transaction which must not have been used before
* **params.lockedInputs** - locked input states (will not be modified, but will be confirmed to be valid states still locked by the given lockId)
* **params.proof** - sender's signature (not verified on-chain, but can be verified by anyone with the private state data)
* **params.options** - ABI-encoded LockOptions struct containing spendTxId, spendHash, and cancelHash
* **data** - encoded Paladin and/or user data

### spendLock

Consume ("spend") the capability represented by this lock. The caller will either be the notary, or a delegated spender. If the caller is a delegated spender, the lock must have been prepared with a spendHash, and the provided state data must match the spendHash.

```json
{
    "name": "spendLock",
    "type": "function",
    "inputs": [
        {"name": "lockId", "type": "bytes32"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockId** - the identifier of the lock
* **data** - ABI-encoded UnlockData struct containing:
    * **txId** - unique identifier for this transaction (must match the spendTxId from the lock options if set)
    * **inputs** - locked input states that will be spent
    * **outputs** - unlocked output states that will be created
    * **data** - any additional transaction data

### cancelLock

Cancel this lock without performing its effect. The caller will either be the notary, or a delegated spender. If the caller is a delegated spender, the lock must have been prepared with a cancelHash, and the provided state data must match the cancelHash.

```json
{
    "name": "cancelLock",
    "type": "function",
    "inputs": [
        {"name": "lockId", "type": "bytes32"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockId** - the identifier of the lock
* **data** - ABI-encoded UnlockData struct containing:
    * **txId** - unique identifier for this transaction (must match the spendTxId from the lock options if set)
    * **inputs** - locked input states that will be spent
    * **outputs** - unlocked output states that will be created (typically refunding to the owner)
    * **data** - any additional transaction data

### delegateLock

Appoint another address as the delegate that can execute a prepared unlock operation. The lock must have been prepared (both spendHash and cancelHash set).

May be invoked by the notary in response to a private `delegateLock` transaction for a lock that is not yet delegated. May also be called directly on the public ABI by the current delegate, to re-delegate to a new address. Delegation can be cancelled if the current delegate re-delegates to the zero address.

```json
{
    "name": "delegateLock",
    "type": "function",
    "inputs": [
        {"name": "lockId", "type": "bytes32"},
        {"name": "newSpender", "type": "address"},
        {"name": "data", "type": "bytes"}
    ]
}
```

Inputs:

* **lockId** - the identifier of the lock
* **newSpender** - address of the delegate party that will be able to execute the unlock
* **data** - ABI-encoded DelegateLockData struct containing:
    * **txId** - unique identifier for this transaction
    * **data** - any additional transaction data

### balanceOf

Returns the balance information for a specified account. This function provides a quick balance check but is limited to processing up to 1000 states and is not intended to replace the role of a proper indexer for comprehensive balance tracking.

```json
{
  "type": "function",
  "name": "balanceOf",
  "inputs": [
    {
      "name": "account",
      "type": "string"
    }
  ],
  "outputs": [
    {
      "name": "totalStates",
      "type": "uint256"
    },
    {
      "name": "totalBalance",
      "type": "uint256"
    },
    {
      "name": "overflow",
      "type": "bool"
    }
  ]
}
```

Inputs:

- **account** - lookup string for the identity to query the balance for

Outputs:

- **totalStates** - number of unspent UTXO states found for the account
- **totalBalance** - sum of all unspent UTXO values for the account
- **overflow** - indicates if there are at least 1000 states available (true means the returned balance may be incomplete)

**Note:** This function is limited to querying up to 1000 states and should not be used as a replacement for proper indexing infrastructure.

## Notary logic

The notary logic (implemented in the domain [Go library](https://github.com/LFDT-Paladin/paladin/tree/main/domains/noto)) is responsible for validating and
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

- **unlock:** Only the creator of a lock may unlock it.
- **burnFrom:** This method is disabled and will always revert.
- **transferFrom:** This method is disabled and will always revert.

### Notary mode: hooks

When a Noto contract is constructed with notary mode `hooks`, the address of a private Pente contract implementing
[INotoHooks](https://github.com/LFDT-Paladin/paladin/blob/main/solidity/contracts/domains/interfaces/INotoHooks.sol)
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
