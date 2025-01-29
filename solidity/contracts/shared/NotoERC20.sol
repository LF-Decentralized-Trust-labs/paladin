// SPDX-License-Identifier: Apache-2.0
// Compatible with OpenZeppelin Contracts ^5.0.0
pragma solidity ^0.8.20;

import {ERC20Upgradeable} from "@openzeppelin/contracts-upgradeable/token/ERC20/ERC20Upgradeable.sol";
import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";

/**
 * @dev Lightweight ERC20, with a fixed balance allocated to a single receiver at creation.
 */
contract NotoERC20 is Initializable, ERC20Upgradeable, OwnableUpgradeable {
    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(
        string memory name,
        string memory symbol,
        address owner,
        address receiver,
        uint256 supply
    ) public initializer {
        __ERC20_init(name, symbol);
        __Ownable_init(owner);
        _mint(receiver, supply);
    }

    function redeem(address account, uint256 value) public onlyOwner {
        _burn(account, value);
    }
}
