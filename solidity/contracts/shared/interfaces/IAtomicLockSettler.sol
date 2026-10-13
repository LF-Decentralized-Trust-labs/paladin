// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title IAtomicLockSettler
 * @dev Interface for an atomic lock settler.
 *      An atomic lock settler is a contract that can spend or cancel a set
 *      of ILockableCapability locks atomically. All locks can be delegated
 *      to the atomic lock settler, which will then spend or cancel them
 *      together.
 */
interface IAtomicLockSettler {
    struct LockEntry {
        address contractAddress; // ILockableCapability implementation
        bytes32 lockId; // The lock to spend or cancel
        bytes spendInputs; // Passed to spendLock() as spendInputs
        bytes cancelInputs; // Passed to cancelLock() as cancelInputs
        bytes data; // Passed to spendLock()/cancelLock() as data
    }

    struct CallResult {
        bool success;
        bytes returnData;
    }

    enum Status {
        Pending,
        Spent,
        Cancelled
    }

    event SettlerStatusChanged(Status status);

    error SettlerNotPending();

    error SpendResult(CallResult[] result);

    /**
     * @dev Spend all locks atomically.
     * Reverts if the AtomicLockSettler has already been spent or cancelled, or if any spendLock call fails.
     */
    function spend() external;

    /**
     * @dev Cancel all locks, preventing their spending.
     * Individual cancel operations may fail without reverting the entire cancel (idempotent).
     * Can be called multiple times if already Cancelled - will retry cancel operations each time.
     */
    function cancel() external;

    /**
     * @dev Simulate the spending of all locks.
     * This function always reverts with the encoded results of the spendLock calls
     * (even if all calls succeed). It should only be used with eth_call,
     * as a transaction with this method will always revert.
     */
    function simulate() external;

    /**
     * @dev Get the number of locks in the AtomicLockSettler.
     */
    function getLockCount() external view returns (uint256);

    /**
     * @dev Get all locks in the AtomicLockSettler.
     */
    function getLocks() external view returns (LockEntry[] memory);

    /**
     * @dev Get a specific lock in the AtomicLockSettler.
     */
    function getLock(uint256 n) external view returns (LockEntry memory);
}
