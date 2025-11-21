// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Address} from "@openzeppelin/contracts/utils/Address.sol";
import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";
import {NotoLocks} from "../private/NotoLocks.sol";

/**
 * Example Noto hooks which track all Noto token movements on a public ERC20.
 * This version is useful only for testing Noto in isolation, as mirroring all token
 * movements to a public contract defeats the purpose of using Noto to begin with.
 * Real-world applications should use the private NotoTrackerERC20 contract in a
 * Pente privacy group instead.
 */
contract NotoTrackerPublicERC20 is INotoHooks, ERC20 {
    using Address for address;

    NotoLocks internal _locks = new NotoLocks();
    address internal _notary;

    modifier onlyNotary(address sender) {
        require(sender == _notary, "Sender is not the notary");
        _;
    }

    modifier onlySelf(address sender, address from) {
        require(sender == from, "Sender is not the from address");
        _;
    }

    modifier onlyLockOwner(address sender, bytes32 lockId) {
        require(
            sender == _locks.ownerOf(lockId),
            "Sender is not the lock owner"
        );
        _;
    }

    constructor(string memory name, string memory symbol) ERC20(name, symbol) {
        _notary = msg.sender;
    }

    function onMint(
        address sender,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override onlyNotary(sender) {
        _mint(to, amount);
        _executeOperation(prepared);
    }

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override onlySelf(sender, from) {
        _transfer(from, to, amount);
        _executeOperation(prepared);
    }

    function onBurn(
        address sender,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override onlySelf(sender, from) {
        _burn(from, amount);
        _executeOperation(prepared);
    }

    function onLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override onlySelf(sender, from) {
        _locks.onLock(lockId, from, amount);
        _executeOperation(prepared);
    }

    function onCreateMintLock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyNotary(sender) {
        _locks.onPrepareUnlock(lockId, recipients);
        _executeOperation(prepared);
    }

    function onPrepareBurnUnlock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override onlySelf(sender, from) {
        // No recipients (so no pending balances to update here)
        _executeOperation(prepared);
    }

    function onUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override onlyLockOwner(sender, lockId) {
        address from = _locks.ownerOf(lockId);
        _locks.onUnlock(lockId, recipients);
        for (uint256 i = 0; i < recipients.length; i++) {
            _transfer(from, recipients[i].to, recipients[i].amount);
        }
        _executeOperation(prepared);
    }

    function onPrepareUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override onlyLockOwner(sender, lockId) {
        _locks.onPrepareUnlock(lockId, recipients);
        _executeOperation(prepared);
    }

    function onDelegateLock(
        address sender,
        bytes32 lockId,
        address delegate,
        PreparedTransaction calldata prepared
    ) external override {
        _executeOperation(prepared);
    }

    function handleDelegateUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data
    ) external override {
        address from = _locks.ownerOf(lockId);
        _locks.handleDelegateUnlock(lockId, recipients);
        for (uint256 i = 0; i < recipients.length; i++) {
            _transfer(from, recipients[i].to, recipients[i].amount);
        }
    }

    function _executeOperation(PreparedTransaction memory op) internal {
        op.contractAddress.functionCall(op.encodedCall);
    }
}
