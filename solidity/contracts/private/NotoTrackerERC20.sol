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
        PreparedTransaction calldata prepared
    ) external {
        _mint(to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external {
        _transfer(from, to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onApproveTransfer(
        address sender,
        address from,
        address delegate,
        PreparedTransaction calldata prepared
    ) external {
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onBurn(
        address sender,
        address from,
        uint256 amount,
        PreparedTransaction calldata prepared
    ) external override {
        _burn(from, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onLock(
        address sender,
        bytes32 id,
        address from,
        uint256 amount,
        address[] calldata recipients,
        PreparedTransaction calldata prepared
    ) external override {
        _createLock(id, from, amount, recipients);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onUnlock(
        bytes32 id,
        address recipient
    ) external override {
        LockDetail memory lock = _removeLock(id);
        _transfer(lock.from, recipient, lock.amount);
    }
}
