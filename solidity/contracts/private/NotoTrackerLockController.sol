// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {NotoTrackerERC20} from "./NotoTrackerERC20.sol";

/**
 * @title NotoTrackerLockController
 * @dev Example Noto hooks which allow a user to specify a base ledger contract to
 *      control their locked tokens.
 */
contract NotoTrackerLockController is NotoTrackerERC20 {
    mapping(bytes32 => address) internal _lockControllers;

    constructor(
        string memory name,
        string memory symbol
    ) NotoTrackerERC20(name, symbol) {}

    function onLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlySelf(sender, from) {
        _onLock(sender, lockId, from, amount, data, prepared);

        address contractAddress;
        address accountAddress;
        // TODO: add an optional marker prefix on "data", to indicate when it can be decoded
        (contractAddress, accountAddress) = abi.decode(
            data,
            (address, address)
        );
        _lockControllers[lockId] = contractAddress;
        emit PenteExternalCall(
            contractAddress,
            abi.encodeWithSignature(
                "onLock(bytes32,address,uint256)",
                lockId,
                accountAddress,
                amount
            )
        );
    }

    function onUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override {
        require(recipients.length == 1, "Only one unlock recipient allowed");
        _onUnlock(sender, lockId, recipients, data, prepared);

        address accountAddress;
        // TODO: we should include a signature to prove ownership of accountAddress
        (accountAddress) = abi.decode(data, (address));
        emit PenteExternalCall(
            _lockControllers[lockId],
            abi.encodeWithSignature(
                "onUnlock(bytes32,address,uint256)",
                lockId,
                accountAddress,
                recipients[0].amount
            )
        );
    }
}
