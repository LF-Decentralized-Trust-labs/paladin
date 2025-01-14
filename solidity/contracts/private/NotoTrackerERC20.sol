// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {INotoHooks} from "../private/interfaces/INotoHooks.sol";
import {NotoLocks} from "./NotoLocks.sol";

/**
 * @dev Example Noto hooks which track all Noto token movements on a private ERC20.
 */
contract NotoTrackerERC20 is INotoHooks, NotoLocks, ERC20 {
    constructor(string memory name, string memory symbol) ERC20(name, symbol) {}

    function onMint(
        address sender,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override {
        _mint(to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override {
        _transfer(from, to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onApproveTransfer(
        address sender,
        address from,
        address delegate,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override {
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onBurn(
        address sender,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override {
        _burn(from, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override {
        _lock(lockId, from, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onUnlock(
        address sender,
        bytes32 lockId,
        address from,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override {
        LockDetail memory lock_ = _unlock(lockId, recipients);
        for (uint256 i = 0; i < recipients.length; i++) {
            _transfer(lock_.from, recipients[i].to, recipients[i].amount);
        }
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onPrepareUnlock(
        address sender,
        bytes32 lockId,
        address from,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external override {
        _prepareUnlock(lockId, recipients);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onDelegateLock(
        address sender,
        bytes32 lockId,
        address from,
        address delegate,
        PreparedTransaction calldata prepared
    ) external override {
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function handleDelegateUnlock(
        address sender,
        bytes32 lockId,
        address from,
        UnlockRecipient[] calldata recipients,
        bytes calldata data
    ) external override {
        LockDetail memory lock_ = _handleDelegateUnlock(lockId, recipients);
        for (uint256 i = 0; i < recipients.length; i++) {
            _transfer(lock_.from, recipients[i].to, recipients[i].amount);
        }
    }
}
