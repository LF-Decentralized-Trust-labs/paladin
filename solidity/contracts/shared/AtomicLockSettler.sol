// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {ReentrancyGuardUpgradeable} from "@openzeppelin/contracts-upgradeable/utils/ReentrancyGuardUpgradeable.sol";
import {IAtomicLockSettler} from "./interfaces/IAtomicLockSettler.sol";
import {ILockableCapability} from "../domains/interfaces/ILockableCapability.sol";

/**
 * @title AtomicLockSettler
 * @dev Atomically spends or cancels a set of ILockableCapability locks.
 */
contract AtomicLockSettler is
    Initializable,
    ReentrancyGuardUpgradeable,
    IAtomicLockSettler
{
    Status public status;
    LockEntry[] private _locks;

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    /**
     * Initialize the AtomicLockSettler with a list of lock entries.
     * @param locks The locks to spend or cancel atomically
     */
    function initialize(LockEntry[] memory locks) external initializer {
        __ReentrancyGuard_init();
        status = Status.Pending;

        for (uint256 i = 0; i < locks.length; i++) {
            _locks.push(locks[i]);
        }

        emit SettlerStatusChanged(status);
    }

    /**
     * @dev Spend all locks atomically.
     * Reverts if the AtomicLockSettler has already been spent or cancelled, or if any spendLock call fails.
     */
    function spend() external nonReentrant {
        if (status != Status.Pending) {
            revert SettlerNotPending();
        }

        for (uint256 i = 0; i < _locks.length; i++) {
            LockEntry storage entry = _locks[i];
            ILockableCapability(entry.contractAddress).spendLock(
                entry.lockId,
                entry.spendInputs,
                entry.data
            );
        }

        status = Status.Spent;
        emit SettlerStatusChanged(status);
    }

    /**
     * @dev Cancel all locks, preventing their spending.
     * Individual cancel operations may fail without reverting the entire cancel (idempotent).
     * Can be called multiple times if already Cancelled - will retry cancel operations each time.
     */
    function cancel() external nonReentrant {
        if (status == Status.Spent) {
            revert SettlerNotPending();
        }

        for (uint256 i = 0; i < _locks.length; i++) {
            LockEntry storage entry = _locks[i];
            (bool success, ) = entry.contractAddress.call(
                abi.encodeCall(
                    ILockableCapability.cancelLock,
                    (entry.lockId, entry.cancelInputs, entry.data)
                )
            );
            if (!success) {
                // intentionally continue even if individual cancels fail (idempotent)
            }
        }

        if (status != Status.Cancelled) {
            status = Status.Cancelled;
            emit SettlerStatusChanged(status);
        }
    }

    /**
     * @dev Simulate the spending of all locks.
     * This function always reverts with the encoded results of the spendLock calls
     * (even if all calls succeed). It should only be used with eth_call,
     * as a transaction with this method will always revert.
     */
    function simulate() external nonReentrant {
        if (status != Status.Pending) {
            revert SettlerNotPending();
        }

        CallResult[] memory results = new CallResult[](_locks.length);
        for (uint256 i = 0; i < _locks.length; i++) {
            LockEntry storage entry = _locks[i];
            (results[i].success, results[i].returnData) = entry
                .contractAddress
                .call(
                    abi.encodeCall(
                        ILockableCapability.spendLock,
                        (entry.lockId, entry.spendInputs, entry.data)
                    )
                );
        }
        revert SpendResult(results);
    }

    function getLockCount() external view returns (uint256) {
        return _locks.length;
    }

    function getLock(uint256 n) external view returns (LockEntry memory) {
        return _locks[n];
    }

    function getLocks() external view returns (LockEntry[] memory) {
        return _locks;
    }
}
