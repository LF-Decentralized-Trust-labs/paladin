// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title IConfidentialToken
 * @dev Base interface for a confidential token.
 *
 *      All token transactions are represented as state transitions, where details (such as
 *      ownership and amounts) are kept private but provably bound to the state IDs.
 *
 *      The interface is agnostic to the model chosen for proving the validity of a given
 *      transition, and the method of distributing any private data associated with the states.
 */
interface IConfidentialToken {
    event Transfer(
        bytes32 txId,
        address indexed operator,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes proof,
        bytes data
    );

    /**
     * @dev Spend states and create new states.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param inputs array of zero or more states that the signer is authorized to spend
     * @param outputs array of zero or more new states to generate, for future transactions to spend
     * @param proof implementation-specific proof for this transaction - may be validated by
     *              the smart contract, or may represent evidence of off-chain validation
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {Transfer} event.
     */
    function transfer(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata proof,
        bytes calldata data
    ) external;

    /**
     * @dev Perform a transfer with no input states.
     *      Behavior should be identical to transfer(), but may come with different constraints on execution.
     */
    function mint(
        bytes32 txId,
        bytes32[] calldata outputs,
        bytes calldata proof,
        bytes calldata data
    ) external;

    /**
     * @dev query whether a TXO is currently in the unspent list
     * @param id the state identifier
     * @return unspent true or false depending on whether the identifier is in the unspent map
     */
    function isUnspent(bytes32 id) external view returns (bool unspent);
}
