// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IConfidentialToken} from "./IConfidentialToken.sol";
import {ILockableCapability} from "./ILockableCapability.sol";

/**
 * @title IConfidentialTokenLockable
 * @dev Extensions for locking/unlocking states.
 *
 *      This interface extends ILockableCapability with opinionated data payloads
 *      and additional methods/events for creating and managing locks that
 *      represent locked confidential token transfers.
 */
interface IConfidentialTokenLockable is
    IConfidentialToken,
    ILockableCapability
{
    // Parameters passed to createLock()
    struct LockParams {
        // A unique identifier for this transaction which must not have been used before
        bytes32 txId;
        // Array of zero or more states that the signer is authorized to spend
        bytes32[] inputs;
        // Array of zero or more new states to generate, for future transactions to spend
        bytes32[] outputs;
        // Array of zero or more locked states to generate, which will be tied to the lockId
        bytes32[] lockedOutputs;
        // Implementation-specific proof for this transaction - may be validated by
        // the smart contract, or may represent evidence of off-chain validation
        bytes proof;
        // Implementation-specific options that control how the lock may be utilized
        bytes options;
    }

    // Parameters passed to updateLock()
    struct UpdateLockParams {
        // A unique identifier for this transaction which must not have been used before
        bytes32 txId;
        // Array of zero or more states locked by the given lockId
        // (will not be modified, but will be confirmed to be valid states still locked by the given lockId)
        bytes32[] lockedInputs;
        // Implementation-specific proof for this transaction - may be validated by
        // the smart contract, or may represent evidence of off-chain validation
        bytes proof;
        // Implementation-specific options that control how the lock may be utilized
        bytes options;
    }

    // Parameters passed (encoded in the data field) to spendLock() and cancelLock()
    struct UnlockData {
        // A unique identifier for this transaction which must not have been used before
        bytes32 txId;
        // Array of zero or more states that the signer is authorized to spend
        bytes32[] inputs;
        // Array of zero or more new states to generate, for future transactions to spend
        bytes32[] outputs;
        // Any additional transaction data (opaque to the blockchain)
        bytes data;
    }

    // Parameters passed (encoded in the data field) to delegateLock()
    struct DelegateLockData {
        // A unique identifier for this transaction which must not have been used before
        bytes32 txId;
        // Any additional transaction data (opaque to the blockchain)
        bytes data;
    }

    /**
     * @dev Emitted when a lock is successfully created.
     */
    event LockCreated(
        bytes32 txId,
        bytes32 indexed lockId,
        address indexed owner,
        address indexed spender,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes32[] lockedOutputs,
        bytes proof,
        bytes data
    );

    /**
     * @dev Emitted when a lock is successfully updated.
     */
    event LockUpdated(
        bytes32 txId,
        bytes32 indexed lockId,
        address indexed operator,
        bytes32[] lockedInputs,
        bytes proof,
        bytes data
    );

    /**
     * @dev Create a new lock, by spending states and creating new locked states.
     *      Locks are identified by a unique lockId, which is generated deterministically.
     *      Locked states can be spent using spendLock(), or control of the lock can be
     *      delegated using delegateLock().
     *
     * @param params The lock parameters (see LockParams struct - note that inputs must be only unlocked states).
     * @param data Any additional transaction data (opaque to the blockchain).
     * @return lockId The generated unique identifier for the lock.
     *
     * Emits a {LockCreated} event.
     */
    function createLock(
        LockParams calldata params,
        bytes calldata data
    ) external returns (bytes32 lockId);

    /**
     * @dev Compute the lockId for given parameters (deterministic generation).
     *      This allows callers to predict the lockId before calling createLock().
     *
     * @param params The lock parameters (implementation may choose to use all or some of the parameters for lockId computation).
     * @return lockId The computed unique identifier for the lock.
     */
    function computeLockId(
        LockParams calldata params
    ) external view returns (bytes32 lockId);

    /**
     * @dev Update the current options for a lock (non-normative method aligned with ILockableCapability recommendations).
     *      Should only be allowed if the lock has not been delegated.
     *
     * @param lockId Unique identifier for the lock.
     * @param params The update parameters (see UpdateLockParams struct).
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {LockUpdated} event.
     */
    function updateLock(
        bytes32 lockId,
        UpdateLockParams calldata params,
        bytes calldata data
    ) external;

    /**
     * @dev Query the lockId for a locked state.
     *
     * @param id The state identifier.
     * @return lockId The lockId set when the lock was created.
     */
    function getLockId(bytes32 id) external view returns (bytes32 lockId);
}
