# Pente - Private EVM Smart Contracts

EVM Smart Contracts are built in programming languages like Solidity and Vyper, and execute as code installed into an Ethereum `Account`.

In an EVM base ledger blockchain, there is a single world state that all smart contracts are operating on. All state for all accounts exist in this one global state, at one global version.

The world moves forwards block-by-block.

Pente is a privacy preserving smart contract for Paladin, which provides a different model for _Private EVM Smart Contracts_, where many worlds can exist isolated from each other all validated by the same shared ledger.

_Each smart contract is its own world state._

![Private Smart Contracts backed by C-UTXO](./diagrams/private_smart_contract_overview.png)

- Private smart contracts exists uniquely within a privacy group
- The transactions are confirmed by spending UTXO states on the base EVM ledger
- The data required for each transition is confidential
- These state transitions can be verified as part of atomic transactions with other tokens

## History of EVM Private Smart Contracts

The concept of Private EVM Smart Contracts is not new.

In Nov 2016 the first implementation was released called [Constellation](https://github.com/Consensys/constellation), attributed to Samer Falah, Patrick Mylund Nielsen and others at JP Morgan Chase.

This combined a modified version of Go-ethereum (as part of the Quorum project) with a Haskell based private transaction manager.

In this model specially identified (though modified `V` values) private transactions were recorded in-line with public transactions in the same blocks, **after** all parties that were included in the private transactions were previously notified of the inputs to those private transactions.

When the block was confirmed by a node which already had the private input data, it would process the private transaction. Other nodes would skip the transaction, assuming they were not a party to it.

This model was evolved through a number of updates over the years, most significantly:

- The re-building of the Haskell private state management runtime in Java
   - [Orion](https://github.com/connsensys/orion): Started in 2019, abandoned in Sep 2021
   - [Tessera](https://github.com/connsensys/tessera): Started in 2018, still maintained in 2024
- Support for private smart contract submission in Hyperledger Besu (additional to Quorum)
   - The modification to the base EVM was reduced, by avoiding modification of the `V` value of transactions, and rather using a special pre-compiled smart contract address
   - A different approach to Private transaction nonce management was adopted in Besu, where a separate nonce is managed in each privacy group
- Quorum added "Private state validation" (not supported in Besu)
   - Provides a validation mode that helps ensure that all transactions for a given private smart contract are always sent to the same participants

However, the model has not fundamentally changed in this time.

Read here about the [Private transaction lifecycle](https://docs.goquorum.consensys.io/concepts/privacy/private-transaction-lifecycle) for Quorum and Besu when used with the Tessera private transaction manager.

### Problems with the existing model

There are two closely related problems with the EVM Private Smart Contract model as implemented in these generations of the technology.

1. When used correctly, state is locked inside of a Private Smart Contract
   - This is fundamental to the programming model of EVM. A Smart Contract holds a single and complete set of state, visible to all parties in the privacy group.
   - Transacting across smart contracts is problematic
   - Implementing something like an ERC-20/ERC-721 token does not make sense in many scenarios, as the token can never be traded outside of the privacy group (noting that private positions modelled with ERC-20 tokens are a good exception to this statement)
2. The system does not provide feedback when this model is broken, intentionally, or via an operational/system error
    - Because _only the inputs_ are recorded to the blockchain for each transaction, via hash, and private smart contracts share a global address space, it is possible to have multiple overlapping sets of privacy groups transacting against the same smart contract
    - This means **the states diverge** so there is no single source of truth of the state of the private smart contract. Different parties have different data. So the fundamental assurances of a Smart Contract of agreed computation are not net.
    - The "Private state validation" feature (Quorum only) was a step towards solving this issue, by preventing mis-use of private smart contracts across different privacy groups.
    - Read more about these challenges [here](https://www.kaleido.io/blockchain-blog/why-tokens-dont-work-using-private-transactions)

## The updated model in Pente

Pente combined with the sophisticated state management engine of Paladin improves upon the model by using a Confidential-UTXO model to manage the private smart contracts.

The guiding principals are:

- No modification to the base EVM
    - No `V` value changes
    - No special execution during block execution / confirmation
    - The base EVM ledger transaction is a pure EVM transaction
- Each Private Smart Contract exists entirely within a privacy group
    - The address of the smart contract is tied to the uniqueness of the UTXO of the deployment
- Submitter nonce management is managed within each privacy group
    - Now deterministic because the nonce spending is managed inside of the UTXO states
- The Hyperledger Besu EVM is used _as a module_ to executed un-modified EVM transactions
    - These are executed in the Paladin runtime (not in the base Hyperledger Besu runtime)
    - State is managed via the UTXO state storage of Paladin (not the Bonsai/Forrest state store of Besu)
- Pente Private Smart Contract transactions are atomically interoperable via the Paladin framework
    - This is the single most important enhancement. Because Pente is just another privacy preserving smart contract in Paladin, it can atomically interoperate with Token smart contracts. 
    - See the [Atomic Interop](./atomic_interop.md) section for how this enables sophisticated DvP scenarios to be programmed via Private EVM

## Cross-contract invocation rules

> TODO: More to come here (Lead: Peter Broadhurst)