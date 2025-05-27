// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";
import {NotoLocks} from "./NotoLocks.sol";

/**
 * @title NotoTrackerERC20
 * @dev Example Noto hooks which track all Noto token movements on a private ERC20.
 */
contract NotoTrackerERC20 is INotoHooks, ERC20 {
    NotoLocks internal _locks  = new NotoLocks();
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

    function _onMint(
        address sender,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) internal virtual {
        _mint(to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) internal virtual {
        _transfer(from, to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onBurn(
        address sender,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) internal virtual {
        _burn(from, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) internal virtual {
        _locks.onLock(lockId, from, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) internal virtual {
        address from = _locks.ownerOf(lockId);
        _locks.onUnlock(lockId, recipients);
        for (uint256 i = 0; i < recipients.length; i++) {
            _transfer(from, recipients[i].to, recipients[i].amount);
        }
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onPrepareUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) internal virtual {
        _locks.onPrepareUnlock(lockId, recipients);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onMint(
        address sender,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyNotary(sender) {
        _onMint(sender, to, amount, data, prepared);
    }

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlySelf(sender, from) {
        _onTransfer(sender, from, to, amount, data, prepared);
    }

    uint256 approvals;

    function onApproveTransfer(
        address sender,
        address from,
        address delegate,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override {
        approvals++; // must store something on each call (see https://github.com/kaleido-io/paladin/issues/252)
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onBurn(
        address sender,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override {
        _onBurn(sender, from, amount, data, prepared);
    }

    function onLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override {
        _onLock(sender, lockId, from, amount, data, prepared);
    }

    function onUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyLockOwner(sender, lockId) {
        _onUnlock(sender, lockId, recipients, data, prepared);
    }

    function onPrepareUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyLockOwner(sender, lockId) {
        _onPrepareUnlock(sender, lockId, recipients, data, prepared);
    }

    function onDelegateLock(
        address sender,
        bytes32 lockId,
        address delegate,
        PreparedTransaction calldata prepared
    ) external virtual override {
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function handleDelegateUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data
    ) external virtual override {
        address from = _locks.ownerOf(lockId);
        _locks.handleDelegateUnlock(lockId, recipients);
        for (uint256 i = 0; i < recipients.length; i++) {
            _transfer(from, recipients[i].to, recipients[i].amount);
        }
    }
}
