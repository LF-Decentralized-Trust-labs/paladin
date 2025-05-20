// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";

/**
 * Helpers for tracking locked amounts from Noto hooks contracts.
 */
contract NotoLocks {
    // Details on all currently active locks and their possible unlocks
    mapping(bytes32 => LockDetail) internal _locks;

    // Balances locked by address (still logically owned by that address)
    mapping(address => uint256) public lockedBalance;

    // Pending balances from prepared unlocks (not yet owned, but approved to be owned when unlocked)
    mapping(address => uint256) public pendingBalance;

    struct LockDetail {
        address from;
        uint256 amount;
        INotoHooks.UnlockRecipient[] recipients;
    }

    function getLock(bytes32 lockId) public view returns (LockDetail memory) {
        return _locks[lockId];
    }

    function ownerOf(bytes32 lockId) public view returns (address) {
        return _locks[lockId].from;
    }

    function onLock(bytes32 lockId, address from, uint256 amount) public {
        LockDetail storage lock = _locks[lockId];
        lock.from = from;
        lock.amount = amount;
        lockedBalance[from] += amount;
    }

    function onUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) public {
        LockDetail storage lock = _locks[lockId];
        for (uint256 i = 0; i < recipients.length; i++) {
            lock.amount -= recipients[i].amount;
            lockedBalance[lock.from] -= recipients[i].amount;
        }

        delete lock.recipients;
        if (lock.amount == 0) {
            delete _locks[lockId];
        }
    }

    function onPrepareUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) public {
        LockDetail storage lock = _locks[lockId];
        delete lock.recipients;

        for (uint256 i = 0; i < recipients.length; i++) {
            pendingBalance[recipients[i].to] += recipients[i].amount;
            lock.recipients.push(recipients[i]);
        }
    }

    function handleDelegateUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) public {
        LockDetail storage lock = _locks[lockId];
        for (uint256 i = 0; i < lock.recipients.length; i++) {
            pendingBalance[lock.recipients[i].to] -= lock.recipients[i].amount;
        }
        onUnlock(lockId, recipients);
    }
}
