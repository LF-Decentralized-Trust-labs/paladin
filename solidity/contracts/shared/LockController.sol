// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {NotoERC20} from "./NotoERC20.sol";

contract LockController {
    address public immutable logic = address(new NotoERC20());
    mapping(bytes32 => address) public locks;

    function _createToken(
        string memory name,
        string memory symbol,
        address receiver,
        uint256 supply
    ) internal returns (address) {
        address instance = Clones.clone(logic);
        NotoERC20(instance).initialize(
            name,
            symbol,
            address(this),
            receiver,
            supply
        );
        return instance;
    }

    function onLock(bytes32 lockId, address receiver, uint256 amount) external {
        locks[lockId] = _createToken("", "", receiver, amount);
    }

    function onUnlock(bytes32 lockId, address sender, uint256 amount) external {
        NotoERC20(locks[lockId]).redeem(sender, amount);
    }
}
