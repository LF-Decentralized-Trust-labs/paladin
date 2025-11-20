// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title INotoErrors
 * @dev Definitions for common errors used in Noto implementations.
 */
interface INotoErrors {
    error NotoInvalidInput(bytes32 id);

    error NotoInvalidOutput(bytes32 id);

    error NotoNotNotary(address sender);

    error NotoDuplicateLock(bytes32 lockId);

    error NotoInvalidUnlockHash(bytes32 expected, bytes32 actual);

    error NotoInvalidUnlockInputs(uint256 expected, uint256 actual);

    error NotoNotPrepared(bytes32 lockId);

    error NotoAlreadyPrepared(bytes32 lockId);

    error NotoDuplicateTransaction(bytes32 txId);

    error NotoInvalidOptions(bytes options);

    error NotoInvalidTransaction(bytes32 txId);
}
