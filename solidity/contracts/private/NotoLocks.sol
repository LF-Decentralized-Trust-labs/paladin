// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

contract NotoLocks {
    struct LockDetail {
        address from;
        uint256 amount;
        address[] recipients;
    }

    mapping(bytes32 => LockDetail) internal _locks;

    function _createLock(
        bytes32 id,
        address from,
        uint256 amount,
        address[] calldata recipients
    ) internal {
        LockDetail storage lock = _locks[id];
        lock.from = from;
        lock.amount = amount;
        lock.recipients = recipients;
    }

    function _removeLock(bytes32 id) internal returns (LockDetail memory) {
        LockDetail memory lock = _locks[id];
        delete _locks[id];
        return lock;
    }
}
