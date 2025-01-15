// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {INotoHooks} from "./interfaces/INotoHooks.sol";

/**
 * Helpers for tracking locked amounts from Noto hooks contracts.
 */
contract NotoLocks {
    // Details on all currently active locks and their possible unlocks
    mapping(bytes32 => LockDetail) internal _locks;

    // Balances locked by address (still logically owned by that address)
    mapping(address => uint256) internal _lockedBalance;

    // Pending balances from prepared unlocks (not yet owned, but approved to be owned when unlocked)
    mapping(address => uint256) internal _pendingBalance;

    struct LockDetail {
        address from;
        uint256 amount;
        INotoHooks.UnlockRecipient[] recipients;
    }

    function _lock(bytes32 lockId, address from, uint256 amount) internal {
        LockDetail storage lock = _locks[lockId];
        lock.from = from;
        lock.amount = amount;
        _lockedBalance[from] += amount;
    }

    function _unlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) internal {
        LockDetail storage lock = _locks[lockId];
        for (uint256 i = 0; i < recipients.length; i++) {
            lock.amount -= recipients[i].amount;
            _lockedBalance[lock.from] -= recipients[i].amount;
        }

        delete lock.recipients;
        if (lock.amount == 0) {
            delete _locks[lockId];
        }
    }

    function _prepareUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) internal {
        LockDetail storage lock = _locks[lockId];
        delete lock.recipients;

        for (uint256 i = 0; i < recipients.length; i++) {
            _pendingBalance[recipients[i].to] += recipients[i].amount;
            lock.recipients.push(recipients[i]);
        }
    }

    function _handleDelegateUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) internal {
        LockDetail storage lock = _locks[lockId];
        for (uint256 i = 0; i < lock.recipients.length; i++) {
            _pendingBalance[lock.recipients[i].to] -= lock.recipients[i].amount;
        }
        _unlock(lockId, recipients);
    }
}
