// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

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
        PreparedUnlock[] unlocks;
    }

    struct PreparedUnlock {
        address[] to;
        uint256[] amounts;
    }

    function _lock(bytes32 lockId, address from, uint256 amount) internal {
        LockDetail storage lock = _locks[lockId];
        lock.from = from;
        lock.amount = amount;
        _lockedBalance[from] += amount;
    }

    function _unlock(
        bytes32 lockId,
        uint256[] calldata amounts
    ) internal returns (LockDetail memory) {
        LockDetail storage lock = _locks[lockId];
        for (uint256 i = 0; i < amounts.length; i++) {
            lock.amount -= amounts[i];
            _lockedBalance[lock.from] -= amounts[i];
        }

        LockDetail memory lockCopy = _locks[lockId];
        if (lockCopy.amount == 0) {
            delete _locks[lockId];
        }
        return lockCopy;
    }

    function _prepareUnlock(
        bytes32 lockId,
        address[] calldata to,
        uint256[] calldata amounts
    ) internal {
        for (uint256 i = 0; i < to.length; i++) {
            _pendingBalance[to[i]] += amounts[i];
        }

        LockDetail storage lock = _locks[lockId];
        lock.unlocks.push(PreparedUnlock(to, amounts));
    }

    function _handleDelegateUnlock(
        bytes32 lockId,
        uint256[] calldata amounts
    ) internal returns (LockDetail memory) {
        LockDetail storage lock = _locks[lockId];
        for (uint256 i = 0; i < lock.unlocks.length; i++) {
            PreparedUnlock storage unlock = lock.unlocks[i];
            for (uint256 j = 0; j < unlock.to.length; j++) {
                _pendingBalance[unlock.to[j]] -= unlock.amounts[j];
            }
        }
        delete _locks[lockId].unlocks;

        return _unlock(lockId, amounts);
    }
}
